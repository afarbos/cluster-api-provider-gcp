/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// KCCObjectMeta contains metadata for a KCC resource.
type KCCObjectMeta struct {
	// Name is the name of the KCC resource.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the KCC resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Annotations is an optional set of annotations on the KCC resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels is an optional set of labels on the KCC resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// KCCResourceRef is a reference to a KCC resource by name.
type KCCResourceRef struct {
	// Name is the name of the referenced KCC resource.
	// +optional
	Name string `json:"name,omitempty"`
}

// KCCSecondaryIPRange defines a secondary IP range for a subnetwork.
type KCCSecondaryIPRange struct {
	// RangeName is the name identifying the secondary range.
	RangeName string `json:"rangeName"`

	// IpCidrRange is the range of IP addresses for this secondary range.
	IpCidrRange string `json:"ipCidrRange"`
}

// --- Intermediate types for KCC ComputeNetwork ---

// GCPKCCNetworkResource wraps a KCC ComputeNetwork spec with metadata.
type GCPKCCNetworkResource struct {
	// Metadata for the KCC ComputeNetwork resource.
	// +optional
	Metadata KCCObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KCC ComputeNetwork.
	// +optional
	Spec GCPKCCComputeNetworkSpec `json:"spec,omitempty"`
}

// GCPKCCComputeNetworkSpec defines commonly-used fields for a KCC ComputeNetwork.
type GCPKCCComputeNetworkSpec struct {
	// AutoCreateSubnetworks controls whether to create a subnetwork for each region automatically.
	// +optional
	AutoCreateSubnetworks *bool `json:"autoCreateSubnetworks,omitempty"`

	// RoutingMode is the network-wide routing mode to use. Possible values: REGIONAL, GLOBAL.
	// +optional
	RoutingMode string `json:"routingMode,omitempty"`

	// Description is a human-readable description of the network.
	// +optional
	Description string `json:"description,omitempty"`

	// AdditionalConfig allows passing additional KCC fields not covered by the typed schema.
	// These fields are merged into the KCC resource spec at reconcile time.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AdditionalConfig *runtime.RawExtension `json:"additionalConfig,omitempty"`
}

// --- Intermediate types for KCC ComputeSubnetwork ---

// GCPKCCSubnetworkResource wraps a KCC ComputeSubnetwork spec with metadata.
type GCPKCCSubnetworkResource struct {
	// Metadata for the KCC ComputeSubnetwork resource.
	// +optional
	Metadata KCCObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KCC ComputeSubnetwork.
	// +optional
	Spec GCPKCCComputeSubnetworkSpec `json:"spec,omitempty"`
}

// GCPKCCComputeSubnetworkSpec defines commonly-used fields for a KCC ComputeSubnetwork.
type GCPKCCComputeSubnetworkSpec struct {
	// IpCidrRange is the range of internal addresses owned by this subnetwork.
	// +optional
	IpCidrRange string `json:"ipCidrRange,omitempty"`

	// Region is the GCP region for this subnetwork.
	// +optional
	Region string `json:"region,omitempty"`

	// SecondaryIpRange defines secondary IP ranges for pods and services.
	// +optional
	SecondaryIpRange []KCCSecondaryIPRange `json:"secondaryIpRange,omitempty"`

	// NetworkRef is a reference to the network this subnetwork belongs to.
	// +optional
	NetworkRef KCCResourceRef `json:"networkRef,omitempty"`

	// Description is a human-readable description of the subnetwork.
	// +optional
	Description string `json:"description,omitempty"`

	// AdditionalConfig allows passing additional KCC fields not covered by the typed schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AdditionalConfig *runtime.RawExtension `json:"additionalConfig,omitempty"`
}

// --- Intermediate types for KCC ContainerCluster ---

// GCPKCCContainerClusterResource wraps a KCC ContainerCluster spec with metadata.
type GCPKCCContainerClusterResource struct {
	// Metadata for the KCC ContainerCluster resource.
	// +optional
	Metadata KCCObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KCC ContainerCluster.
	// +optional
	Spec GCPKCCContainerClusterSpec `json:"spec,omitempty"`
}

// GCPKCCContainerClusterSpec defines commonly-used fields for a KCC ContainerCluster.
type GCPKCCContainerClusterSpec struct {
	// Location is the region or zone for the GKE cluster.
	// +optional
	Location string `json:"location,omitempty"`

	// NetworkingMode configures the networking mode. Possible values: VPC_NATIVE, ROUTES.
	// +optional
	NetworkingMode string `json:"networkingMode,omitempty"`

	// InitialNodeCount is the number of nodes to create in this cluster's default node pool.
	// +optional
	InitialNodeCount *int32 `json:"initialNodeCount,omitempty"`

	// RemoveDefaultNodePool controls whether to delete the default node pool after cluster creation.
	// +optional
	RemoveDefaultNodePool *bool `json:"removeDefaultNodePool,omitempty"`

	// NetworkRef is a reference to the VPC network for this cluster.
	// +optional
	NetworkRef KCCResourceRef `json:"networkRef,omitempty"`

	// SubnetworkRef is a reference to the VPC subnetwork for this cluster.
	// +optional
	SubnetworkRef KCCResourceRef `json:"subnetworkRef,omitempty"`

	// IpAllocationPolicy configures IP allocation for pods and services.
	// +optional
	IpAllocationPolicy *KCCIPAllocationPolicy `json:"ipAllocationPolicy,omitempty"`

	// MinMasterVersion is the minimum version of the master. Managed by CAPI version override.
	// +optional
	MinMasterVersion string `json:"minMasterVersion,omitempty"`

	// ReleaseChannel configures the release channel for automatic upgrades.
	// +optional
	ReleaseChannel *KCCReleaseChannel `json:"releaseChannel,omitempty"`

	// LoggingService is the logging service the cluster should use.
	// +optional
	LoggingService string `json:"loggingService,omitempty"`

	// MonitoringService is the monitoring service the cluster should use.
	// +optional
	MonitoringService string `json:"monitoringService,omitempty"`

	// WorkloadIdentityConfig configures workload identity for the cluster.
	// +optional
	WorkloadIdentityConfig *KCCWorkloadIdentityConfig `json:"workloadIdentityConfig,omitempty"`

	// PrivateClusterConfig configures private cluster settings.
	// +optional
	PrivateClusterConfig *KCCPrivateClusterConfig `json:"privateClusterConfig,omitempty"`

	// MasterAuthorizedNetworksConfig configures master authorized networks.
	// +optional
	MasterAuthorizedNetworksConfig *KCCMasterAuthorizedNetworksConfig `json:"masterAuthorizedNetworksConfig,omitempty"`

	// AdditionalConfig allows passing additional KCC fields not covered by the typed schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AdditionalConfig *runtime.RawExtension `json:"additionalConfig,omitempty"`
}

// KCCIPAllocationPolicy configures IP allocation for pods and services.
type KCCIPAllocationPolicy struct {
	// ClusterSecondaryRangeName is the name of the secondary range for pod IPs.
	// +optional
	ClusterSecondaryRangeName string `json:"clusterSecondaryRangeName,omitempty"`

	// ServicesSecondaryRangeName is the name of the secondary range for service IPs.
	// +optional
	ServicesSecondaryRangeName string `json:"servicesSecondaryRangeName,omitempty"`
}

// KCCReleaseChannel configures the release channel.
type KCCReleaseChannel struct {
	// Channel is the release channel. Possible values: RAPID, REGULAR, STABLE, EXTENDED.
	// +optional
	Channel string `json:"channel,omitempty"`
}

// KCCWorkloadIdentityConfig configures workload identity.
type KCCWorkloadIdentityConfig struct {
	// WorkloadPool is the workload identity pool.
	// +optional
	WorkloadPool string `json:"workloadPool,omitempty"`
}

// KCCPrivateClusterConfig configures private cluster settings.
type KCCPrivateClusterConfig struct {
	// EnablePrivateEndpoint controls whether the master's internal IP is used as the cluster endpoint.
	// +optional
	EnablePrivateEndpoint *bool `json:"enablePrivateEndpoint,omitempty"`

	// EnablePrivateNodes controls whether nodes have internal IP addresses only.
	// +optional
	EnablePrivateNodes *bool `json:"enablePrivateNodes,omitempty"`

	// MasterIpv4CidrBlock is the IP range in CIDR notation for the master network.
	// +optional
	MasterIpv4CidrBlock string `json:"masterIpv4CidrBlock,omitempty"`
}

// KCCMasterAuthorizedNetworksConfig configures master authorized networks.
type KCCMasterAuthorizedNetworksConfig struct {
	// CidrBlocks defines the list of CIDR blocks allowed to access the master.
	// +optional
	CidrBlocks []KCCMasterAuthorizedNetworksCidrBlock `json:"cidrBlocks,omitempty"`
}

// KCCMasterAuthorizedNetworksCidrBlock defines a CIDR block for master authorized networks.
type KCCMasterAuthorizedNetworksCidrBlock struct {
	// DisplayName is a display name for the CIDR block.
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// CidrBlock is an external network that can access the master.
	// +optional
	CidrBlock string `json:"cidrBlock,omitempty"`
}

// --- Intermediate types for KCC ContainerNodePool ---

// GCPKCCContainerNodePoolResource wraps a KCC ContainerNodePool spec with metadata.
type GCPKCCContainerNodePoolResource struct {
	// Metadata for the KCC ContainerNodePool resource.
	// +optional
	Metadata KCCObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the KCC ContainerNodePool.
	// +optional
	Spec GCPKCCContainerNodePoolSpec `json:"spec,omitempty"`
}

// GCPKCCContainerNodePoolSpec defines commonly-used fields for a KCC ContainerNodePool.
type GCPKCCContainerNodePoolSpec struct {
	// Location is the region or zone for the node pool.
	// +optional
	Location string `json:"location,omitempty"`

	// InitialNodeCount is the initial number of nodes for the pool.
	// +optional
	InitialNodeCount *int32 `json:"initialNodeCount,omitempty"`

	// Version is the Kubernetes version for the nodes.
	// +optional
	Version string `json:"version,omitempty"`

	// NodeLocations is the list of zones in which the node pool's nodes should be located.
	// +optional
	NodeLocations []string `json:"nodeLocations,omitempty"`

	// ClusterRef is a reference to the ContainerCluster this pool belongs to.
	// +optional
	ClusterRef KCCResourceRef `json:"clusterRef,omitempty"`

	// NodeConfig defines the configuration of each node in the pool.
	// +optional
	NodeConfig *KCCNodeConfig `json:"nodeConfig,omitempty"`

	// Autoscaling configures autoscaling for the node pool.
	// +optional
	Autoscaling *KCCNodePoolAutoscaling `json:"autoscaling,omitempty"`

	// Management specifies node management options (auto-upgrade, auto-repair).
	// +optional
	Management *KCCNodePoolManagement `json:"management,omitempty"`

	// AdditionalConfig allows passing additional KCC fields not covered by the typed schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AdditionalConfig *runtime.RawExtension `json:"additionalConfig,omitempty"`
}

// KCCNodeConfig defines the configuration of each node.
type KCCNodeConfig struct {
	// MachineType is the Google Compute Engine machine type.
	// +optional
	MachineType string `json:"machineType,omitempty"`

	// DiskSizeGb is the size of the disk attached to each node, specified in GB.
	// +optional
	DiskSizeGb *int32 `json:"diskSizeGb,omitempty"`

	// DiskType is the type of the disk attached to each node.
	// +optional
	DiskType string `json:"diskType,omitempty"`

	// ImageType is the image type to use for nodes.
	// +optional
	ImageType string `json:"imageType,omitempty"`

	// Labels is the map of Kubernetes labels applied to each node.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Tags is the list of instance tags applied to each node.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// OauthScopes is the set of Google API scopes available on all nodes.
	// +optional
	OauthScopes []string `json:"oauthScopes,omitempty"`

	// ServiceAccountRef is a reference to the service account to be used by nodes.
	// +optional
	ServiceAccountRef *KCCResourceRef `json:"serviceAccountRef,omitempty"`
}

// KCCNodePoolAutoscaling configures autoscaling.
type KCCNodePoolAutoscaling struct {
	// MinNodeCount is the minimum number of nodes in the pool.
	// +optional
	MinNodeCount *int32 `json:"minNodeCount,omitempty"`

	// MaxNodeCount is the maximum number of nodes in the pool.
	// +optional
	MaxNodeCount *int32 `json:"maxNodeCount,omitempty"`

	// Enabled controls whether autoscaling is enabled.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
}

// KCCNodePoolManagement specifies auto-upgrade and auto-repair options.
type KCCNodePoolManagement struct {
	// AutoUpgrade specifies whether node auto-upgrade is enabled.
	// +optional
	AutoUpgrade *bool `json:"autoUpgrade,omitempty"`

	// AutoRepair specifies whether the node auto-repair is enabled.
	// +optional
	AutoRepair *bool `json:"autoRepair,omitempty"`
}
