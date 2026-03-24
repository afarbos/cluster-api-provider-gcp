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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

const (
	// KCCClusterFinalizer allows clean up of KCC resources associated with GCPKCCManagedCluster.
	KCCClusterFinalizer = "gcpkccmanagedcluster.infrastructure.cluster.x-k8s.io"
)

// GCPKCCManagedClusterSpec defines the desired state of GCPKCCManagedCluster.
type GCPKCCManagedClusterSpec struct {
	// Network defines the KCC ComputeNetwork resource (metadata + spec as raw JSON).
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Network *runtime.RawExtension `json:"network,omitempty"`

	// Subnetwork defines the KCC ComputeSubnetwork resource (metadata + spec as raw JSON).
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Subnetwork *runtime.RawExtension `json:"subnetwork,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1beta1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// GCPKCCManagedClusterStatus defines the observed state of GCPKCCManagedCluster.
type GCPKCCManagedClusterStatus struct {
	// Initialization contains fields that track the status of cluster initialization.
	// +optional
	Initialization *GCPKCCManagedClusterInitializationStatus `json:"initialization,omitempty"`

	// Ready indicates that the cluster infrastructure is ready.
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// FailureDomains defines the failure domains for the cluster.
	// +optional
	FailureDomains clusterv1beta1.FailureDomains `json:"failureDomains,omitempty"`

	// Conditions defines current state of the GCPKCCManagedCluster.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// NetworkName is the name of the created KCC ComputeNetwork resource.
	// +optional
	NetworkName string `json:"networkName,omitempty"`

	// SubnetworkName is the name of the created KCC ComputeSubnetwork resource.
	// +optional
	SubnetworkName string `json:"subnetworkName,omitempty"`
}

// GCPKCCManagedClusterInitializationStatus contains initialization status.
type GCPKCCManagedClusterInitializationStatus struct {
	// Provisioned indicates that the infrastructure has been provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedclusters,scope=Namespaced,categories=cluster-api,shortName=gcpkccmc
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this GCPKCCManagedCluster belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Cluster infrastructure is ready"
// +kubebuilder:printcolumn:name="Network",type="string",JSONPath=".status.networkName",description="KCC ComputeNetwork name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// GCPKCCManagedCluster is the Schema for the gcpkccmanagedclusters API.
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

// GetConditions returns the conditions.
func (r *GCPKCCManagedCluster) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions.
func (r *GCPKCCManagedCluster) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}
