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
	// KCCControlPlaneFinalizer allows clean up of Config Connector resources before removing the GCPKCCManagedControlPlane.
	KCCControlPlaneFinalizer = "gcpkccmanagedcontrolplane.infrastructure.cluster.x-k8s.io"
)

// GCPKCCManagedControlPlaneSpec defines the desired state of GCPKCCManagedControlPlane.
//
// Users provide a complete Config Connector ContainerCluster resource specification.
// CAPG creates the resource, watches its status, and once the GKE cluster endpoint is
// available, generates the kubeconfig secret and marks the control plane initialized.
//
// The Config Connector resource must include the "cnrm.cloud.google.com/project-id"
// annotation to indicate which GCP project to use.
type GCPKCCManagedControlPlaneSpec struct {
	// Cluster is a complete Config Connector ContainerCluster resource spec.
	// CAPG creates this resource and manages its lifecycle via owner references.
	//
	// Example:
	//   apiVersion: container.cnrm.cloud.google.com/v1beta1
	//   kind: ContainerCluster
	//   metadata:
	//     name: my-cluster
	//     annotations:
	//       cnrm.cloud.google.com/project-id: "my-gcp-project"
	//   spec:
	//     location: us-central1
	//     initialNodeCount: 1
	//
	// +required
	// +kubebuilder:validation:XEmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	Cluster runtime.RawExtension `json:"cluster"`

	// Version is the Kubernetes version of the GKE cluster (e.g., "1.29").
	// This is informational; the actual version is set in the ContainerCluster spec.
	// +optional
	Version *string `json:"version,omitempty"`

	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// Populated by the controller once the GKE cluster endpoint is available.
	// +optional
	ControlPlaneEndpoint clusterv1beta1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// GCPKCCManagedControlPlaneStatus defines the observed state of GCPKCCManagedControlPlane.
type GCPKCCManagedControlPlaneStatus struct {
	// Initialization holds the v1beta2 ControlPlane initialization status.
	// +optional
	Initialization *GCPKCCManagedControlPlaneInitializationStatus `json:"initialization,omitempty"`

	// ExternalManagedControlPlane is always true for GKE clusters, indicating that the
	// Kubernetes control plane is managed externally (by GKE) and CAPI should not attempt
	// to manage it directly. This is a mandatory v1beta2 ControlPlane contract field.
	// +optional
	ExternalManagedControlPlane *bool `json:"externalManagedControlPlane,omitempty"`

	// Ready denotes that the control plane is ready to serve requests.
	// This is a v1beta1 compatibility field; prefer Initialization.ControlPlaneInitialized.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Initialized denotes that the control plane has been initialized.
	// This is a v1beta1 compatibility field; prefer Initialization.ControlPlaneInitialized.
	// +optional
	Initialized bool `json:"initialized,omitempty"`

	// Version is the Kubernetes version of the running GKE cluster.
	// +optional
	Version *string `json:"version,omitempty"`

	// Conditions represents the latest observations of the GCPKCCManagedControlPlane's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GCPKCCManagedControlPlaneInitializationStatus holds initialization fields per the CAPI v1beta2 ControlPlane contract.
type GCPKCCManagedControlPlaneInitializationStatus struct {
	// ControlPlaneInitialized is true when the GKE cluster endpoint is available and
	// the kubeconfig secret has been written. This is the v1beta2 replacement for status.initialized.
	// +optional
	ControlPlaneInitialized *bool `json:"controlPlaneInitialized,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedcontrolplanes,scope=Namespaced,categories=cluster-api,shortName=gcpkccmcp
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"

// GCPKCCManagedControlPlane is the Schema for the gcpkccmanagedcontrolplanes API.
// It manages a GKE cluster via a Config Connector ContainerCluster resource and
// generates the kubeconfig secret once the cluster endpoint is available.
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
