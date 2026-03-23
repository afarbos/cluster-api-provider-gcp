<!-- /autoplan restore point: /Users/afarbos/.gstack/projects/afarbos-cluster-api-provider-gcp/configconnector-autoplan-restore-20260323-092043.md -->
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
| Resource embedding | `[]runtime.RawExtension` (generic list) | Named typed fields (`spec.network`, `spec.cluster`, etc.) |
| Resource identity | Users list any resources in any order | Each field has a defined role — network, subnetwork, cluster, node pool |
| CAPI field patching | JSON merge patches via mutator pipeline | `unstructured.SetNestedField()` on `*unstructured.Unstructured` |
| KCC Go dependency | Full ASO Go types imported | None — controllers use `*unstructured.Unstructured`; only KCC CRDs needed at runtime |
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
2. **`runtime.RawExtension`** per named field — each field holds a complete Config Connector resource spec (the user controls all fields that CAPG doesn't patch)
3. **No KCC Go dependency** — controllers parse `runtime.RawExtension` into `*unstructured.Unstructured` and use `unstructured.SetNestedField()` for CAPI-derived patches; only KCC CRDs are needed at runtime, not the KCC Go module
4. **CAPI field minimization** — only fields that CAPG must patch for CAPI compatibility are enforced; everything else is user-controlled
5. **CAPI v1beta2 contract compliance** — all required spec/status fields, conditions, and labels as per the current contract spec

### GCPKCCManagedCluster (InfraCluster)

Implements the [InfraCluster contract](https://cluster-api.sigs.k8s.io/developer/providers/contracts/infra-cluster).

```go
type GCPKCCManagedClusterSpec struct {
    // Network is the Config Connector ComputeNetwork resource spec.
    // User provides the complete resource specification including
    // the "cnrm.cloud.google.com/project-id" annotation.
    // CAPG creates and manages the lifecycle of this resource.
    // +required
    // +kubebuilder:validation:XEmbeddedResource
    // +kubebuilder:pruning:PreserveUnknownFields
    Network runtime.RawExtension `json:"network"`

    // Subnetwork is the Config Connector ComputeSubnetwork resource spec.
    // CAPG patches spec.secondaryIpRange from Cluster.Spec.ClusterNetwork.
    // +required
    // +kubebuilder:validation:XEmbeddedResource
    // +kubebuilder:pruning:PreserveUnknownFields
    Subnetwork runtime.RawExtension `json:"subnetwork"`

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
    // Cluster is the Config Connector ContainerCluster resource spec.
    // User provides the complete resource specification including
    // the "cnrm.cloud.google.com/project-id" annotation.
    // CAPG patches spec.minMasterVersion from spec.Version (CAPI field).
    // CAPG sets spec.networkRef and spec.subnetworkRef from the sibling GCPKCCManagedCluster.
    // +required
    // +kubebuilder:validation:XEmbeddedResource
    // +kubebuilder:pruning:PreserveUnknownFields
    Cluster runtime.RawExtension `json:"cluster"`

    // Version is the Kubernetes version for the control plane.
    // Required by the ControlPlane v1beta2 contract.
    // CAPG patches this into ContainerCluster.spec.minMasterVersion.
    // +optional
    Version string `json:"version,omitempty"`

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
    // NodePool is the Config Connector ContainerNodePool resource spec.
    // User provides the complete resource specification including
    // the "cnrm.cloud.google.com/project-id" annotation.
    // CAPG patches: spec.nodeCount (from MachinePool.spec.replicas),
    //               spec.version (from MachinePool.spec.template.spec.version),
    //               spec.clusterRef (from sibling GCPKCCManagedControlPlane),
    //               spec.nodeLocations (from MachinePool.spec.failureDomains).
    // +required
    // +kubebuilder:validation:XEmbeddedResource
    // +kubebuilder:pruning:PreserveUnknownFields
    NodePool runtime.RawExtension `json:"nodePool"`

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

### CAPI Field Mapping

| CAPI Field | Config Connector Field | Resource |
|------------|------------------------|----------|
| `Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]` | `spec.secondaryIpRange[pods].ipCidrRange` | ComputeSubnetwork |
| `Cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]` | `spec.secondaryIpRange[services].ipCidrRange` | ComputeSubnetwork |
| `GCPKCCManagedControlPlane.Spec.Version` | `spec.minMasterVersion` | ContainerCluster |
| `MachinePool.Spec.Replicas` | `spec.nodeCount` (when autoscaling disabled) | ContainerNodePool |
| `MachinePool.Spec.Template.Spec.Version` | `spec.version` | ContainerNodePool |
| `MachinePool.Spec.FailureDomains` | `spec.nodeLocations` | ContainerNodePool |
| *(from GCPKCCManagedCluster.status)* | `spec.networkRef`, `spec.subnetworkRef` | ContainerCluster |
| *(from GCPKCCManagedControlPlane.status)* | `spec.clusterRef` | ContainerNodePool |

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
2. Check KCC CRDs are installed — surface condition and requeue if not
3. Check `cluster.x-k8s.io/managed-by` label — skip if externally managed
4. Add finalizer
5. Create/update ComputeNetwork from `spec.network` (parsed as `*unstructured.Unstructured`, namespace/name defaulted)
6. Create/update ComputeSubnetwork from `spec.subnetwork`, patching secondary IP ranges with `unstructured.SetNestedField()`
7. Set owner references on both KCC resources
8. Read KCC resource `status.conditions[Ready]` to determine readiness
9. Update `status.initialization.provisioned` and conditions

**Reconciliation Logic:**
```go
func (r *Reconciler) Reconcile(ctx, req) {
    // 1. Fetch GCPKCCManagedCluster
    // 2. Check feature.Gates.Enabled(feature.ConfigConnector) → return if false
    // 3. Check KCC CRDs present → surface condition if not
    // 4. Fetch owner CAPI Cluster
    // 5. Handle pause (set Paused condition, return)
    // 6. Handle deletion (remove finalizer, owner refs cascade deletes KCC resources)
    // 7. Add finalizer if not present
    // 8. Create/update ComputeNetwork
    // 9. Create/update ComputeSubnetwork (patch CIDRs from CAPI)
    // 10. Check KCC resource readiness
    // 11. Set status.initialization.provisioned, conditions, networkName, subnetworkName
}
```

**Deletion:** Owner references on KCC resources ensure cascaded deletion. The controller simply removes its finalizer once the KCC resources are gone (checking for their absence before removing).

### GCPKCCManagedControlPlane Controller

**Responsibilities:**
1. Check feature gate, KCC CRDs, pause, externally-managed
2. Add finalizer
3. Wait for `GCPKCCManagedCluster.status.initialization.provisioned = true`
4. Create/update ContainerCluster, patching `spec.minMasterVersion`, `spec.networkRef`, `spec.subnetworkRef`
5. Set `spec.removeDefaultNodePool: true` annotation
6. Watch ContainerCluster `status.conditions[Ready]`
7. When ready: extract endpoint, update `spec.controlPlaneEndpoint`
8. Generate kubeconfig secret: `<cluster>-kubeconfig`
9. Set `status.initialization.controlPlaneInitialized`, `status.externalManagedControlPlane = true`, conditions

**Kubeconfig generation:**
```go
// When ContainerCluster is Ready:
// 1. Read cluster CA cert from containerCluster.status.masterAuth.clusterCaCertificate
// 2. Read endpoint from containerCluster.status.endpoint
// 3. Generate kubeconfig with exec credential plugin:
//    - users[0].user.exec.command: gke-gcloud-auth-plugin
//    - users[0].user.exec.provideClusterInfo: true
// 4. Write Secret:
//    Name:      <cluster-name>-kubeconfig
//    Namespace: <cluster-namespace>
//    Type:      cluster.x-k8s.io/secret
//    Labels:    cluster.x-k8s.io/cluster-name: <cluster-name>
//    Data:      value: <base64-encoded kubeconfig>
```

### GCPKCCMachinePool Controller

**Responsibilities:**
1. Check feature gate, KCC CRDs, pause, externally-managed
2. Add finalizer
3. Wait for `GCPKCCManagedControlPlane.status.initialization.controlPlaneInitialized = true`
4. Create/update ContainerNodePool, patching `spec.clusterRef`, `spec.nodeCount`, `spec.version`, `spec.nodeLocations`
5. Watch ContainerNodePool `status.conditions[Ready]`
6. When ControlPlane kubeconfig is available: list workload cluster `Node` objects to populate `spec.providerIDList`
7. Set `status.initialization.provisioned`, `status.replicas`, `status.readyReplicas`, conditions

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

Config Connector must be installed separately by users before enabling the `ConfigConnector` feature gate:

```bash
# Install Config Connector operator
kubectl apply -f https://github.com/GoogleCloudPlatform/k8s-config-connector/releases/download/v1.121.0/install-bundle.yaml

