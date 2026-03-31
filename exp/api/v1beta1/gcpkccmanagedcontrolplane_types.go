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
	// KCCManagedControlPlaneFinalizer allows clean up of KCC resources.
	KCCManagedControlPlaneFinalizer = "gcpkccmanagedcontrolplane.infrastructure.cluster.x-k8s.io"
)

// GCPKCCManagedControlPlaneSpec defines the desired state of GCPKCCManagedControlPlane.
type GCPKCCManagedControlPlaneSpec struct {
	// ContainerCluster defines the KCC ContainerCluster resource (metadata + spec as raw JSON).
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ContainerCluster *runtime.RawExtension `json:"containerCluster,omitempty"`

	// Version is the desired Kubernetes version.
	// +optional
	Version *string `json:"version,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1beta1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// GCPKCCManagedControlPlaneStatus defines the observed state of GCPKCCManagedControlPlane.
type GCPKCCManagedControlPlaneStatus struct {
	// Initialization tracks control plane initialization.
	// +optional
	Initialization *GCPKCCManagedControlPlaneInitializationStatus `json:"initialization,omitempty"`

	// ExternalManagedControlPlane indicates the control plane is externally managed. Always true.
	// +optional
	ExternalManagedControlPlane *bool `json:"externalManagedControlPlane,omitempty"`

	// Ready indicates that the control plane is ready.
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// Initialized indicates the control plane is available for initial contact.
	// +optional
	Initialized bool `json:"initialized,omitempty"`

	// Version is the observed Kubernetes version of the control plane.
	// +optional
	Version *string `json:"version,omitempty"`

	// Conditions defines current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ClusterName is the name of the KCC ContainerCluster resource.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`

	// Replicas is the number of control plane instances. Always 1 for managed GKE.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready control plane instances.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas is the number of available control plane instances.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// UpToDateReplicas is the number of up-to-date control plane instances.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
}

// GCPKCCManagedControlPlaneInitializationStatus contains control plane initialization status.
type GCPKCCManagedControlPlaneInitializationStatus struct {
	// ControlPlaneInitialized indicates the control plane is initialized.
	// +optional
	ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedcontrolplanes,scope=Namespaced,categories=cluster-api,shortName=gcpkccmcp
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster to which this GCPKCCManagedControlPlane belongs"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Control plane is ready"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version",description="Kubernetes version"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="API Endpoint",priority=1
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// GCPKCCManagedControlPlane is the Schema for the gcpkccmanagedcontrolplanes API.
type GCPKCCManagedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GCPKCCManagedControlPlaneSpec   `json:"spec,omitempty"`
	Status GCPKCCManagedControlPlaneStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCManagedControlPlaneList contains a list of GCPKCCManagedControlPlane.
type GCPKCCManagedControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCManagedControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCManagedControlPlane{}, &GCPKCCManagedControlPlaneList{})
}

// GetConditions returns the conditions.
func (r *GCPKCCManagedControlPlane) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions.
func (r *GCPKCCManagedControlPlane) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}
