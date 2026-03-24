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
)

const (
	// KCCManagedMachinePoolFinalizer allows clean up of KCC resources associated with GCPKCCManagedMachinePool.
	KCCManagedMachinePoolFinalizer = "gcpkccmanagedmachinepool.infrastructure.cluster.x-k8s.io"
)

// GCPKCCManagedMachinePoolSpec defines the desired state of GCPKCCManagedMachinePool.
type GCPKCCManagedMachinePoolSpec struct {
	// NodePool defines the KCC ContainerNodePool resource (metadata + spec as raw JSON).
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	NodePool *runtime.RawExtension `json:"nodePool,omitempty"`

	// ProviderIDList are the provider IDs of instances in the node pool.
	// +optional
	ProviderIDList []string `json:"providerIDList,omitempty"`
}

// GCPKCCManagedMachinePoolStatus defines the observed state of GCPKCCManagedMachinePool.
type GCPKCCManagedMachinePoolStatus struct {
	// Initialization tracks provisioning status.
	// +optional
	Initialization *GCPKCCManagedMachinePoolInitializationStatus `json:"initialization,omitempty"`

	// Ready indicates the node pool has joined the cluster.
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// Replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Version is the observed Kubernetes version of the node pool.
	// +optional
	Version *string `json:"version,omitempty"`

	// NodePoolName is the name of the KCC ContainerNodePool resource.
	// +optional
	NodePoolName string `json:"nodePoolName,omitempty"`

	// Conditions defines current state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GCPKCCManagedMachinePoolInitializationStatus contains machine pool initialization status.
type GCPKCCManagedMachinePoolInitializationStatus struct {
	// Provisioned indicates the node pool has been provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedmachinepools,scope=Namespaced,categories=cluster-api,shortName=gcpkccmmp
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Node pool is ready"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas",description="Number of replicas"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version",description="Kubernetes version"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// GCPKCCManagedMachinePool is the Schema for the gcpkccmanagedmachinepools API.
type GCPKCCManagedMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GCPKCCManagedMachinePoolSpec   `json:"spec,omitempty"`
	Status GCPKCCManagedMachinePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCManagedMachinePoolList contains a list of GCPKCCManagedMachinePool.
type GCPKCCManagedMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCManagedMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCManagedMachinePool{}, &GCPKCCManagedMachinePoolList{})
}

// GetConditions returns the conditions.
func (r *GCPKCCManagedMachinePool) GetConditions() []metav1.Condition {
	return r.Status.Conditions
}

// SetConditions sets the conditions.
func (r *GCPKCCManagedMachinePool) SetConditions(conditions []metav1.Condition) {
	r.Status.Conditions = conditions
}
