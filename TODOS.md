# TODOS — Config Connector Integration

Branch: af/featkcc | Updated: 2026-03-23
See docs/proposals/config-connector-integration.md for full design.

## Implementation approach
- API types: KCC Go types per named field (`kcccomputev1beta1.ComputeNetwork`, `kcccontainerv1beta1.ContainerCluster`, etc.)
- Controllers: direct typed field access on KCC structs (no unstructured parsing)
- KCC Go dependency: `github.com/GoogleCloudPlatform/k8s-config-connector` — only the generated API type packages (`pkg/clients/generated/apis/`), which import standard k8s types only (no GCP client libraries)
- Full CRD schema for all KCC fields → ClusterClass patches are validated, `kubectl explain` works

## Phase 1: API Types

### CAPI v1beta2 contract fixes

- [ ] **GCPKCCManagedCluster**: Add `status.initialization.provisioned` (*bool)
  Keep `status.ready` as v1beta1 compat shim.

- [ ] **GCPKCCManagedControlPlane**: Add `status.initialization.controlPlaneInitialized` (*bool)
  Add `status.externalManagedControlPlane *bool` (always `true` for GKE).
  Keep `status.initialized` and `status.ready` as v1beta1 compat shims.

- [ ] **GCPKCCMachinePool**: Add `spec.providerIDList []string` (MANDATORY per InfraMachinePool contract).
  `status.readyReplicas` reflects actual node count, not desired count.
  Keep `status.ready` as v1beta1 compat shim.

- [ ] **All types**: Use `[]metav1.Condition` (v1beta2). No v1beta1 failureReason/failureMessage.

- [ ] **CRD labels**: `cluster.x-k8s.io/v1beta2=v1beta2` on all 6 CRDs + templates.

- [ ] **Template types**: GCPKCCManagedClusterTemplate, GCPKCCManagedControlPlaneTemplate, GCPKCCMachinePoolTemplate.

- [ ] **KCC Go types**: Use typed KCC structs (`kcccomputev1beta1.ComputeNetwork`, `kcccontainerv1beta1.ContainerCluster`, etc.) instead of `runtime.RawExtension`. Add `allowDangerousTypes=true` to controller-gen (KCC uses `float64` fields).

- [ ] **Regenerate**: `make generate` — 6 CRDs with full KCC schemas, deepcopy generated.

## Phase 2: GCPKCCManagedCluster Controller

- [ ] Feature gate check: `feature.Gates.Enabled(feature.ConfigConnector)` as step 1 of Reconcile()
- [ ] KCC CRD presence check in SetupWithManager (ComputeNetwork + ComputeSubnetwork)
- [ ] `cluster.x-k8s.io/managed-by` skip (externally managed pattern)
- [ ] Add finalizer
- [ ] Pause handling — set Paused condition, return
- [ ] Deletion: check KCC resources are gone before removing finalizer
- [ ] `status.initialization.provisioned` set when both network resources ready
- [ ] v1beta2 conditions: Ready, Paused
- [ ] patchSubnetworkCIDRs: patches secondary IP ranges from Cluster.Spec.ClusterNetwork

## Phase 3: GCPKCCManagedControlPlane Controller

- [ ] Feature gate check, KCC CRD check (ContainerCluster), externally-managed check, pause handling
- [ ] Add finalizer
- [ ] Gate ContainerCluster creation on GCPKCCManagedCluster being provisioned (via `getInfraCluster`)
- [ ] InfrastructureRef kind check before fetching GCPKCCManagedCluster
- [ ] `status.externalManagedControlPlane = true` always set
- [ ] Kubeconfig generation:
  - Extract CA cert from `containerCluster.Status.ObservedState.MasterAuth.ClusterCaCertificate` (typed)
  - Extract endpoint from `containerCluster.Status.Endpoint` (typed `*string`)
  - Kubeconfig uses `gke-gcloud-auth-plugin` exec credential
  - Secret: name=`<cluster>-kubeconfig`, type=`cluster.x-k8s.io/secret`, key=`value`
- [ ] `status.initialization.controlPlaneInitialized` set when ready
- [ ] v1beta2 conditions: Available, Paused

## Phase 4: GCPKCCMachinePool Controller

- [ ] Feature gate check, KCC CRD check (ContainerNodePool), pause handling
- [ ] Add finalizer
- [ ] Gate ContainerNodePool creation on GCPKCCManagedControlPlane being initialized (via `getControlPlane`)
- [ ] Fetch owner MachinePool via `exputil.GetOwnerMachinePool`
- [ ] `ReadyReplicas` reflects actual node count from workload cluster Nodes
- [ ] `spec.providerIDList` population:
  - Fetches kubeconfig secret
  - Builds workload cluster client
  - Lists Node objects, collects `node.Spec.ProviderID` (format: `gce://<project>/<zone>/<instance>`)
- [ ] `status.initialization.provisioned` set when ready
- [ ] v1beta2 conditions: Ready, Paused

## Phase 5: Tests

