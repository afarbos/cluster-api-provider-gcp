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
	kcccomputev1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/compute/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

const (
	// KCCClusterFinalizer allows clean up of Config Connector resources before removing the GCPKCCManagedCluster.
	KCCClusterFinalizer = "gcpkccmanagedcluster.infrastructure.cluster.x-k8s.io"
)

// GCPKCCManagedClusterSpec defines the desired state of GCPKCCManagedCluster.
//
// Users provide complete Config Connector resource specifications for the network
// and subnetwork. CAPG creates those resources and patches the CAPI-derived fields
// (secondary IP ranges for pods/services) into the subnetwork spec.
//
// Each Config Connector resource must include the "cnrm.cloud.google.com/project-id"
// annotation to indicate which GCP project to use.
type GCPKCCManagedClusterSpec struct {
	// Network is a complete Config Connector ComputeNetwork resource.
	// CAPG creates this resource and manages its lifecycle via owner references.
	// +required
	Network kcccomputev1beta1.ComputeNetwork `json:"network"`

	// Subnetwork is a complete Config Connector ComputeSubnetwork resource.
	// CAPG creates this resource and patches the secondaryIpRange field from
	// Cluster.Spec.ClusterNetwork (pods and services CIDRs).
	// +required
	Subnetwork kcccomputev1beta1.ComputeSubnetwork `json:"subnetwork"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// Populated by the GCPKCCManagedControlPlane controller once the GKE cluster endpoint is available.
	// +optional
	ControlPlaneEndpoint clusterv1beta1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// GCPKCCManagedClusterStatus defines the observed state of GCPKCCManagedCluster.
type GCPKCCManagedClusterStatus struct {
	// Initialization holds the v1beta2 InfraCluster initialization status.
	// +optional
	Initialization *GCPKCCManagedClusterInitializationStatus `json:"initialization,omitempty"`

	// Ready denotes that the cluster network infrastructure is fully provisioned.
	// This is a v1beta1 compatibility field; prefer Initialization.Provisioned.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// FailureDomains lists the failure domains available for scheduling.
	// +optional
	FailureDomains clusterv1beta1.FailureDomains `json:"failureDomains,omitempty"`

	// Conditions represents the latest observations of the GCPKCCManagedCluster's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// NetworkName is the name of the created ComputeNetwork resource.
	// +optional
	NetworkName string `json:"networkName,omitempty"`

	// SubnetworkName is the name of the created ComputeSubnetwork resource.
	// +optional
	SubnetworkName string `json:"subnetworkName,omitempty"`
}

// GCPKCCManagedClusterInitializationStatus holds initialization fields per the CAPI v1beta2 InfraCluster contract.
type GCPKCCManagedClusterInitializationStatus struct {
	// Provisioned is true when the network infrastructure (ComputeNetwork and ComputeSubnetwork)
	// are fully provisioned in GCP. This is the v1beta2 replacement for status.ready.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedclusters,scope=Namespaced,categories=cluster-api,shortName=gcpkccmc
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Network",type="string",JSONPath=".status.networkName"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"

// GCPKCCManagedCluster is the Schema for the gcpkccmanagedclusters API.
// It manages GKE network infrastructure (VPC, subnet) via Config Connector resources.
type GCPKCCManagedCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GCPKCCManagedClusterSpec   `json:"spec,omitempty"`
	Status GCPKCCManagedClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCManagedClusterList contains a list of GCPKCCManagedCluster.
type GCPKCCManagedClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCManagedCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCManagedCluster{}, &GCPKCCManagedClusterList{})
}