# Configure Config Connector (in cnrm-system namespace)
kubectl apply -f config/config-connector/configconnector.yaml
```

The `ConfigConnector` resource must be configured in `cnrm-system` with your GCP credentials (Workload Identity or service account key).

## Implementation Plan

### Phase 1: API Types ✅ (exists, needs revision)
- [ ] Revise API types for full CAPI v1beta2 contract compliance:
  - Add `status.initialization.provisioned` to GCPKCCManagedCluster
  - Add `status.initialization.controlPlaneInitialized` to GCPKCCManagedControlPlane
  - Add `status.externalManagedControlPlane` to GCPKCCManagedControlPlane
  - Add `spec.providerIDList` to GCPKCCMachinePool
  - Switch from `status.failureReason/failureMessage` to v1beta2 conditions
  - Add CRD label `cluster.x-k8s.io/v1beta2: v1beta2`
- [ ] Regenerate DeepCopy methods and CRDs

### Phase 2: Infrastructure Cluster Controller (rewrite)
- [ ] Add feature gate check in Reconcile()
- [ ] Add KCC CRD startup check
- [ ] Add `cluster.x-k8s.io/managed-by` (externally managed) handling
- [ ] Implement proper pause handling with Paused condition
- [ ] Fix deletion: verify KCC resources are gone before removing finalizer
- [ ] Update status to use `status.initialization.provisioned`
- [ ] Emit v1beta2 conditions (Ready, Paused)

### Phase 3: Control Plane Controller (rewrite)
- [ ] Add feature gate, KCC CRD, pause handling
- [ ] Wait for GCPKCCManagedCluster to be provisioned before creating ContainerCluster
- [ ] Set `status.externalManagedControlPlane = true`
- [ ] Implement kubeconfig generation from ContainerCluster status
- [ ] Update status to use `status.initialization.controlPlaneInitialized`
- [ ] Emit v1beta2 conditions (Available, Paused)

### Phase 4: Machine Pool Controller (rewrite)
- [ ] Add feature gate, KCC CRD, pause handling
- [ ] Wait for GCPKCCManagedControlPlane to be initialized
- [ ] Fix `ReadyReplicas` — don't blindly set equal to Replicas
- [ ] Implement `spec.providerIDList` population from workload cluster Nodes
- [ ] Update status to use `status.initialization.provisioned`
- [ ] Emit v1beta2 conditions (Ready, Paused)

### Phase 5: Unit Tests (blocking alpha)
- [ ] Fix test compilation: update CAPI API import from `api/v1beta1` → `api/core/v1beta2`
- [ ] Fix `patchSubnetworkCIDRs` test: use `*kcccv1beta1.ComputeSubnetwork` not `*unstructured.Unstructured`
- [ ] Unit tests for all pure functions (isNetworkReady, isAutoscalingEnabled, getResourceNameOrDefault)
- [ ] Reconciler unit tests using envtest + fake KCC CRDs (happy path, deletion, pause)

### Phase 6: Documentation + Examples
- [ ] Update user guide
- [ ] Update templates to reflect new API shapes
- [ ] Add migration guide from old KCC API types (if any users exist)

### Future Phases

- [ ] Integration tests (kind + KCC operator)
- [ ] E2E tests (create/scale/upgrade/delete lifecycle)
- [ ] Validation webhooks for inline CC specs
- [ ] Event-driven watches on KCC resources (replace 30s polling)
- [ ] Graduation to beta
- [ ] Additional CC resources (CloudSQL, CloudMemorystore, etc.)

## Testing Strategy

### Unit Tests (required for alpha)
- All pure functions tested in isolation
- Reconciler tests using envtest with fake KCC CRDs installed
- Happy path, error paths, deletion, pause

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
- [x] API types defined
- [ ] API types fully CAPI v1beta2 compliant (revision)
- [ ] Controllers implemented and passing unit tests
- [ ] Feature gate enforced in all controllers
- [ ] KCC CRD check at startup
- [ ] Kubeconfig generation implemented
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

### Risk: Version Compatibility with KCC
**Impact**: KCC API changes could break CAPG
**Mitigation**: Document supported KCC versions. Use stable KCC APIs. Pin the KCC Go module version.

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

### Alternative 2: Fully Typed KCC Structs (embed `kcccontainerv1beta1.ContainerCluster`)
```go
type Spec struct {
    Cluster kcccontainerv1beta1.ContainerCluster `json:"cluster"`
}
```
**Pros**: Full schema validation at admission time
**Cons**: Tightly couples CAPG API version to KCC API version; bumping KCC API version requires a CAPG API version bump; breaks when KCC adds/removes fields
**Decision**: Rejected. `runtime.RawExtension` per named field gives named-field ergonomics without version lock-in.

### Alternative 3: Reference-Only Pattern
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

**VERDICT:** PROPOSAL UPDATED — current PR is a false start. See Implementation Plan above for what needs to be redone. See TODOS.md for ordered task list.
