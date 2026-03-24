# TODOS — Config Connector Integration

Branch: af/featkcc | Updated: 2026-03-23
See docs/proposals/config-connector-integration.md for full design.

## Implementation approach
- API types: CAPG-defined intermediate types per named field (covering common KCC fields + `AdditionalConfig` passthrough for advanced fields)
- Controllers: convert intermediate types → unstructured KCC resources at reconcile time; watch owned KCC resources via `Owns()` for immediate status propagation
- No KCC Go module dependency — CAPG defines its own types; no `allowDangerousTypes` needed
- Typed CRD schema for common fields → ClusterClass patches validated, `kubectl explain` works
- Feature gate checked at registration in `main.go` (matching existing GKE pattern)
- Update strategy: strategic merge patch (preserves user-managed fields, re-applies forced overrides)
- Namespace scoping: KCC resources created in same namespace as owning CAPG resource; GCP project configured at namespace level via KCC `ConfigConnectorContext`

## Phase 1: API Types + Intermediate Type Definitions

### Intermediate types (CAPG-defined, no KCC Go dependency)

- [ ] **GCPKCCComputeNetworkSpec**: Common ComputeNetwork fields (`autoCreateSubnetworks`, `routingMode`, etc.) + `AdditionalConfig *runtime.RawExtension`
- [ ] **GCPKCCComputeSubnetworkSpec**: Common ComputeSubnetwork fields (`ipCidrRange`, `region`, `secondaryIpRange`, etc.) + `AdditionalConfig`
- [ ] **GCPKCCContainerClusterSpec**: Common ContainerCluster fields (`location`, `networkingMode`, `initialNodeCount`, `networkRef`, `subnetworkRef`, `ipAllocationPolicy`, `minMasterVersion`, etc.) + `AdditionalConfig`
- [ ] **GCPKCCContainerNodePoolSpec**: Common ContainerNodePool fields (`location`, `initialNodeCount`, `version`, `nodeLocations`, `machineType`, `clusterRef`, etc.) + `AdditionalConfig`
- [ ] **Conversion helpers**: intermediate types → unstructured KCC resources (merge typed fields + AdditionalConfig)

### CAPI v1beta2 contract compliance

- [ ] **GCPKCCManagedCluster** (`gcpkccmc`): Add `status.initialization.provisioned` (*bool)
  Keep `status.ready` as v1beta1 compat shim.

- [ ] **GCPKCCManagedControlPlane** (`gcpkccmcp`): Add `status.initialization.controlPlaneInitialized` (*bool)
  Add `status.externalManagedControlPlane *bool` (always `true` for GKE).
  Keep `status.initialized` and `status.ready` as v1beta1 compat shims.
  Rename `spec.cluster` → `spec.containerCluster` (avoids ambiguity with CAPI Cluster).

- [ ] **GCPKCCManagedMachinePool** (`gcpkccmmp`): Add `spec.providerIDList []string` (MANDATORY per InfraMachinePool contract).
  `status.readyReplicas` reflects actual node count, not desired count.
  Keep `status.ready` as v1beta1 compat shim.

- [ ] **All types**: Use `[]metav1.Condition` (v1beta2). No v1beta1 failureReason/failureMessage.

- [ ] **CRD labels**: `cluster.x-k8s.io/v1beta2=v1beta2` on all 6 CRDs + templates.

- [ ] **Short names**: `gcpkccmc`, `gcpkccmcp`, `gcpkccmmp`, `gcpkccmct`, `gcpkccmcpt`, `gcpkccmmpt`

- [ ] **Print columns**: Ready, Version, Replicas, Age as appropriate per type.

- [ ] **Template types**: GCPKCCManagedClusterTemplate, GCPKCCManagedControlPlaneTemplate, GCPKCCManagedMachinePoolTemplate.

- [ ] **Regenerate**: `make generate` — 6 CRDs with manageable schemas, deepcopy generated.

## Phase 2: GCPKCCManagedCluster Controller + Defaults/Overrides

