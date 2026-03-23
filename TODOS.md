# TODOS — Config Connector Integration

Branch: af/featkcc | Updated: 2026-03-23
See docs/proposals/config-connector-integration.md for full design.

## Implementation approach
- API types: KCC Go types per named field (`kcccomputev1beta1.ComputeNetwork`, `kcccontainerv1beta1.ContainerCluster`, etc.)
- Controllers: direct typed field access on KCC structs (no unstructured parsing)
- KCC Go dependency: `github.com/GoogleCloudPlatform/k8s-config-connector` — only the generated API type packages (`pkg/clients/generated/apis/`), which import standard k8s types only (no GCP client libraries)
- Full CRD schema for all KCC fields → ClusterClass patches are validated, `kubectl explain` works

## Phase 1: Revise API Types ✅ DONE

### CAPI v1beta2 contract fixes

- [x] **GCPKCCManagedCluster**: Add `status.initialization.provisioned` (*bool)
  Keep `status.ready` as v1beta1 compat shim.

- [x] **GCPKCCManagedControlPlane**: Add `status.initialization.controlPlaneInitialized` (*bool)
  Add `status.externalManagedControlPlane *bool` (always `true` for GKE).
  Keep `status.initialized` and `status.ready` as v1beta1 compat shims.

- [x] **GCPKCCMachinePool**: Add `spec.providerIDList []string` (MANDATORY per InfraMachinePool contract).
  `status.readyReplicas` reflects actual node count, not desired count.
  Keep `status.ready` as v1beta1 compat shim.

- [x] **All types**: Use `[]metav1.Condition` (v1beta2). No v1beta1 failureReason/failureMessage.

- [x] **CRD labels**: `cluster.x-k8s.io/v1beta2=v1beta2` on all 6 CRDs + templates.

- [x] **Template types**: Created GCPKCCManagedClusterTemplate, GCPKCCManagedControlPlaneTemplate, GCPKCCMachinePoolTemplate.

- [x] **Regenerate**: `make generate` run — 6 CRDs generated, deepcopy generated.

## Phase 2: GCPKCCManagedCluster Controller ✅ DONE

- [x] Feature gate check: `feature.Gates.Enabled(feature.ConfigConnector)` as step 1 of Reconcile()
- [x] KCC CRD presence check in SetupWithManager (ComputeNetwork + ComputeSubnetwork)
- [x] `cluster.x-k8s.io/managed-by` skip (externally managed pattern)
- [x] Pause handling — set Paused condition, return
- [x] Deletion: check KCC resources are gone before removing finalizer
- [x] `status.initialization.provisioned` set when both network resources ready
- [x] v1beta2 conditions: Ready, Paused
- [x] patchSubnetworkCIDRs: patches secondary IP ranges from Cluster.Spec.ClusterNetwork

## Phase 3: GCPKCCManagedControlPlane Controller ✅ DONE

- [x] Feature gate check, KCC CRD check (ContainerCluster), externally-managed check, pause handling
- [x] Gate ContainerCluster creation on GCPKCCManagedCluster being provisioned
- [x] InfrastructureRef kind check before fetching GCPKCCManagedCluster
- [x] `status.externalManagedControlPlane = true` always set
- [x] Kubeconfig generation:
  - Extract CA cert from `containerCluster.Status.ObservedState.MasterAuth.ClusterCaCertificate` (typed)
  - Extract endpoint from `containerCluster.Status.Endpoint` (typed `*string`)
  - Kubeconfig uses `gke-gcloud-auth-plugin` exec credential
  - Secret: name=`<cluster>-kubeconfig`, type=`cluster.x-k8s.io/secret`, key=`value`
- [x] `status.initialization.controlPlaneInitialized` set when ready
- [x] v1beta2 conditions: Available, Paused

## Phase 4: GCPKCCMachinePool Controller ✅ DONE

