# Config Connector Integration Proposal

## Summary

This proposal introduces Config Connector (KCC) integration in cluster-api-provider-gcp (CAPG), enabling users to manage GKE clusters through Config Connector resources while maintaining full Cluster API (CAPI) v1beta2 contract compliance. The design adds new provider types (`GCPKCCManagedCluster`, `GCPKCCManagedControlPlane`, `GCPKCCMachinePool`) as a parallel path alongside the existing GKE provider — users choose one or the other, not a migration.

## Motivation

### Goals

1. **Enable Advanced GKE Features**: Provide access to the full GKE API surface through Config Connector — Binary Authorization, Security Posture, Managed Prometheus, and hundreds of other fields that CAPG does not and should not expose directly
2. **Respect all CAPI v1beta2 Contracts**: Full compliance with the [CAPI provider contracts](https://cluster-api.sigs.k8s.io/developer/providers/contracts/overview) — all required fields, status fields, conditions, labels, and kubeconfig conventions
3. **Stronger Typing than CAPZ/ASO**: Use named typed fields (`spec.network`, `spec.cluster`, `spec.nodePool`) rather than a generic untyped `[]runtime.RawExtension` list, so each resource role is explicit in the API
4. **Minimize Field Duplication**: Leverage existing CAPI fields (`spec.version`, `spec.replicas`, cluster network CIDRs) instead of re-declaring them; CAPG only patches the CAPI-derived fields
5. **Maintain CAPI Compatibility**: Ensure full integration with CAPI workflows and tools (`clusterctl`, `kubectl get cluster`, etc.)
6. **Support GitOps**: Enable Kubernetes-native, declarative GCP resource management via ArgoCD, Flux, or any GitOps tool
7. **Simplify Maintenance**: Reduce CAPG code by delegating GCP resource management to Config Connector

### Non-Goals

1. Support for self-managed (non-GKE) clusters
2. Automatic migration from existing CAPG clusters to the KCC path
3. Config Connector installation automation — KCC is a user-managed prerequisite (see below)
4. Bundling KCC with CAPG

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
| Resource embedding | `[]runtime.RawExtension` (generic list) | Named typed fields (`spec.network`, `spec.cluster`, etc.) using KCC Go types |
| Resource identity | Users list any resources in any order | Each field has a defined role — network, subnetwork, cluster, node pool |
| CAPI field patching | JSON merge patches via mutator pipeline | Direct typed field access on KCC structs |
| KCC Go dependency | Full ASO Go types imported | KCC generated API types only (`pkg/clients/generated/apis/`); lightweight — no GCP client libs |
| ClusterClass compatibility | Full schema in CRD | Full schema in CRD — ClusterClass patches are validated against KCC type schemas |
| Dependency tracking | Shared `ResourceReconciler` helper | Per-controller sequential reconciliation |

### Architecture

```
Management Cluster
─────────────────────────────────────────────────────
  CAPI Cluster ──── InfrastructureRef ──▶ GCPKCCManagedCluster
                ╰── ControlPlaneRef   ──▶ GCPKCCManagedControlPlane
                                              │
  CAPI MachinePool ─ InfraRef ──▶ GCPKCCMachinePool

  GCPKCCManagedCluster
    └── creates ──▶ ComputeNetwork (KCC)   ──▶ GCP VPC
    └── creates ──▶ ComputeSubnetwork (KCC)──▶ GCP Subnet

  GCPKCCManagedControlPlane
    └── creates ──▶ ContainerCluster (KCC)  ──▶ GKE Control Plane
    └── writes  ──▶ <cluster>-kubeconfig (Secret)

  GCPKCCMachinePool
    └── creates ──▶ ContainerNodePool (KCC) ──▶ GKE Node Pool

  Config Connector controllers watch KCC resources
  and reconcile them to GCP via the GCP API.
─────────────────────────────────────────────────────
```

## API Design

### Key Principles

1. **Named typed fields** for each resource role (`network`, `subnetwork`, `cluster`, `nodePool`) rather than a generic list
2. **KCC Go types** per named field — each field uses the concrete KCC generated type (e.g. `kcccontainerv1beta1.ContainerCluster`), giving full CRD schema, ClusterClass patch validation, and `kubectl explain` support
3. **Lightweight KCC dependency** — imports only `pkg/clients/generated/apis/{compute,container}/v1beta1` which are pure type definitions; no GCP client libraries are pulled in (the heavy deps like BigQuery, Spanner etc. are only in KCC's controller packages, not the API types)
4. **CAPI field minimization** — only fields that CAPG must patch for CAPI compatibility are enforced; everything else is user-controlled via the full KCC type schema
5. **CAPI v1beta2 contract compliance** — all required spec/status fields, conditions, and labels as per the current contract spec

### GCPKCCManagedCluster (InfraCluster)

Implements the [InfraCluster contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster).

```go
type GCPKCCManagedClusterSpec struct {
    // Network is a complete Config Connector ComputeNetwork resource.
    // CAPG creates and manages the lifecycle of this resource.
    // +required
    Network kcccomputev1beta1.ComputeNetwork `json:"network"`

    // Subnetwork is a complete Config Connector ComputeSubnetwork resource.
    // CAPG patches spec.secondaryIpRange from Cluster.Spec.ClusterNetwork.
    // +required
    Subnetwork kcccomputev1beta1.ComputeSubnetwork `json:"subnetwork"`

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
type GCPKCCManagedControlPlaneSpec struct {
    // Cluster is a complete Config Connector ContainerCluster resource.
    // CAPG creates this resource and manages its lifecycle.
    // +required
    Cluster kcccontainerv1beta1.ContainerCluster `json:"cluster"`

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

### GCPKCCMachinePool (InfraMachinePool)

Implements the [InfraMachinePool contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-machine-pool).

```go
type GCPKCCMachinePoolSpec struct {
    // NodePool is a complete Config Connector ContainerNodePool resource.
    // CAPG creates this resource and manages its lifecycle.
    // +required
    NodePool kcccontainerv1beta1.ContainerNodePool `json:"nodePool"`

    // ProviderIDList contains GCE instance provider IDs for nodes in this pool.
    // Format: gce://<project>/<zone>/<instance>
    // Required by the InfraMachinePool v1beta2 contract.
    // Populated by the controller by listing Node objects in the workload cluster.
    // +optional
    ProviderIDList []string `json:"providerIDList,omitempty"`
}

type GCPKCCMachinePoolStatus struct {
    // Initialization contains fields per the v1beta2 InfraMachinePool contract.
    // +optional
    Initialization *GCPKCCMachinePoolInitializationStatus `json:"initialization,omitempty"`

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

type GCPKCCMachinePoolInitializationStatus struct {
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
| Finalizer | ✅ `gcpkccmachinepool.infrastructure.cluster.x-k8s.io` |

**ProviderIDList population:** GKE node pools don't expose instance providerIDs through the KCC ContainerNodePool status. To populate `spec.providerIDList`, the controller lists `Node` objects in the **workload cluster** once it is reachable and reads `node.Spec.ProviderID` (format: `gce://<project>/<zone>/<instance>`). This requires the kubeconfig secret to be available first — creating a natural ordering: ControlPlane ready → kubeconfig available → MachinePool can populate providerIDs.

### Template Types (for ClusterClass support)

Each resource type has a corresponding template type for ClusterClass compatibility:
- `GCPKCCManagedClusterTemplate` / `GCPKCCManagedClusterTemplateList`
- `GCPKCCManagedControlPlaneTemplate` / `GCPKCCManagedControlPlaneTemplateList`
- `GCPKCCMachinePoolTemplate` / `GCPKCCMachinePoolTemplateList`

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
    spec:
      ipCidrRange: "10.0.0.0/20"
      region: us-central1

# GCPKCCManagedControlPlane — just version (location defaulted from subnet region)
spec:
  cluster: {}
  version: "1.31"

# GCPKCCMachinePool — just machine type (name, clusterRef, location defaulted; replicas/version from MachinePool)
spec:
  nodePool:
    spec:
      nodeConfig:
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

**Patches** navigate into the typed KCC structs with validated paths:
```yaml
patches:
- definitions:
  - selector:
      kind: GCPKCCManagedControlPlaneTemplate
    jsonPatches:
    - op: replace
      path: /spec/template/spec/cluster/spec/location  # schema knows this is a string
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
3. GCPKCCMachinePool         → creates ContainerNodePool (refs cluster)
                              → populates providerIDList from workload cluster Nodes
```

### GCPKCCManagedCluster Controller

**Responsibilities:**
1. Check feature gate `ConfigConnector` is enabled
2. Check KCC CRDs are installed (`verifyKCCCRDs` in `SetupWithManager`)
3. Check `cluster.x-k8s.io/managed-by` label — skip if externally managed
4. Add finalizer
5. Create `ComputeNetwork` from `spec.network` (typed `kcccomputev1beta1.ComputeNetwork`, DeepCopy + set namespace/owner ref)
6. Create `ComputeSubnetwork` from `spec.subnetwork` (typed), patching `Spec.SecondaryIpRange` directly from `Cluster.Spec.ClusterNetwork`
7. Check readiness via typed `isKCCConditionTrue(network.Status.Conditions, ReadyConditionType)`
8. Update `status.initialization.provisioned` and conditions

**Deletion:** The controller checks for KCC resource absence before removing the finalizer. Owner references on KCC resources enable cascaded GC.

### GCPKCCManagedControlPlane Controller

**Responsibilities:**
1. Check feature gate, KCC CRDs, pause, externally-managed
2. Add finalizer
3. Gate on `GCPKCCManagedCluster.status.initialization.provisioned = true`
4. Create `ContainerCluster` from `spec.cluster` (typed `kcccontainerv1beta1.ContainerCluster`)
5. Check readiness via typed `isKCCConditionTrue(containerCluster.Status.Conditions, ReadyConditionType)`
6. Extract endpoint from `containerCluster.Status.Endpoint` (typed `*string`)
7. Extract CA cert from `containerCluster.Status.ObservedState.MasterAuth.ClusterCaCertificate` (typed)
8. Generate kubeconfig with `gke-gcloud-auth-plugin` exec credential; write `<cluster>-kubeconfig` secret
9. Set `status.externalManagedControlPlane = true`, `status.initialization.controlPlaneInitialized`

**Deletion:** Checks ContainerCluster is gone before removing finalizer.

### GCPKCCMachinePool Controller

**Responsibilities:**
1. Check feature gate, KCC CRDs, pause
2. Add finalizer
3. Gate on `GCPKCCManagedControlPlane.status.initialization.controlPlaneInitialized = true`
4. Create `ContainerNodePool` from `spec.nodePool` (typed `kcccontainerv1beta1.ContainerNodePool`)
5. Check readiness via typed conditions
6. When kubeconfig available: list workload cluster `Node` objects to populate `spec.providerIDList`
7. Set `status.initialization.provisioned`, `status.replicas`, `status.readyReplicas`, conditions

**Deletion:** Checks ContainerNodePool is gone before removing finalizer.

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

Every controller's `Reconcile()` function must check this gate as step 2 and return early if disabled.

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

### Phase 1: API Types
- [ ] Full CAPI v1beta2 compliant types with typed KCC Go fields
- [ ] v1beta2 conditions, CRD labels, template types
- [ ] KCC Go types from `pkg/clients/generated/apis/`, `allowDangerousTypes=true`
- [ ] `make generate` — 6 CRDs with full KCC schemas, deepcopy

### Phase 2: GCPKCCManagedCluster Controller
- [ ] Feature gate, KCC CRD check, externally-managed, pause, finalizer
- [ ] Creates typed ComputeNetwork + ComputeSubnetwork
- [ ] Patches secondary CIDR ranges, deletion gate
- [ ] `status.initialization.provisioned`, v1beta2 conditions

### Phase 3: GCPKCCManagedControlPlane Controller
- [ ] Feature gate, KCC CRD, externally-managed, pause, finalizer
- [ ] Gated on infra cluster provisioned (via `getInfraCluster`)
- [ ] Creates typed ContainerCluster, kubeconfig generation
- [ ] `status.initialization.controlPlaneInitialized`, `externalManagedControlPlane = true`

### Phase 4: GCPKCCMachinePool Controller
- [ ] Feature gate, KCC CRD, pause, finalizer
- [ ] Gated on control plane initialized (via `getControlPlane`)
- [ ] Creates typed ContainerNodePool, `spec.providerIDList` from workload Nodes
- [ ] Fetches owner MachinePool via `exputil.GetOwnerMachinePool`

### Phase 5: Tests
- [ ] Pure function tests: `isKCCConditionTrue`, `patchSubnetworkCIDRs`
- [ ] Defaults tests: all 6 `apply*` functions
- [ ] Reconciler tests: GCPKCCManagedCluster (feature gate, CRUD, delete flows)

### Phase 6: Makefile + Installation Script
- [ ] `hack/install-config-connector.sh` with secret-based credentials
- [ ] Makefile targets: `create-management-cluster-kcc`, `install-config-connector`

### Phase 7: Template Flavors
- [ ] `cluster-template-gke-kcc.yaml` — simple non-topology
- [ ] `cluster-template-gke-kcc-clusterclass.yaml` — ClusterClass with variables + typed patches
- [ ] `cluster-template-gke-kcc-topology.yaml` — topology-based

### Phase 8: Reasonable Defaults + CAPI Field Overrides
- [ ] `gcpkcc_defaults.go`: 6 pure functions (see "Defaults and Overrides" section)
- [ ] Controller wiring: defaults after DeepCopy, overrides from CAPI fields
- [ ] `gcpkcc_defaults_test.go`: table-driven tests

### Future Phases

- [ ] Integration tests (kind + KCC operator)
- [ ] E2E tests (create/scale/upgrade/delete lifecycle)
- [ ] Validation webhooks for inline CC specs
- [ ] Event-driven watches on KCC resources (replace 30s polling)
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

### Risk: providerIDList Population Requires Workload Cluster Access
**Impact**: GCPKCCMachinePool can't populate `spec.providerIDList` until the workload cluster is reachable
**Mitigation**: This is the same ordering constraint as the existing GKE provider. Gate providerIDList population on kubeconfig secret availability. CAPI tolerates an empty providerIDList during provisioning.

### Risk: KCC Go Type Version Coupling
**Impact**: CAPG's KCC type version is pinned in go.mod. New KCC fields added after this version won't be available until CAPG bumps the dependency. KCC marks their go-client as ALPHA, meaning breaking changes are possible.
**Mitigation**: Pin to a known-stable KCC version (`v1.147.1`). The generated API types are auto-generated from Terraform schemas and change infrequently. Only the `pkg/clients/generated/apis/` packages are imported — these have zero GCP client library dependencies, so dependency conflicts are minimal. Bump the KCC version as part of regular dependency updates.

### Risk: Two Ways to Create GKE Clusters
**Impact**: User confusion between standard CAPG GKE path and KCC path
**Mitigation**: Clear documentation with a comparison table. Each path has a distinct set of CRD kinds — there is no overlap or ambiguity at runtime.

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

### Alternative 3 (adopted): KCC Go Types per Named Field
```go
type Spec struct {
    Cluster kcccontainerv1beta1.ContainerCluster `json:"cluster"`
}
```
**Pros**: Full CRD schema generated by controller-gen → ClusterClass patches validated → `kubectl explain` works; typed controller code (no `unstructured.NestedString`); compile-time safety
**Cons**: CAPG's KCC type version is pinned to the KCC module version in go.mod; new KCC fields require a CAPG dependency bump. However, since the KCC go-client is auto-generated and marked ALPHA, this is an acceptable tradeoff for alpha.
**Decision**: Adopted.

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

## References

- [CAPI Provider Contracts](https://cluster-api.sigs.k8s.io/developer/providers/contracts/overview)
- [CAPZ Azure Service Operator Proposal](https://github.com/kubernetes-sigs/cluster-api-provider-azure/blob/main/docs/proposals/20230123-azure-service-operator.md)
- [Config Connector Documentation](https://cloud.google.com/config-connector/docs/overview)
- [Cluster API Book](https://cluster-api.sigs.k8s.io/)
- [GKE API Reference](https://cloud.google.com/kubernetes-engine/docs/reference/rest)

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 1 | ⚠️ Proposal revised | See above |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | Unavailable |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | ⚠️ Proposal revised | See above |
| Design Review | `/plan-design-review` | UI/UX gaps | 0 | — | No UI scope |

**VERDICT:** DESIGN COMPLETE — Ready for review. See TODOS.md for implementation phases. No code has been written yet.