- [ ] Feature gate at registration in `main.go` (matching existing GKE pattern)
- [ ] KCC CRD presence check in SetupWithManager (ComputeNetwork + ComputeSubnetwork)
- [ ] `cluster.x-k8s.io/managed-by` skip (externally managed pattern)
- [ ] Add finalizer
- [ ] Pause handling — set Paused condition, return
- [ ] Defaults: `applyNetworkDefaults`, `applySubnetworkDefaults` (inline with controller)
- [ ] Overrides: CAPI CIDR ranges → secondary IP ranges (forced)
- [ ] Convert intermediate types → unstructured KCC resources, set namespace (same as owner), set owner ref
- [ ] Watch owned ComputeNetwork + ComputeSubnetwork via `Owns()` for immediate status propagation
- [ ] Patch existing KCC resources via strategic merge patch
- [ ] `status.initialization.provisioned` set when both network resources ready
- [ ] v1beta2 conditions: Ready, Paused, Degraded (30m reconciliation timeout)
- [ ] Deletion: check KCC resources are gone before removing finalizer; `DeletionBlocked` condition after 30m

## Phase 3: GCPKCCManagedControlPlane Controller + Defaults/Overrides

- [ ] Feature gate at registration, KCC CRD check (ContainerCluster), externally-managed check, pause handling
- [ ] Add finalizer
- [ ] Defaults: `applyContainerClusterDefaults` (inline); region defaulted from subnetwork if user doesn't provide
- [ ] Overrides: `spec.version` → `minMasterVersion` (forced, documented)
- [ ] Gate ContainerCluster creation on GCPKCCManagedCluster being provisioned (via `getInfraCluster`)
- [ ] InfrastructureRef kind check before fetching GCPKCCManagedCluster
- [ ] `status.externalManagedControlPlane = true` always set
- [ ] Watch owned ContainerCluster via `Owns()`
- [ ] Kubeconfig generation — reuse existing GKE auth pattern from current provider
- [ ] `status.initialization.controlPlaneInitialized` set when ready
- [ ] v1beta2 conditions: Available, Paused, Degraded (30m timeout)
- [ ] Deletion: `DeletionBlocked` condition after 30m

## Phase 4: GCPKCCManagedMachinePool Controller + Defaults/Overrides

- [ ] Feature gate at registration, KCC CRD check (ContainerNodePool), pause handling
- [ ] Add finalizer
- [ ] Defaults: `applyContainerNodePoolDefaults` (inline)
- [ ] Overrides: `replicas` → `initialNodeCount`, `version`, `failureDomains` → `nodeLocations` (forced; document autoscaler interaction — `initialNodeCount` is creation-time only, autoscaler manages after)
- [ ] Gate ContainerNodePool creation on GCPKCCManagedControlPlane being initialized (via `getControlPlane`)
- [ ] Fetch owner MachinePool via `exputil.GetOwnerMachinePool`
- [ ] Watch owned ContainerNodePool via `Owns()` for real-time `status.replicas` updates
- [ ] `spec.providerIDList` population:
  - Investigate GCP Compute API (instance group URLs) as primary source
  - Workload cluster Node listing as fallback
  - Gate on kubeconfig secret availability for fallback path
- [ ] `status.initialization.provisioned` set when ready
- [ ] v1beta2 conditions: Ready, Paused, Degraded (30m timeout)
- [ ] Deletion: `DeletionBlocked` condition after 30m

## Phase 5: Tests

- [ ] Unit tests for pure functions (`gcpkcc_helpers_test.go`):
  - `isKCCConditionTrue`: Ready=True/False, no conditions, absent condition, multi-condition
  - `patchSubnetworkCIDRs`: no network, pods only, services only, both, update in place
  - Intermediate type → unstructured conversion: typed fields, AdditionalConfig merge, field overlap
- [ ] Defaults/overrides tests (`gcpkcc_defaults_test.go`): table-driven for all 6 `apply*` functions
- [ ] Reconciler tests for all 3 controllers:
  - Feature gate disabled, NotFound, no owner, normal reconcile, readiness, delete waits/completes
  - 30m timeout → Degraded condition
  - Forced override of user-provided value (verify field is overwritten)

## Phase 6: Template Flavors

- [ ] `cluster-template-gke-kcc.yaml` — Simple non-topology flavor (Cluster + KCC resources + MachinePool)
- [ ] `cluster-template-gke-kcc-clusterclass.yaml` — ClusterClass definition with variables (`region`, `machineType`) and JSON patches into typed intermediate fields
- [ ] `cluster-template-gke-kcc-topology.yaml` — Topology-based Cluster referencing the ClusterClass

## Phase 7: Makefile + Installation Script (development/E2E only)