- [ ] Unit tests for pure functions (`gcpkcc_helpers_test.go`):
  - `isKCCConditionTrue`: Ready=True/False, no conditions, absent condition, multi-condition
  - `patchSubnetworkCIDRs`: no network, pods only, services only, both, update in place (typed `ComputeSubnetwork`)
- [ ] Reconciler tests for GCPKCCManagedCluster (`gcpkccmanagedcluster_controller_test.go`):
  - Feature gate disabled, NotFound, no owner, normal reconcile, readiness, delete waits/completes

## Phase 6: Makefile + Installation Script

- [ ] `hack/install-config-connector.sh <version>` — downloads release bundle from GCS, installs CRDs + operator, creates credentials Secret from `GOOGLE_APPLICATION_CREDENTIALS` JSON key, applies cluster-mode ConfigConnector with `spec.credentialSecretName`, waits for readiness
- [ ] `CONF_CONNECTOR_VER ?= 1.146.0` variable in Makefile
- [ ] `create-management-cluster-kcc` target — full kind + CAPI + CAPG + KCC
- [ ] `install-config-connector` standalone target

## Phase 7: Template Flavors

- [ ] `cluster-template-gke-kcc.yaml` — Simple non-topology flavor (Cluster + KCC resources + MachinePool)
- [ ] `cluster-template-gke-kcc-clusterclass.yaml` — ClusterClass definition with variables (`project`, `region`, `machineType`) and JSON patches into typed KCC fields
- [ ] `cluster-template-gke-kcc-topology.yaml` — Topology-based Cluster referencing the ClusterClass

## Phase 8: Reasonable Defaults + CAPI Field Overrides

### Defaults (fill empty fields, user values win)

| Source | Destination | Resource |
|--------|-------------|----------|
| `{kccCluster.Name}` | `metadata.name` | ComputeNetwork |
| `false` | `spec.autoCreateSubnetworks` | ComputeNetwork |
| `"REGIONAL"` | `spec.routingMode` | ComputeNetwork |
| `{kccCluster.Name}` | `metadata.name` | ComputeSubnetwork |
| resolved network name | `spec.networkRef.name` | ComputeSubnetwork |
| CAPI `Cluster.Name` | `metadata.name` | ContainerCluster |
| `GCPKCCManagedCluster.Status.NetworkName` | `spec.networkRef.name` | ContainerCluster |
| `GCPKCCManagedCluster.Status.SubnetworkName` | `spec.subnetworkRef.name` | ContainerCluster |
| `1` | `spec.initialNodeCount` | ContainerCluster |
| `"VPC_NATIVE"` | `spec.networkingMode` | ContainerCluster |
| `{pods, services}` range names | `spec.ipAllocationPolicy` | ContainerCluster (when CAPI CIDRs set) |
| `"true"` | annotation `cnrm.cloud.google.com/remove-default-node-pool` | ContainerCluster |
| `GCPKCCManagedCluster.Spec.Subnetwork.Spec.Region` | `spec.location` | ContainerCluster |
| `{kccMP.Name}` | `metadata.name` | ContainerNodePool |
| `MachinePool.Spec.ClusterName` | `spec.clusterRef.name` | ContainerNodePool |
| ContainerCluster `spec.location` | `spec.location` | ContainerNodePool |

### Forced overrides (CAPI always wins)

| Source (CAPI) | Destination (KCC) | Resource |
|---------------|-------------------|----------|
| `Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]` | `spec.secondaryIpRange[pods].ipCidrRange` | ComputeSubnetwork |
| `Cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]` | `spec.secondaryIpRange[services].ipCidrRange` | ComputeSubnetwork |
| `GCPKCCManagedControlPlane.Spec.Version` | `spec.minMasterVersion` | ContainerCluster |
| `MachinePool.Spec.Replicas` | `spec.initialNodeCount` | ContainerNodePool |
| `MachinePool.Spec.Template.Spec.Version` | `spec.version` | ContainerNodePool |
| `MachinePool.Spec.FailureDomains` | `spec.nodeLocations` | ContainerNodePool |

### Implementation

- [ ] `gcpkcc_defaults.go`: 6 pure functions (`applyNetworkDefaults`, `applySubnetworkDefaults`, `applyContainerClusterDefaults`, `applyContainerClusterOverrides`, `applyContainerNodePoolDefaults`, `applyContainerNodePoolOverrides`)
- [ ] Controller wiring: `getInfraCluster` returns `*GCPKCCManagedCluster`, `getControlPlane` returns `*GCPKCCManagedControlPlane`, fetch owner MachinePool, delete helpers handle empty names
- [ ] `gcpkcc_defaults_test.go`: table-driven tests for all 6 functions

## Remaining

- [ ] Write user guide: quickstart, auth setup, end-to-end walkthrough

## Future / Beta

- [ ] Event-driven watches on KCC resources (replace 30s polling)
- [ ] Integration tests (kind + KCC operator)
- [ ] E2E lifecycle tests
- [ ] Validation webhooks for inline CC specs
- [ ] Additional resources: CloudSQL, CloudMemorystore, etc.
