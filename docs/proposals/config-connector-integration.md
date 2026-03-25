# Config Connector Integration Proposal

## Summary

This proposal introduces Config Connector (KCC) integration in cluster-api-provider-gcp (CAPG), enabling users to manage GKE clusters through Config Connector resources while maintaining full Cluster API (CAPI) v1beta2 contract compliance. The design adds new provider types (`GCPKCCManagedCluster`, `GCPKCCManagedControlPlane`, `GCPKCCManagedMachinePool`) as a parallel path alongside the existing GKE provider — users choose one or the other, not a migration.

## Motivation

### Goals

1. **Enable Advanced GKE Features**: Provide access to the full GKE API surface through Config Connector — Binary Authorization, Security Posture, Managed Prometheus, and hundreds of other fields that CAPG does not and should not expose directly
2. **Respect all CAPI v1beta2 Contracts**: Full compliance with the [CAPI provider contracts](https://cluster-api.sigs.k8s.io/developer/providers/contracts/overview) — all required fields, status fields, conditions, labels, and kubeconfig conventions
3. **Stronger Typing than CAPZ/ASO**: Use named typed fields (`spec.network`, `spec.containerCluster`, `spec.nodePool`) rather than a generic untyped `[]runtime.RawExtension` list, so each resource role is explicit in the API
4. **Minimize Field Duplication**: Leverage existing CAPI fields (`spec.version`, `spec.replicas`, cluster network CIDRs) instead of re-declaring them; CAPG only patches the CAPI-derived fields
5. **Maintain CAPI Compatibility**: Ensure full integration with CAPI workflows and tools (`clusterctl`, `kubectl get cluster`, etc.)
6. **Support GitOps**: Enable Kubernetes-native, declarative GCP resource management via ArgoCD, Flux, or any GitOps tool
7. **Simplify Maintenance**: Reduce CAPG code by delegating GCP resource management to Config Connector

### Non-Goals

1. Support for self-managed (non-GKE) clusters
2. Automatic migration from existing CAPG clusters to the KCC path
3. Bundling KCC with CAPG — KCC is a user-managed prerequisite (see below). CAPG provides `hack/install-config-connector.sh` for development and E2E testing only.
4. Replacing the existing GKE provider — the KCC path permanently coexists as a parallel option for users who need the full GKE API surface

### Why KCC Must Be Installed Separately

Config Connector must be installed independently by users for two reasons:

1. **cluster-api-operator uses kustomize** and has no mechanism to disable a bundled component's installation. If CAPG bundled KCC, users who don't want KCC (the majority) would get it anyway.
2. **KCC requires `cnrm-system` namespace configuration** (Google Service Account binding, namespace modes, etc.) that is environment-specific and cannot be managed by CAPG. Users must configure KCC for their GCP project.

CAPG's controllers should detect whether KCC CRDs are present at startup and fail gracefully if not, rather than panic or crash-loop.

## Design Overview

### Design Precedent

This design is inspired by the [CAPZ Azure Service Operator integration](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/main/docs/proposals/20230123-azure-service-operator.md) but differs in key ways:

| Aspect | CAPZ/ASO | This design |
|--------|----------|-------------|
| Resource embedding | `[]runtime.RawExtension` (generic list) | Named typed fields (`spec.network`, `spec.containerCluster`, etc.) using CAPG-defined intermediate types |
| Resource identity | Users list any resources in any order | Each field has a defined role — network, subnetwork, containerCluster, nodePool |
| CAPI field patching | JSON merge patches via mutator pipeline | Direct typed field access on intermediate structs, converted to KCC unstructured at reconcile time |
| Go dependency | Full ASO Go types imported | No KCC Go dependency — CAPG defines its own types covering common fields; additional fields via `runtime.RawExtension` passthrough |
| ClusterClass compatibility | Full schema in CRD | Typed schema for common fields (ClusterClass patches validated); passthrough for advanced fields |
| Dependency tracking | Shared `ResourceReconciler` helper | Per-controller sequential reconciliation |

### Architecture

```
Management Cluster
─────────────────────────────────────────────────────
  CAPI Cluster ──── InfrastructureRef ──▶ GCPKCCManagedCluster
                ╰── ControlPlaneRef   ──▶ GCPKCCManagedControlPlane
                                              │
  CAPI MachinePool ─ InfraRef ──▶ GCPKCCManagedMachinePool

  GCPKCCManagedCluster
    └── creates ──▶ ComputeNetwork (KCC)   ──▶ GCP VPC
    └── creates ──▶ ComputeSubnetwork (KCC)──▶ GCP Subnet

  GCPKCCManagedControlPlane
    └── creates ──▶ ContainerCluster (KCC)  ──▶ GKE Control Plane
    └── writes  ──▶ <cluster>-kubeconfig (Secret)

  GCPKCCManagedMachinePool
    └── creates ──▶ ContainerNodePool (KCC) ──▶ GKE Node Pool

  Config Connector controllers watch KCC resources
  and reconcile them to GCP via the GCP API.
─────────────────────────────────────────────────────
```

## API Design

### Key Principles

1. **Named typed fields** for each resource role (`network`, `subnetwork`, `containerCluster`, `nodePool`) rather than a generic list
2. **CAPG-defined intermediate types** — each field uses a CAPG-defined Go struct that covers the most commonly used KCC fields (e.g. `GCPKCCContainerClusterSpec`), giving typed CRD schema, ClusterClass patch validation, and `kubectl explain` support for common fields. Advanced/uncommon KCC fields are accessible via a `runtime.RawExtension` passthrough field on each resource.
3. **No KCC Go module dependency** — CAPG defines its own types mirroring KCC field structure. Controllers convert these to unstructured KCC resources at reconcile time. This avoids coupling to KCC's ALPHA Go client and keeps CRD sizes manageable.
4. **CAPI field minimization** — only fields that CAPG must patch for CAPI compatibility are enforced; everything else is user-controlled via the typed fields or the passthrough extension
5. **CAPI v1beta2 contract compliance** — all required spec/status fields, conditions, and labels as per the current contract spec
6. **Namespace-scoped KCC resources** — KCC resources are created in the same namespace as their owning CAPG resource by default. GCP project is configured at the KCC namespace level via `ConfigConnectorContext`, not repeated per-resource.

### Intermediate Type Pattern

Instead of embedding full KCC Go types (which would produce enormous CRDs and couple CAPG to KCC's ALPHA Go client), CAPG defines its own intermediate types that cover the most commonly used fields for each KCC resource kind. Each intermediate type includes:

- **Typed fields** for commonly-configured options (validated by CRD schema, patchable by ClusterClass)
- **`AdditionalConfig *runtime.RawExtension`** passthrough for advanced KCC fields not covered by the typed fields

At reconcile time, controllers merge the typed fields and `AdditionalConfig` into an unstructured KCC resource for creation/update. This gives ClusterClass validation on common fields while preserving access to KCC's full API surface.

### GCPKCCManagedCluster (InfraCluster)

Implements the [InfraCluster contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster).

```go
// +kubebuilder:resource:shortName=gcpkccmc
type GCPKCCManagedClusterSpec struct {
    // Network defines the Config Connector ComputeNetwork resource.
    // CAPG creates and manages the lifecycle of this resource.
    // +required
    Network GCPKCCComputeNetworkSpec `json:"network"`

    // Subnetwork defines the Config Connector ComputeSubnetwork resource.
    // CAPG patches spec.secondaryIpRange from Cluster.Spec.ClusterNetwork.
    // +required
    Subnetwork GCPKCCComputeSubnetworkSpec `json:"subnetwork"`

    // ControlPlaneEndpoint is the endpoint used to communicate with the control plane.
    // Populated by the ControlPlane provider once the GKE cluster endpoint is available.
    // +optional
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

type GCPKCCManagedClusterStatus struct {
    // Initialization contains fields per the v1beta2 InfraCluster contract.
    // +optional
    Initialization *GCPKCCManagedClusterInitializationStatus `json:"initialization,omitempty"`

    // Ready is a v1beta1 compatibility field. Use Initialization.Provisioned for v1beta2.
    // Deprecated: will be removed ~August 2026.
    // +optional
    Ready bool `json:"ready,omitempty"`

    // FailureDomains lists the failure domains available for the cluster.
    // +optional
    FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`

    // Conditions represents the observations of the cluster's current state.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // NetworkName is the name of the created ComputeNetwork resource.
    // +optional
    NetworkName string `json:"networkName,omitempty"`

    // SubnetworkName is the name of the created ComputeSubnetwork resource.
    // +optional
    SubnetworkName string `json:"subnetworkName,omitempty"`
}

type GCPKCCManagedClusterInitializationStatus struct {
    // Provisioned is true when the network infrastructure is fully provisioned.
    // Required by the InfraCluster v1beta2 contract.
    // +optional
    Provisioned *bool `json:"provisioned,omitempty"`
}
```

**CAPI contract compliance:**

| Contract requirement | Implementation |
|---|---|
| `status.initialization.provisioned` (*bool) | ✅ Required — set true when both ComputeNetwork and ComputeSubnetwork have `Ready=True` |
| `status.ready` (bool) | ✅ v1beta1 compat field, same value |
| `spec.controlPlaneEndpoint` | ✅ Populated by ControlPlane controller |
| `status.conditions[Ready]` | ✅ Set when resources are ready |
| `status.conditions[Paused]` | ✅ Set when cluster is paused |
| Finalizer | ✅ `gcpkccmanagedcluster.infrastructure.cluster.x-k8s.io` |
| `cluster.x-k8s.io/managed-by` skip | ✅ Use `ResourceIsNotExternallyManaged` predicate |
| CRD label `cluster.x-k8s.io/v1beta2: v1beta2` | ✅ Required |

### GCPKCCManagedControlPlane (ControlPlane Provider)

Implements the [ControlPlane contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/control-plane).

```go
// +kubebuilder:resource:shortName=gcpkccmcp
type GCPKCCManagedControlPlaneSpec struct {
    // ContainerCluster defines the Config Connector ContainerCluster resource.
    // CAPG creates this resource and manages its lifecycle.
    // +required
    ContainerCluster GCPKCCContainerClusterSpec `json:"containerCluster"`

    // Version is the Kubernetes version for the control plane.
    // +optional
    Version *string `json:"version,omitempty"`

    // ControlPlaneEndpoint represents the API server endpoint.
    // Populated when the ContainerCluster becomes available.
    // +optional
    ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

type GCPKCCManagedControlPlaneStatus struct {
    // Initialization contains fields per the v1beta2 ControlPlane contract.
    // +optional
    Initialization *GCPKCCManagedControlPlaneInitializationStatus `json:"initialization,omitempty"`

    // ExternalManagedControlPlane indicates that the control plane is managed
    // externally (by GKE) and not by individual CAPI Machine objects.
    // Required by the ControlPlane v1beta2 contract for managed (serverless) control planes.
    // Always true for this provider.
    // +optional
    ExternalManagedControlPlane *bool `json:"externalManagedControlPlane,omitempty"`

    // Ready is a v1beta1 compatibility field.
    // Deprecated: will be removed ~August 2026.
    // +optional
    Ready bool `json:"ready,omitempty"`

    // Initialized is a v1beta1 compatibility field.
    // Deprecated: use Initialization.ControlPlaneInitialized.
    // +optional
    Initialized bool `json:"initialized,omitempty"`

    // Version is the observed Kubernetes version of the GKE cluster.
    // Required by the ControlPlane contract when spec.version is implemented.
    // +optional
    Version string `json:"version,omitempty"`

    // Conditions represents the observations of the control plane's current state.
    // Includes: Available, Paused, RollingOut, ScalingUp, ScalingDown.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // ClusterName is the name of the created ContainerCluster resource.
    // +optional
    ClusterName string `json:"clusterName,omitempty"`
}

type GCPKCCManagedControlPlaneInitializationStatus struct {
    // ControlPlaneInitialized is true when the GKE API server can accept requests.
    // Required by the ControlPlane v1beta2 contract.
    // +optional
    ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}
```

**CAPI contract compliance:**

| Contract requirement | Implementation |
|---|---|
| `status.initialization.controlPlaneInitialized` (*bool) | ✅ Required — set true when ContainerCluster `Ready=True` |
| `status.externalManagedControlPlane = true` | ✅ Required for managed (GKE) control planes |
| `spec.version` | ✅ Patched into ContainerCluster `spec.minMasterVersion` |
| `status.version` | ✅ Populated from ContainerCluster `status.masterVersion` |
| `spec.controlPlaneEndpoint` | ✅ Populated from ContainerCluster `status.endpoint` |
| Kubeconfig secret | ✅ `<cluster>-kubeconfig`, type `cluster.x-k8s.io/secret`, key `value` |
| `status.conditions[Available]` | ✅ Set when cluster is Running |
| `status.conditions[Paused]` | ✅ Set when paused |
| Finalizer | ✅ `gcpkccmanagedcontrolplane.infrastructure.cluster.x-k8s.io` |
| `status.ready` / `status.initialized` | ✅ v1beta1 compat |

**Kubeconfig generation:** When the ContainerCluster reaches `Ready=True`, CAPG extracts the cluster CA certificate and endpoint from the ContainerCluster status and generates a kubeconfig using the `exec` credential mode (pointing to the `gke-gcloud-auth-plugin`). The secret is named `<cluster>-kubeconfig`, in the same namespace as the CAPI Cluster, with type `cluster.x-k8s.io/secret` and label `cluster.x-k8s.io/cluster-name=<cluster>`. This follows the CAPI kubeconfig secret convention exactly.

### GCPKCCManagedMachinePool (InfraMachinePool)

Implements the [InfraMachinePool contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-machine-pool).

```go
// +kubebuilder:resource:shortName=gcpkccmmp
type GCPKCCManagedMachinePoolSpec struct {
    // NodePool defines the Config Connector ContainerNodePool resource.
    // CAPG creates this resource and manages its lifecycle.
    // +required
    NodePool GCPKCCContainerNodePoolSpec `json:"nodePool"`

    // ProviderIDList contains GCE instance provider IDs for nodes in this pool.
    // Format: gce://<project>/<zone>/<instance>
    // Required by the InfraMachinePool v1beta2 contract.
    // Populated by the controller from workload cluster Nodes or GCP Compute API.
    // +optional
    ProviderIDList []string `json:"providerIDList,omitempty"`
}

type GCPKCCManagedMachinePoolStatus struct {
    // Initialization contains fields per the v1beta2 InfraMachinePool contract.
    // +optional
    Initialization *GCPKCCManagedMachinePoolInitializationStatus `json:"initialization,omitempty"`

    // Ready is a v1beta1 compatibility field.
    // Deprecated: will be removed ~August 2026.
    // +optional
    Ready bool `json:"ready,omitempty"`

    // Replicas is the most recently observed replica count.
    // Required by the InfraMachinePool v1beta2 contract.
    // +optional
    Replicas int32 `json:"replicas,omitempty"`

    // ReadyReplicas is the number of replicas that are ready (fully running nodes).
    // Derived from GKE node pool instance status, not blindly equal to Replicas.
    // +optional
    ReadyReplicas int32 `json:"readyReplicas,omitempty"`

    // Version is the observed Kubernetes version of the node pool.
    // +optional
    Version string `json:"version,omitempty"`

    // NodePoolName is the name of the created ContainerNodePool resource.
    // +optional
    NodePoolName string `json:"nodePoolName,omitempty"`

    // Conditions represents the observations of the machine pool's current state.
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type GCPKCCManagedMachinePoolInitializationStatus struct {
    // Provisioned is true when the node pool infrastructure is fully provisioned.
    // Required by the InfraMachinePool v1beta2 contract.
    // +optional
    Provisioned *bool `json:"provisioned,omitempty"`
}
```

**CAPI contract compliance:**

| Contract requirement | Implementation |
|---|---|
| `spec.providerIDList` ([]string) | ✅ Required — populated from workload cluster Node objects |
| `status.replicas` (int32) | ✅ Required — from ContainerNodePool status |
| `status.initialization.provisioned` (*bool) | ✅ Required — set true when ContainerNodePool `Ready=True` |
| `status.ready` | ✅ v1beta1 compat |
| `status.conditions[Ready]` | ✅ Mirrored to MachinePool `InfrastructureReady` |
| `status.conditions[Paused]` | ✅ Set when paused |
| Finalizer | ✅ `gcpkccmanagedmachinepool.infrastructure.cluster.x-k8s.io` |

**ProviderIDList population:** GKE node pools don't expose instance providerIDs through the KCC ContainerNodePool status. To populate `spec.providerIDList`, the controller lists `Node` objects in the **workload cluster** once it is reachable and reads `node.Spec.ProviderID` (format: `gce://<project>/<zone>/<instance>`). This requires the kubeconfig secret to be available first — creating a natural ordering: ControlPlane ready → kubeconfig available → MachinePool can populate providerIDs.

### Template Types (for ClusterClass support)

Each resource type has a corresponding template type for ClusterClass compatibility:
- `GCPKCCManagedClusterTemplate` (`gcpkccmct`) / `GCPKCCManagedClusterTemplateList`
- `GCPKCCManagedControlPlaneTemplate` (`gcpkccmcpt`) / `GCPKCCManagedControlPlaneTemplateList`
- `GCPKCCManagedMachinePoolTemplate` (`gcpkccmmpt`) / `GCPKCCManagedMachinePoolTemplateList`

Templates use SSA dry-run compatible webhooks (controller-runtime `CustomValidator`) per the CAPI ClusterClass contract.

### Defaults and Overrides

Controllers apply two categories of field patching at reconcile time (after `DeepCopy()`, before `Create()`):

**Defaults** are applied only when the field is empty/nil. User-provided values always win.

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

**Forced overrides** always apply — CAPI is the source of truth for these fields.

| Source (CAPI) | Destination (KCC) | Resource | Notes |
|---------------|-------------------|----------|-------|
| `Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]` | `spec.secondaryIpRange[pods].ipCidrRange` | ComputeSubnetwork | |
| `Cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]` | `spec.secondaryIpRange[services].ipCidrRange` | ComputeSubnetwork | |
| `GCPKCCManagedControlPlane.Spec.Version` | `spec.minMasterVersion` | ContainerCluster | |
| `MachinePool.Spec.Replicas` | `spec.initialNodeCount` | ContainerNodePool | For initial sizing; autoscaling manages after |
| `MachinePool.Spec.Template.Spec.Version` | `spec.version` | ContainerNodePool | When set |
| `MachinePool.Spec.FailureDomains` | `spec.nodeLocations` | ContainerNodePool | When set |

**Not defaulted** (user must always provide):

| Field | Why |
|-------|-----|
| `ComputeSubnetwork.spec.ipCidrRange` | User's CIDR choice |
| `ComputeSubnetwork.spec.region` | User's region choice |

This reduces the minimal viable YAML to:

```yaml
# GCPKCCManagedCluster — only subnet CIDR and region required
spec:
  network: {}
  subnetwork:
    ipCidrRange: "10.0.0.0/20"
    region: us-central1

# GCPKCCManagedControlPlane — just version (location defaulted from subnet region)
spec:
  containerCluster: {}
  version: "1.31"

# GCPKCCManagedMachinePool — just machine type (name, clusterRef, location defaulted; replicas/version from MachinePool)
spec:
  nodePool:
    machineType: e2-medium
```

### ClusterClass Support

Because the API types use concrete KCC Go types (not opaque `runtime.RawExtension`), controller-gen produces full CRD schemas for all nested KCC fields. This enables first-class ClusterClass support:

**Variables** can target any KCC field with schema validation:
```yaml
variables:
- name: region
  schema:
    openAPIV3Schema:
      type: string  # validated against ContainerClusterSpec.Location (string)
```

**Patches** navigate into the typed intermediate structs with validated paths:
```yaml
patches:
- definitions:
  - selector:
      kind: GCPKCCManagedControlPlaneTemplate
    jsonPatches:
    - op: replace
      path: /spec/template/spec/containerCluster/location  # schema knows this is a string
      valueFrom:
        variable: region
```

Three template flavors are provided:

| Flavor | File | Use case |
|--------|------|----------|
| Simple | `cluster-template-gke-kcc.yaml` | Direct resource creation, no topology |
| ClusterClass | `cluster-template-gke-kcc-clusterclass.yaml` | Reusable class with `project`, `region`, `machineType` variables |
| Topology | `cluster-template-gke-kcc-topology.yaml` | Topology-based Cluster referencing the ClusterClass |

## Controller Design

### Reconciliation Order

The controllers have a natural dependency chain that must be respected:

```
1. GCPKCCManagedCluster     → creates network + subnetwork
2. GCPKCCManagedControlPlane → creates ContainerCluster (refs network/subnet)
                              → writes kubeconfig secret
3. GCPKCCManagedMachinePool         → creates ContainerNodePool (refs cluster)
                              → populates providerIDList from workload cluster Nodes
```

### GCPKCCManagedCluster Controller

**Responsibilities:**
1. Check feature gate `ConfigConnector` at registration in `main.go` (matching existing GKE pattern — not in Reconcile())
2. Check KCC CRDs are installed (`verifyKCCCRDs` in `SetupWithManager`)
3. Check `cluster.x-k8s.io/managed-by` label — skip if externally managed
4. Add finalizer
5. Create `ComputeNetwork` from `spec.network` — convert intermediate type to unstructured KCC resource, set namespace (same as owner) and owner ref
6. Create `ComputeSubnetwork` from `spec.subnetwork`, patching secondary IP ranges from `Cluster.Spec.ClusterNetwork`
7. Watch owned ComputeNetwork and ComputeSubnetwork via `Owns()` for immediate status propagation (no polling)
8. Check readiness via `isKCCConditionTrue` on watched resource status
9. Update `status.initialization.provisioned` and conditions
10. Set reconciliation timeout condition: if resources not Ready within 30m, set `Degraded` condition

**Update strategy:** Use `Patch` (strategic merge patch) for reconciling existing KCC resources. CAPG-managed fields (forced overrides) are re-applied each reconcile; user-managed fields are preserved. Forced overrides that change a user-provided value are documented but do not emit events.

**Deletion:** The controller checks for KCC resource absence before removing the finalizer. Owner references on KCC resources enable cascaded GC. Sets `DeletionBlocked` condition with actionable message after 30m timeout.

### GCPKCCManagedControlPlane Controller

**Responsibilities:**
1. Check feature gate (at registration in `main.go`, matching existing GKE pattern), KCC CRDs, pause, externally-managed
2. Add finalizer
3. Gate on `GCPKCCManagedCluster.status.initialization.provisioned = true`
4. Create `ContainerCluster` from `spec.containerCluster` (convert intermediate type to unstructured KCC resource)
5. Watch owned ContainerCluster via `Owns()` for immediate status propagation (no polling)
6. Extract endpoint and CA cert from ContainerCluster status
7. Generate kubeconfig using existing GKE auth pattern (reuse from current GKE provider)
8. Set `status.externalManagedControlPlane = true`, `status.initialization.controlPlaneInitialized`
9. Set reconciliation timeout condition: if ContainerCluster not Ready within 30m, set `Degraded` condition

**Deletion:** Checks ContainerCluster is gone before removing finalizer. Sets `DeletionBlocked` condition with actionable message after 30m timeout.

### GCPKCCManagedMachinePool Controller

**Responsibilities:**
1. Check feature gate (at registration in `main.go`), KCC CRDs, pause
2. Add finalizer
3. Gate on `GCPKCCManagedControlPlane.status.initialization.controlPlaneInitialized = true`
4. Create `ContainerNodePool` from `spec.nodePool` (convert intermediate type to unstructured KCC resource)
5. Watch owned ContainerNodePool via `Owns()` for immediate status propagation and real-time `status.replicas` updates
6. Populate `spec.providerIDList` — investigate GCP Compute API (instance group URLs) as primary, workload cluster Node listing as fallback
7. Set `status.initialization.provisioned`, `status.replicas`, `status.readyReplicas`, conditions

**Deletion:** Checks ContainerNodePool is gone before removing finalizer. Sets `DeletionBlocked` condition after 30m timeout.

### RBAC

```yaml
rules:
  - apiGroups: ["compute.cnrm.cloud.google.com"]
    resources: ["computenetworks", "computesubnetworks"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["compute.cnrm.cloud.google.com"]
    resources: ["computenetworks/status", "computesubnetworks/status"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["container.cnrm.cloud.google.com"]
    resources: ["containerclusters", "containernodepools"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["container.cnrm.cloud.google.com"]
    resources: ["containerclusters/status", "containernodepools/status"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Feature Gate

```go
// feature/feature.go
const (
    ConfigConnector featuregate.Feature = "ConfigConnector"
)

var defaultCAPGFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
    ConfigConnector: {Default: false, PreRelease: featuregate.Alpha},
}
```

Enable with: `--feature-gates=ConfigConnector=true`

The feature gate is checked at controller registration time in `main.go` (matching the existing GKE pattern), not in every `Reconcile()` call. If the gate is disabled, the controllers are never registered and impose zero runtime cost.

### KCC Startup Check

Each `SetupWithManager` must verify KCC CRDs are present before registering the controller:

```go
func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, opts controller.Options) error {
    // Check that KCC CRDs are installed
    if err := checkKCCCRDsPresent(ctx, mgr.GetClient()); err != nil {
        return fmt.Errorf("Config Connector CRDs not found: %w. "+
            "Install Config Connector before enabling the ConfigConnector feature gate", err)
    }
    // ...
}
```

### Config Connector Installation

Config Connector must be installed separately by users before enabling the `ConfigConnector` feature gate. CAPG ships `hack/install-config-connector.sh` to automate this. It follows the same credential convention as CAPG: a Kubernetes Secret containing the GCP service account key JSON, referenced via `GOOGLE_APPLICATION_CREDENTIALS` (the standard GCP variable):

```bash
# Install Config Connector operator and configure it
GCP_PROJECT=my-project \
GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa-key.json \
  ./hack/install-config-connector.sh 1.146.0
```

Or when creating a fresh management cluster from scratch:

```bash
# Create kind cluster + CAPI + CAPG + Config Connector in one step
GCP_PROJECT=my-project \
GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa-key.json \
  make create-management-cluster-kcc
```

The script downloads the release bundle from `storage.googleapis.com/configconnector-operator/{version}/release-bundle.tar.gz`, creates a `gcp-key` Secret in `cnrm-system` from the JSON key file, applies a cluster-mode `ConfigConnector` resource pointing to that secret, and waits for controllers to be ready.

The `CONF_CONNECTOR_VER` Makefile variable (default `1.146.0`) controls which version is installed. The `KCC_CREDENTIALS_SECRET` variable overrides the secret name (default: `gcp-key`).

## Implementation Plan

### Phase 1: API Types + Intermediate Type Definitions
- [ ] CAPG-defined intermediate types for ComputeNetwork, ComputeSubnetwork, ContainerCluster, ContainerNodePool (covering common fields + `AdditionalConfig` passthrough)
- [ ] Full CAPI v1beta2 compliant provider types: `GCPKCCManagedCluster`, `GCPKCCManagedControlPlane`, `GCPKCCManagedMachinePool`
- [ ] v1beta2 conditions, CRD labels, template types with short names (`gcpkccmc`, `gcpkccmcp`, `gcpkccmmp`, etc.)
- [ ] `+kubebuilder:printcolumn` markers: Ready, Version, Replicas, Age as appropriate per type
- [ ] Conversion helpers: intermediate types → unstructured KCC resources
- [ ] `make generate` — 6 CRDs with manageable schemas, deepcopy

### Phase 2: GCPKCCManagedCluster Controller + Defaults/Overrides
- [ ] Feature gate at registration in `main.go`, KCC CRD check, externally-managed, pause, finalizer
- [ ] Defaults: `applyNetworkDefaults`, `applySubnetworkDefaults` (inline with controller)
- [ ] Overrides: CAPI CIDR ranges → secondary IP ranges (forced)
- [ ] Creates ComputeNetwork + ComputeSubnetwork (convert intermediate → unstructured, set namespace from owner, set owner ref)
- [ ] Watches owned KCC resources via `Owns()` for immediate status propagation
- [ ] Patches existing KCC resources via strategic merge patch (preserves user-managed fields)
- [ ] `status.initialization.provisioned`, v1beta2 conditions, 30m reconciliation timeout → `Degraded` condition
- [ ] Deletion gate with `DeletionBlocked` condition after 30m timeout

### Phase 3: GCPKCCManagedControlPlane Controller + Defaults/Overrides
- [ ] Feature gate at registration, KCC CRD, externally-managed, pause, finalizer
- [ ] Defaults: `applyContainerClusterDefaults` (inline); region defaulted from subnetwork if not set
- [ ] Overrides: `spec.version` → `minMasterVersion` (forced)
- [ ] Gated on infra cluster provisioned (via `getInfraCluster`)
- [ ] Creates ContainerCluster, kubeconfig generation (reuse existing GKE auth pattern)
- [ ] Watches owned ContainerCluster via `Owns()`
- [ ] `status.initialization.controlPlaneInitialized`, `externalManagedControlPlane = true`
- [ ] 30m reconciliation timeout, deletion timeout

### Phase 4: GCPKCCManagedMachinePool Controller + Defaults/Overrides
- [ ] Feature gate at registration, KCC CRD, pause, finalizer
- [ ] Defaults: `applyContainerNodePoolDefaults` (inline)
- [ ] Overrides: `replicas` → `initialNodeCount`, `version`, `failureDomains` → `nodeLocations` (forced, document autoscaler interaction)
- [ ] Gated on control plane initialized (via `getControlPlane`)
- [ ] Creates ContainerNodePool, watches via `Owns()` for real-time `status.replicas` updates
- [ ] `spec.providerIDList` — investigate GCP Compute API as primary source, workload Node listing as fallback
- [ ] Fetches owner MachinePool via `exputil.GetOwnerMachinePool`
- [ ] 30m reconciliation timeout, deletion timeout

### Phase 5: Tests
- [ ] Pure function tests: `isKCCConditionTrue`, `patchSubnetworkCIDRs`, intermediate → unstructured conversion
- [ ] Defaults/overrides tests: all `apply*` functions (table-driven)
- [ ] Reconciler tests: all 3 controllers (feature gate, CRUD, delete flows, timeout conditions)

### Phase 6: Template Flavors
- [ ] `cluster-template-gke-kcc.yaml` — simple non-topology
- [ ] `cluster-template-gke-kcc-clusterclass.yaml` — ClusterClass with variables + typed patches
- [ ] `cluster-template-gke-kcc-topology.yaml` — topology-based

### Phase 7: Makefile + Installation Script (for development/E2E only)
- [ ] `hack/install-config-connector.sh` with secret-based credentials (development/E2E automation, not for cluster-api-operator installs)
- [ ] Makefile targets: `create-management-cluster-kcc`, `install-config-connector`

### Future Phases

- [ ] Integration tests within existing framework (user-driven initially, automation last)
- [ ] E2E tests (create/scale/upgrade/delete lifecycle)
- [ ] Validation webhooks for inline CC specs
- [ ] Graduation to beta
- [ ] Additional CC resources (CloudSQL, CloudMemorystore, etc.)

## Testing Strategy

### Unit Tests (required for alpha)
- Pure functions: `isKCCConditionTrue`, `patchSubnetworkCIDRs`, all 6 `apply*Default/Override` functions
- Reconciler tests with fake client + typed KCC objects
- Coverage: feature gate, not found, no owner, normal reconcile, readiness gate, delete flows

### Integration Tests (required for beta)
- kind cluster + KCC operator installed
- Full reconciliation loop with real KCC resources against GCP
- CAPI field changes trigger appropriate KCC patches

### E2E Tests (required for beta)
- Complete cluster lifecycle: create, scale node pool, upgrade version, delete
- Verify kubeconfig is generated and the cluster is reachable
- Multiple scenarios: basic cluster, private cluster, autoscaling node pool
- Verify `clusterctl get kubeconfig` works

## Graduation Criteria

### Alpha
- [ ] API types defined and fully CAPI v1beta2 compliant (typed KCC Go types)
- [ ] All three controllers implemented with typed KCC field access
- [ ] Reasonable defaults + CAPI field overrides
- [ ] Unit tests for pure functions, defaults, and reconcilers
- [ ] Feature gate (`ConfigConnector=true`) enforced in all controllers
- [ ] KCC CRD presence check in all `SetupWithManager` methods
- [ ] Kubeconfig generation (gke-gcloud-auth-plugin exec credential)
- [ ] `hack/install-config-connector.sh` + Makefile targets
- [ ] Cluster template flavors: simple, ClusterClass, topology
- [ ] ClusterClass fully functional with typed KCC CRD schemas
- [ ] User guide written
- [ ] Feature gate enabled by default: **NO**

### Beta
- [ ] Integration tests passing
- [ ] E2E tests for common scenarios
- [ ] Production usage feedback
- [ ] Event-driven status watches (not polling)
- [ ] Validation webhooks
- [ ] Feature gate enabled by default: **YES**

### GA
- [ ] Multiple production deployments
- [ ] No major bugs for 2+ releases
- [ ] Comprehensive documentation
- [ ] Feature gate removed (always enabled)

## Risks and Mitigations

### Risk: KCC Not Installed
**Impact**: Controller startup fails or panics
**Mitigation**: CRD presence check in `SetupWithManager` with a clear error message. Controllers that fail this check are not registered — the manager continues running other controllers.

### Risk: kubeconfig Format Compatibility
**Impact**: Generated kubeconfig uses `gke-gcloud-auth-plugin` exec credential, which requires the plugin to be installed on the user's machine
**Mitigation**: Document the requirement. Consider also generating a service-account-token-based kubeconfig as a fallback for tooling that doesn't support exec credentials.

### Risk: providerIDList Population
**Impact**: GCPKCCManagedMachinePool needs instance provider IDs for CAPI contract compliance
**Mitigation**: Investigate GCP Compute API (instance group URLs from node pool) as primary source — this works even when the workload cluster is unreachable. Fall back to workload cluster Node listing if Compute API is insufficient. Gate on kubeconfig secret availability for fallback path. CAPI tolerates an empty providerIDList during provisioning.

### Risk: Two Ways to Create GKE Clusters
**Impact**: User confusion between standard CAPG GKE path and KCC path
**Mitigation**: The two paths permanently coexist. Clear documentation with a comparison table and decision framework ("use KCC path if you need full GKE API surface; use existing path for simpler setups"). Each path has a distinct set of CRD kinds — there is no overlap or ambiguity at runtime.

### Risk: KCC Resource Reconciliation Timeout
**Impact**: KCC resources can take 10-30 minutes to reconcile; if KCC gets wedged (IAM issues, quota, API bugs), CAPG controllers poll forever
**Mitigation**: 30-minute reconciliation timeout. If a KCC resource is not Ready within 30m, controllers set a `Degraded` condition with the last KCC status message. During deletion, set `DeletionBlocked` condition after 30m with actionable guidance.

## Alternatives Considered

### Alternative 1: Generic `[]runtime.RawExtension` List (CAPZ/ASO pattern)
```go
type Spec struct {
    Resources []runtime.RawExtension `json:"resources"`
}
```
**Pros**: Maximum flexibility; users can add any CC resource
**Cons**: No type safety; CAPG can't know which resource is the network vs the cluster; harder to document and validate; harder to enforce patching rules
**Decision**: Rejected in favor of named typed fields.

### Alternative 2: `runtime.RawExtension` per Named Field (original design)
```go
type Spec struct {
    Cluster runtime.RawExtension `json:"cluster"` // +kubebuilder:validation:XEmbeddedResource
}
```
**Pros**: No KCC Go dependency; zero version coupling; maximum forward compatibility
**Cons**: CRD schema is opaque — ClusterClass variable patches have no schema validation; `kubectl explain` shows nothing; users get no admission-time validation on the embedded resource structure
**Decision**: Rejected. Analysis showed that KCC's generated API type packages (`pkg/clients/generated/apis/`) import only standard k8s types (no GCP client libraries), so the dependency footprint is lightweight. The ClusterClass usability gain outweighs the version coupling concern.

### Alternative 3: KCC Go Types per Named Field
```go
type Spec struct {
    Cluster kcccontainerv1beta1.ContainerCluster `json:"cluster"`
}
```
**Pros**: Full CRD schema generated by controller-gen → ClusterClass patches validated → `kubectl explain` works; typed controller code; compile-time safety
**Cons**: CRDs become enormous (500KB-1MB+ per CRD embedding full KCC types); couples CAPG to KCC's ALPHA Go module; `allowDangerousTypes=true` required globally; KCC monorepo dependency may conflict with existing CAPG deps
**Decision**: Rejected. CRD size risk and ALPHA Go client coupling are too high. Intermediate types provide ClusterClass validation for common fields without these risks.

### Alternative 4: Reference-Only Pattern
```go
type Spec struct {
    NetworkRef *ObjectReference    `json:"networkRef"`
    SubnetworkRef *ObjectReference `json:"subnetworkRef"`
}
```
**Pros**: Users create KCC resources separately; cleaner separation
**Cons**: CAPG can't patch CAPI-derived fields (CIDRs, versions) onto user-created resources; no lifecycle management
**Decision**: Rejected. Patching CAPI fields onto CC resources is the core value proposition.

### Alternative 5 (adopted): CAPG-Defined Intermediate Types per Named Field
```go
type GCPKCCManagedControlPlaneSpec struct {
    ContainerCluster GCPKCCContainerClusterSpec `json:"containerCluster"`
}

type GCPKCCContainerClusterSpec struct {
    // Common typed fields — validated by CRD schema, patchable by ClusterClass
    Location        *string `json:"location,omitempty"`
    NetworkingMode  *string `json:"networkingMode,omitempty"`
    // ... other commonly-used fields

    // AdditionalConfig allows setting any KCC ContainerCluster field not covered above.
    // Merged into the KCC resource at reconcile time.
    // +optional
    AdditionalConfig *runtime.RawExtension `json:"additionalConfig,omitempty"`
}
```
**Pros**: Manageable CRD sizes; no KCC Go module dependency; ClusterClass patches validated for common fields; `kubectl explain` works for common fields; advanced KCC fields accessible via `AdditionalConfig` passthrough; no `allowDangerousTypes` needed
**Cons**: Common fields must be manually curated — new KCC fields require a CAPG PR to add to the intermediate type (same as the existing GKE path, but for a smaller set of fields). `AdditionalConfig` fields are not schema-validated.
**Decision**: Adopted. Best balance of ClusterClass usability, CRD size, and dependency hygiene. The `AdditionalConfig` passthrough preserves access to KCC's full API surface for power users.

## References

- [CAPI Provider Contracts](https://cluster-api.sigs.k8s.io/developer/providers/contracts/overview)
- [CAPZ Azure Service Operator Proposal](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/main/docs/proposals/20230123-azure-service-operator.md)
- [Config Connector Documentation](https://cloud.google.com/config-connector/docs/overview)
- [Cluster API Book](https://cluster-api.sigs.k8s.io/)
- [GKE API Reference](https://cloud.google.com/kubernetes-engine/docs/reference/rest)

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 2 | ✅ Proposal revised | Phase ordering fixed, coexistence clarified, timeouts added |
| Adversarial Review | `/codex review` | Independent 2nd opinion | 1 | ✅ Proposal revised | CRD size → intermediate types, providerIDList investigation, auth clarified |
| Eng Review | `/plan-eng-review` | Architecture & tests | 1 | ⚠️ Timed out | Covered by adversarial review |
| Design Review | `/plan-design-review` | API ergonomics | 1 | ✅ Proposal revised | Renamed MachinePool, added short names, containerCluster field, printcolumns |

**Key changes from review round 2:**
- Switched from full KCC Go type embedding to CAPG-defined intermediate types (CRD size + dependency risk)
- Merged Phase 8 (defaults/overrides) into Phases 2-4 (they are core controller logic)
- Renamed `GCPKCCMachinePool` → `GCPKCCManagedMachinePool` (consistency)
- Renamed `spec.cluster` → `spec.containerCluster` (disambiguation)
- Added short names, print columns for all types
- Feature gate at registration time (`main.go`), not in `Reconcile()`
- Watches via `Owns()` instead of 30s polling
- 30m reconciliation/deletion timeouts with `Degraded`/`DeletionBlocked` conditions
- Namespace-level project via KCC `ConfigConnectorContext`
- Strategic merge patch for updates
- Permanent coexistence with existing GKE path (not a replacement)

**VERDICT:** DESIGN REVISED — Ready for implementation. See TODOS.md for phased plan.

---

## Addendum: Post-Implementation Review Feedback

After initial implementation and review, the following changes are required. These apply broadly across all files.

### A1. Use upstream CAPI v1beta2 condition constants

**Problem:** Custom condition reasons like `WaitingForKCCInfraClusterReason`, `WaitingForKCCControlPlaneReason`, `KCCResourceDeletingReason` duplicate upstream CAPI constants.

**Change:** Replace custom reasons with upstream `clusterv1` constants from `sigs.k8s.io/cluster-api/api/core/v1beta2`:
- `WaitingForKCCInfraClusterReason` → `clusterv1.WaitingForClusterInfrastructureReadyReason`
- `WaitingForKCCControlPlaneReason` → `clusterv1.WaitingForControlPlaneInitializedReason`
- `KCCResourceDeletingReason` → `clusterv1.DeletingReason`
- `KCCResourceDeletedReason` → `clusterv1.DeletionCompletedReason`
- `KCCDeletionTimeoutReason` → keep (no CAPI equivalent)
- `KCCReconciliationTimeoutReason` → keep (no CAPI equivalent)
- `KCCResourceCreatingReason` → keep (KCC-specific lifecycle state)
- `KCCResourceReadyReason` → `clusterv1.ReadyReason`
- `KCCResourceNotReadyReason` → `clusterv1.NotReadyReason`

KCC-specific condition *types* (`KCCNetworkReadyCondition`, `KCCSubnetworkReadyCondition`, `KCCClusterReadyCondition`, `KCCNodePoolReadyCondition`) remain — CAPI has no GCP sub-resource equivalents.

**Files:** `exp/api/v1beta1/gcpkcc_conditions.go`, all 3 controllers.

### A2. Reduce intermediate types to controller-only fields, use RawExtension for KCC specs

**Problem:** The intermediate types duplicate many KCC fields as typed Go structs (e.g., `KCCReleaseChannel`, `KCCPrivateClusterConfig`, `KCCNodePoolAutoscaling`). These are never used by controller defaults/overrides logic. The original concern was that ClusterClass patches would not work with `runtime.RawExtension` — but this is incorrect.

**Analysis:** ClusterClass JSON patches operate on raw JSON at runtime via the `evanphx/json-patch` library (`sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/engine.go`). The ClusterClass webhook (`sigs.k8s.io/cluster-api/internal/webhooks/patch_validation.go`, `validateJSONPatches` line 320) only validates that:
1. `op` is `add`/`replace`/`remove`
2. `path` starts with `/spec`
3. Array index restrictions
4. Variable/template syntax

**It does NOT validate paths against the CRD schema.** A patch like `/spec/template/spec/containerCluster/spec/releaseChannel/channel` works identically whether `releaseChannel` is a typed field or inside a `RawExtension`. The only difference is `kubectl explain` discoverability and apiserver-level field validation on create/update.

**Change:** Make each KCC resource spec a `*runtime.RawExtension` instead of a large typed struct. Only keep typed fields that the controller must read/write for defaults and overrides:

```go
type GCPKCCContainerClusterResource struct {
    Metadata metav1.ObjectMeta           `json:"metadata,omitempty"`
    Spec     *runtime.RawExtension       `json:"spec,omitempty"`
}
```

The controller applies defaults/overrides by:
1. Unmarshalling `Spec` into `map[string]interface{}`
2. Setting default values for missing keys (e.g., `location`, `initialNodeCount`, `networkRef`)
3. Force-overriding CAPI fields (e.g., `minMasterVersion` from version)
4. Marshalling back and passing to the KCC unstructured builder

This eliminates all intermediate Go types except:
- `KCCResourceRef` (used in defaults for `networkRef`, `subnetworkRef`, `clusterRef`)
- `KCCSecondaryIPRange` (used in CIDR override logic)
- `KCCIPAllocationPolicy` (used in CIDR override logic)

The conversion layer simplifies dramatically — instead of mapping 30+ typed fields to `map[string]interface{}`, it just unmarshals the `RawExtension` directly and merges any controller-applied overrides.

**Impact on ClusterClass:** No impact. Users write the same JSON patch paths targeting KCC field names (e.g., `/spec/template/spec/containerCluster/spec/releaseChannel/channel`). These work because JSON patches operate on the raw JSON document regardless of Go type structure.

**Impact on CRD size:** Significant reduction. Each KCC resource spec becomes a single opaque object field instead of 20+ typed fields.

**Files:** `exp/api/v1beta1/gcpkcc_types.go`, `exp/api/v1beta1/gcpkcc_conversion.go`, all controllers, defaults, template types.

### A3. Default namespace on KCC resources

**Problem:** KCC resources need a namespace set but it was not defaulted.

**Change:** In each `apply*Defaults` function, add:
```go
if res.Metadata.Namespace == "" {
    res.Metadata.Namespace = ownerNamespace
}
```

**Files:** `exp/controllers/gcpkcc_defaults.go`.

### A4. Define KCC GVK constants once

**Problem:** KCC GVK strings (`compute.cnrm.cloud.google.com`, `container.cnrm.cloud.google.com`) appear in both `gcpkcc_helpers.go` (as `var` constants) and `gcpkcc_conversion.go` (as inline strings).

**Change:** Move GVK constants to `exp/api/v1beta1/gcpkcc_conversion.go` (API package — accessible to both API and controllers). Reference them from `gcpkcc_helpers.go` and all controllers via the `infrav1exp` import.

**Files:** `exp/api/v1beta1/gcpkcc_conversion.go`, `exp/controllers/gcpkcc_helpers.go`.

### A5. Use `metav1.ObjectMeta` for KCC resource metadata

**Problem:** Custom `KCCObjectMeta` struct duplicates standard Kubernetes metadata fields.

**Change:** Replace `KCCObjectMeta` with `metav1.ObjectMeta` from `k8s.io/apimachinery/pkg/apis/meta/v1`. This is the standard Kubernetes type that users already know. While it exposes extra fields (`UID`, `ResourceVersion`, etc.), these are ignored at conversion time — only `Name`, `Namespace`, `Annotations`, `Labels` are used.

Update conversion functions to read from `metav1.ObjectMeta` fields instead of `KCCObjectMeta`.

**Files:** `exp/api/v1beta1/gcpkcc_types.go`, `exp/api/v1beta1/gcpkcc_conversion.go`, all controllers, all defaults.

### A6. Simplify and deduplicate `deleteKCCResourceIfExists`

**Problem:** The function is defined as a method 3 times (once per controller), and the get-then-delete pattern is unnecessary.

**Change:**
1. Rename to `deleteResource` (generic — works for any unstructured resource)
2. Simplify: just call `Delete()` and handle `IsNotFound` error (no need to `Get` first)
3. Move to `gcpkcc_helpers.go` as a standalone function: `func deleteResource(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, name, namespace string) error`
4. Remove the 3 method definitions from controllers

**Files:** `exp/controllers/gcpkcc_helpers.go`, all 3 controllers.

### A7. Kubeconfig: use bearer token from CAPG GCP credentials

**Problem:** Current implementation uses `gke-gcloud-auth-plugin` exec credential which requires the plugin installed on the user's machine and doesn't work for CAPI infrastructure.

**Change:** Adapt the existing CAPG kubeconfig generation pattern from `cloud/services/container/clusters/kubeconfig.go`:
1. Read GCP service account credentials from the CAPG credentials Secret (same secret the existing provider uses, referenced via the management cluster configuration)
2. Create an IAM Credentials API client
3. Call `GenerateAccessToken` to get an OAuth2 bearer token
4. Build kubeconfig with `Token` field (not `Exec`)
5. Store in `<cluster>-kubeconfig` secret per CAPI convention

Create a lightweight credential helper in `exp/controllers/gcpkcc_credentials.go` that:
- Reads SA key JSON from a Kubernetes Secret or falls back to Application Default Credentials
- Creates an `IamCredentialsClient`
- Exposes `GenerateAccessToken(ctx, clientEmail)` method

This avoids the full scope pattern while reusing the proven token generation flow.

**Files:** new `exp/controllers/gcpkcc_credentials.go`, `exp/controllers/gcpkccmanagedcontrolplane_controller.go`.

### A8. Status.replicas from KCC nodeCount with state-into-spec: merge

**Problem:** `status.replicas` was set from `MachinePool.Spec.Replicas` (desired) rather than actual node count from GCP.

**Change:**
1. Set annotation `cnrm.cloud.google.com/state-into-spec: merge` on ContainerNodePool KCC resources (in conversion or defaults)
2. Read `spec.initialNodeCount` from the KCC ContainerNodePool unstructured resource (KCC populates this from GCP state via the merge annotation)
3. Use that value for `status.replicas`

**Limitation (document):** With autoscaling enabled, `initialNodeCount` may not reflect the actual current node count. This is a known KCC limitation. For accurate counts with autoscaling, a future enhancement could read instance group sizes via the GCP Compute API.

**Files:** `exp/controllers/gcpkccmanagedmachinepool_controller.go`, `exp/controllers/gcpkcc_defaults.go` or `gcpkcc_conversion.go` (for the annotation).

### A9. Whole KCC resource as `*runtime.RawExtension`

**Problem:** A2 kept `Metadata metav1.ObjectMeta` typed while making `Spec *runtime.RawExtension`. This splits the KCC resource across two fields and requires a `namespace` parameter in `ToUnstructured*` functions even though defaults already set `Metadata.Namespace`.

**Change:** Make each KCC resource field a single `*runtime.RawExtension` containing the full Kubernetes-style object (`metadata` + `spec`). The resource wrapper types (`GCPKCCNetworkResource`, etc.) are deleted. Parent spec types become:

```go
type GCPKCCManagedClusterSpec struct {
    // +optional
    // +kubebuilder:pruning:PreserveUnknownFields
    // +kubebuilder:validation:Schemaless
    Network *runtime.RawExtension `json:"network,omitempty"`
    // ...same for Subnetwork
}
```

Replace the 4 specific `ToUnstructured*` functions with a single generic:
```go
func ToUnstructured(raw *runtime.RawExtension, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error)
```

Add raw metadata helpers (`getRawMetadataName`, `setRawMetadataName`, `getRawAnnotations`, etc.) for defaults to use.

**Files:** `exp/api/v1beta1/gcpkcc_types.go`, `exp/api/v1beta1/gcpkcc_conversion.go`, all controllers, defaults, template types, tests.

### A10. Merge readiness check, use standard Ready condition

**Problem:** `isKCCResourceReady` and `getKCCConditionMessage` are always called together. Four custom condition types (`KCCNetworkReadyCondition`, etc.) are unnecessary — the CAPI contract only requires a single `Ready` condition with descriptive messages.

**Change:**
1. Merge into `getKCCReadiness(obj) (ready bool, message string)`
2. Drop `KCCNetworkReadyCondition`, `KCCSubnetworkReadyCondition`, `KCCClusterReadyCondition`, `KCCNodePoolReadyCondition`
3. Use standard `"Ready"` condition type with messages like "KCC ComputeNetwork is ready" or "Waiting for KCC ContainerCluster: <kcc-message>"
4. Keep `status.ready` bool field for v1beta1 compat per CAPI contract

**Files:** `exp/api/v1beta1/gcpkcc_conditions.go`, `exp/controllers/gcpkcc_helpers.go`, all 3 controllers.

### A11. Defaults take parent objects

**Problem:** `applyContainerClusterDefaults` takes 6 string parameters. Verbose and error-prone.

**Change:** Replace per-resource standalone functions with per-controller functions taking parent objects:
```go
func applyClusterDefaults(kccCluster *infrav1exp.GCPKCCManagedCluster, cluster *clusterv1.Cluster) error
func applyControlPlaneDefaults(kccCP *infrav1exp.GCPKCCManagedControlPlane, cluster *clusterv1.Cluster, infraCluster *infrav1exp.GCPKCCManagedCluster) error
func applyMachinePoolDefaults(kccMMP *infrav1exp.GCPKCCManagedMachinePool, machinePool *clusterv1.MachinePool, controlPlane *infrav1exp.GCPKCCManagedControlPlane) error
```

Override functions are integrated into the same functions (defaults + overrides in one pass).

**Files:** `exp/controllers/gcpkcc_defaults.go`, all 3 controllers, tests.

### A12. Rename helper, document defer pattern

**Problem:** `getKCCStatusField` is generic but has KCC prefix. The defer status patch pattern is not documented.

**Change:**
1. Rename `getKCCStatusField` → `getStatusFieldFromUnstructured`
2. Add comment explaining the defer status patch pattern in all 3 controllers

**Files:** `exp/controllers/gcpkcc_helpers.go`, `exp/controllers/gcpkccmanagedcontrolplane_controller.go`.

### A13. ClusterClass template: add variables and JSON patches

**Problem:** `cluster-template-gke-kcc-clusterclass.yaml` defines a ClusterClass but has no `variables` or `patches` sections. Without patches, topology variables (region, machineType, replicas) cannot be plumbed through.

**Change:** Add variables (`region`, `machineType`, `kubernetesVersion`) and JSON patches to the ClusterClass definition targeting the template specs.

**Files:** `templates/cluster-template-gke-kcc-clusterclass.yaml`, `templates/cluster-template-gke-kcc-topology.yaml`.

### A14. Nest ConfigConnector under GKE feature gate

**Problem:** `ConfigConnector` is a standalone feature gate but semantically depends on GKE. The MachinePool controller should also require the `MachinePool` feature gate.

**Change:** Nest `ConfigConnector` check inside the `GKE` gate. Nest `GCPKCCManagedMachinePoolReconciler` inside an additional `MachinePool` gate check:
```go
if feature.Gates.Enabled(feature.GKE) {
    // ...existing GKE controllers...
    if feature.Gates.Enabled(feature.ConfigConnector) {
        // KCCManagedCluster, KCCManagedControlPlane
        if feature.Gates.Enabled(capifeature.MachinePool) {
            // KCCManagedMachinePool
        }
    }
}
```

**Files:** `main.go`.

## Addendum: E2E Testing Findings (A15-A20)

After end-to-end testing with kind + Tilt + Config Connector against a live GCP project, the following changes were required:

### A15. Server-side apply for KCC resources

**Problem:** `createOrPatchKCCResource` used `client.MergeFrom` which computes a diff and removes fields not in the desired state. KCC-managed immutable fields like `spec.resourceID` caused admission webhook rejections on re-reconcile.

**Change:** Replace `createOrPatchKCCResource` with `applyKCCResource` using server-side apply (`client.Apply` + `client.FieldOwner`). This only sends CAPG-managed fields and leaves KCC-managed fields untouched. Handles both create and update in one call.

**Files:** `exp/controllers/gcpkcc_helpers.go`, all 3 controllers.

### A16. Fix ContainerCluster defaults

**Problem:** Multiple issues found during E2E:
- `spec.removeDefaultNodePool` is not a valid KCC spec field — it's an annotation (`cnrm.cloud.google.com/remove-default-node-pool`)
- `spec.ipAllocationPolicy` was missing but required for VPC_NATIVE clusters
- `spec.initialNodeCount` must be forced to 1 (GKE requires >= 1 at creation; the default node pool is removed via annotation)
- `spec.networkingMode` should be explicitly set to `VPC_NATIVE`

**Change:**
- Remove `removeDefaultNodePool` from spec defaults (already set as annotation)
- Add `ipAllocationPolicy` default when `networkingMode` is `VPC_NATIVE`
- Force `initialNodeCount` to 1
- Add `state-into-spec: merge` annotation on ContainerCluster so KCC populates spec fields from GCP state

**Files:** `exp/controllers/gcpkcc_defaults.go`.

### A17. Fix kubeconfig generation

**Problem:** CA certificate was not found at `status.masterAuth.clusterCaCertificate` — KCC places it under `status.observedState.masterAuth.clusterCaCertificate`. Additionally, the control plane was marked `Ready=true` before kubeconfig generation succeeded.

**Change:**
- Fix CA cert path to `status.observedState.masterAuth.clusterCaCertificate`
- Gate `Ready=true` on successful kubeconfig creation — requeue if endpoint or CA cert not yet available

**Files:** `exp/controllers/gcpkccmanagedcontrolplane_controller.go`.

### A18. Node pool location from live ContainerCluster

**Problem:** Node pool defaults tried to read location from `controlPlane.Spec.ContainerCluster` (user's raw JSON) which didn't contain the defaulted location. The location only existed on the live KCC ContainerCluster object.

**Change:** The machine pool controller fetches the live KCC ContainerCluster and passes it to `applyMachinePoolDefaults`, which reads `spec.location` from it (populated via `state-into-spec: merge`).

**Files:** `exp/controllers/gcpkcc_defaults.go`, `exp/controllers/gcpkccmanagedmachinepool_controller.go`.

### A19. Minimal templates with resourceID

**Problem:** Templates duplicated fields that already have defaults (networkRef, subnetworkRef, initialNodeCount, removeDefaultNodePool, etc.).

**Change:** Strip templates to only user-required fields (project annotation, CIDR, region, version, machineType). Add `spec.resourceID` to network, subnetwork, and ContainerCluster templates for BYOI (Bring Your Own Infrastructure) support.

**Files:** `templates/cluster-template-gke-kcc.yaml`.

### A20. Tilt and install-config-connector improvements

**Problem:** Multiple issues in the local dev workflow:
- Config Connector operator is a StatefulSet, not a Deployment (install script waited on wrong resource type)
- KCC webhook pods OOM with default 128Mi in kind clusters
- The ConfigConnector CR schema rejected `googleServiceAccount: ""` with `credentialSecretName` (oneOf validation)
- KCC CRDs missing from kustomize config
- Config Connector must be installed before CAPG (KCC CRD check at startup)
- `MachinePool` feature gate missing from CAPG args in tilt-settings

**Change:**
- Fix install script: StatefulSet rollout status, remove invalid `googleServiceAccount` field
- Use `ControllerResource` CRD to set proper memory limits via the operator (not kubectl patch)
- Add KCC CRDs to `config/crd/kustomization.yaml`
- Install Config Connector before CAPG in Tiltfile
- Add `MachinePool=true` to CAPG feature gates in tilt-settings
- Decode credentials via python3 to avoid shell quoting issues

**Files:** `hack/install-config-connector.sh`, `Tiltfile`, `tilt-settings.json`, `config/crd/kustomization.yaml`.

### Known limitations (for future work)

- **`providerIDList` not populated:** CAPI MachinePool stays in `ScalingUp` phase because `spec.providerIDList` is empty. Requires reading GCP Compute API instance group URLs or workload cluster Node listing.
- **`readyReplicas` not set:** Needed for CAPI to consider MachinePool fully provisioned.
- **`status.failureDomains` not set:** The infra cluster should expose available zones from the ContainerCluster's `spec.nodeLocations` (via state-into-spec merge) as `status.failureDomains` per CAPI contract.
- **Cluster phase stuck at `Provisioning`:** Related to missing providerIDList — CAPI doesn't transition to `Provisioned` until MachinePool reports ready replicas correctly.