- [x] Feature gate check, KCC CRD check (ContainerNodePool), pause handling
- [x] Gate ContainerNodePool creation on GCPKCCManagedControlPlane being initialized
- [x] `ReadyReplicas` reflects actual node count from workload cluster Nodes
- [x] `spec.providerIDList` population:
  - Fetches kubeconfig secret
  - Builds workload cluster client
  - Lists Node objects, collects `node.Spec.ProviderID` (format: `gce://<project>/<zone>/<instance>`)
- [x] `status.initialization.provisioned` set when ready
- [x] v1beta2 conditions: Ready, Paused

## Phase 5: Tests ✅ DONE

- [x] Unit tests for pure functions (`gcpkcc_helpers_test.go`):
  - `isKCCConditionTrue`: Ready=True/False, no conditions, absent condition, multi-condition
  - `patchSubnetworkCIDRs`: no network, pods only, services only, both, update in place (typed `ComputeSubnetwork`)
- [x] Reconciler tests for GCPKCCManagedCluster (`gcpkccmanagedcluster_controller_test.go`):
  - Feature gate disabled: no-op
  - NotFound: graceful no-op
  - No owner cluster: requeues
  - Normal reconcile: adds finalizer, creates ComputeNetwork + ComputeSubnetwork
  - Ready once KCC resources ready: sets status.ready and initialization.provisioned
  - Delete waits for KCC resources: requeues while resources still exist
  - Delete removes finalizer: clears finalizer once resources are gone

## Phase 6: Makefile + Installation Script ✅ DONE

- [x] `hack/install-config-connector.sh <version>` — downloads release bundle from GCS, installs CRDs + operator, creates credentials Secret from `GOOGLE_APPLICATION_CREDENTIALS` JSON key, applies cluster-mode ConfigConnector with `spec.credentialSecretName`, waits for readiness.
- [x] `CONF_CONNECTOR_VER ?= 1.146.0` variable in Makefile
- [x] `create-management-cluster-kcc` target — full kind + CAPI + CAPG + KCC
- [x] `install-config-connector` standalone target

## Phase 7: Templates + Documentation ✅ DONE

- [x] `cluster-template-gke-kcc.yaml` — Simple non-topology flavor (Cluster + KCC resources + MachinePool)
- [x] `cluster-template-gke-kcc-clusterclass.yaml` — ClusterClass definition with variables (`project`, `region`, `machineType`) and JSON patches into typed KCC fields
- [x] `cluster-template-gke-kcc-topology.yaml` — Topology-based Cluster referencing the ClusterClass

## Phase 8: Reasonable Defaults + CAPI Field Overrides ✅ DONE

- [x] `gcpkcc_defaults.go`: 6 pure functions — `applyNetworkDefaults`, `applySubnetworkDefaults`, `applyContainerClusterDefaults` (incl. `cnrm.cloud.google.com/remove-default-node-pool` annotation, location from subnet region), `applyContainerClusterOverrides` (minMasterVersion), `applyContainerNodePoolDefaults` (clusterRef from MachinePool.Spec.ClusterName), `applyContainerNodePoolOverrides` (initialNodeCount, version, nodeLocations)
- [x] Controller wiring: `getInfraCluster` returns `*GCPKCCManagedCluster`, `getControlPlane` returns `*GCPKCCManagedControlPlane`, fetches owner MachinePool via `exputil.GetOwnerMachinePool`, delete helpers handle empty names
- [x] `gcpkcc_defaults_test.go`: 31 table-driven tests across all 6 functions

## Remaining (blocking alpha PR review)

- [ ] Write user guide: quickstart, auth setup, end-to-end walkthrough

## Future / Beta

- [ ] Event-driven watches on KCC resources (replace 30s polling)
- [ ] Integration tests (kind + KCC operator)
- [ ] E2E lifecycle tests
- [ ] Validation webhooks for inline CC specs
- [ ] Additional resources: CloudSQL, CloudMemorystore, etc.