- [ ] `hack/install-config-connector.sh <version>` — downloads release bundle from GCS, installs CRDs + operator, creates credentials Secret from `GOOGLE_APPLICATION_CREDENTIALS` JSON key, applies cluster-mode ConfigConnector with `spec.credentialSecretName`, waits for readiness
- [ ] `CONF_CONNECTOR_VER ?= 1.146.0` variable in Makefile
- [ ] `create-management-cluster-kcc` target — full kind + CAPI + CAPG + KCC
- [ ] `install-config-connector` standalone target
- [ ] Note: this is for development and E2E testing, NOT for cluster-api-operator installs

## Remaining

- [ ] Write user guide: quickstart, auth setup, end-to-end walkthrough
- [ ] Document forced overrides and which fields are controller-managed
- [ ] Document autoscaler interaction with `replicas` → `initialNodeCount`
- [ ] Document namespace scoping: KCC resources in owner namespace, project via ConfigConnectorContext

## Future / Beta

- [ ] Integration tests within existing framework (user-driven initially, automation as last step)
- [ ] E2E lifecycle tests
- [ ] Validation webhooks for inline CC specs
- [ ] Additional resources: CloudSQL, CloudMemorystore, etc.

## Design decisions (from review)

## Post-Implementation Review (Addendum A1-A8)

- [ ] **A1: Use CAPI v1beta2 condition constants** — Replace custom reasons with upstream `clusterv1.*Reason` constants (WaitingForClusterInfrastructureReadyReason, DeletingReason, ReadyReason, etc.). Keep KCC-specific condition types (KCCNetworkReady, etc.).
- [ ] **A2: Use RawExtension for KCC specs** — Replace typed intermediate structs with `*runtime.RawExtension` for each KCC resource spec. Remove all passthrough-only types (KCCReleaseChannel, KCCWorkloadIdentityConfig, KCCPrivateClusterConfig, KCCMasterAuthorizedNetworksConfig, KCCNodePoolAutoscaling, KCCNodePoolManagement, KCCNodeConfig, etc.). Controller defaults/overrides operate by unmarshalling RawExtension → map, setting/overriding keys, marshalling back. Keep only KCCResourceRef, KCCSecondaryIPRange, KCCIPAllocationPolicy as helper types for override logic. ClusterClass patches work unchanged — JSON patches operate on raw JSON regardless of Go types (verified: CAPI webhook does NOT validate paths against CRD schema).
- [ ] **A3: Default namespace on KCC resources** — Add `if Metadata.Namespace == "": set to ownerNamespace` in all apply*Defaults functions.
- [ ] **A4: Define KCC GVK constants once** — Move GVK constants to API package (`exp/api/v1beta1/gcpkcc_conversion.go`). Remove duplicates from `gcpkcc_helpers.go`. Reference via `infrav1exp.*GVK`.
- [ ] **A5: Use metav1.ObjectMeta** — Replace custom `KCCObjectMeta` with `metav1.ObjectMeta`. Update conversion, controllers, defaults.
- [ ] **A6: Simplify deleteResource** — Rename `deleteKCCResourceIfExists` → `deleteResource`. Simplify to delete + handle NotFound. Move to `gcpkcc_helpers.go` as standalone function. Remove 3 method duplicates.
- [ ] **A7: Kubeconfig with bearer token** — Replace gke-gcloud-auth-plugin exec credential with actual OAuth2 token from IAM Credentials API. Create lightweight credential helper in `exp/controllers/gcpkcc_credentials.go` that reads SA key from CAPG GCP secret.
- [ ] **A8: Status.replicas from KCC nodeCount** — Set `cnrm.cloud.google.com/state-into-spec: merge` annotation on ContainerNodePool. Read `spec.initialNodeCount` from KCC resource for `status.replicas`. Document autoscaler limitation.

---

## Design Decisions

- **Intermediate types over full KCC embedding**: CRD size risk with full KCC types; intermediate types give ClusterClass validation for common fields + passthrough for advanced fields
- **Permanent coexistence**: KCC path coexists with existing GKE path; this is a proposal for the upstream project
- **No scope pattern**: KCC controllers don't call GCP APIs directly (KCC handles that), so the scope pattern adds ceremony without value — deliberate divergence from existing CAPG controllers
- **Watches not polling**: `Owns()` on KCC resources gives immediate status propagation, including real-time `status.replicas` updates on drift/external changes
- **Namespace-level project**: GCP project configured via KCC `ConfigConnectorContext` at namespace level, not repeated per-resource
- **Region defaulting**: Subnetwork region is used as soft default for ContainerCluster location if user doesn't provide it
- **Kubeconfig auth**: Reuse existing GKE auth pattern from current provider
- **Crossplane not considered**: Not a Google-supported solution
