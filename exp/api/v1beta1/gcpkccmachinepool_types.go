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
	// KCCMachinePoolFinalizer allows clean up of Config Connector resources before removing the GCPKCCMachinePool.
	KCCMachinePoolFinalizer = "gcpkccmachinepool.infrastructure.cluster.x-k8s.io"
)

// GCPKCCMachinePoolSpec defines the desired state of GCPKCCMachinePool.
//
// Users provide a complete Config Connector ContainerNodePool resource specification.
// CAPG creates the resource, waits for it to be ready, then populates ProviderIDList
// by listing Node objects in the workload cluster.
//
// The Config Connector resource must include the "cnrm.cloud.google.com/project-id"
// annotation to indicate which GCP project to use.
type GCPKCCMachinePoolSpec struct {
	// NodePool is a complete Config Connector ContainerNodePool resource spec.
	// CAPG creates this resource and manages its lifecycle via owner references.
	//
	// Example:
	//   apiVersion: container.cnrm.cloud.google.com/v1beta1
	//   kind: ContainerNodePool
	//   metadata:
	//     name: my-nodepool
	//     annotations:
	//       cnrm.cloud.google.com/project-id: "my-gcp-project"
	//   spec:
	//     location: us-central1
	//     clusterRef:
	//       name: my-cluster
	//     initialNodeCount: 3
	//
	// +required
	// +kubebuilder:validation:XEmbeddedResource
	// +kubebuilder:pruning:PreserveUnknownFields
	NodePool runtime.RawExtension `json:"nodePool"`

	// ProviderIDList contains the provider IDs for each node in this node pool.
	// Format: gce://<project>/<zone>/<instance-name>
	// Populated by the controller after the node pool is ready and workload cluster
	// nodes are available. This is a MANDATORY field per the CAPI v1beta2 InfraMachinePool contract.
	// +optional
	ProviderIDList []string `json:"providerIDList,omitempty"`
}

// GCPKCCMachinePoolStatus defines the observed state of GCPKCCMachinePool.
type GCPKCCMachinePoolStatus struct {
	// Initialization holds the v1beta2 InfraMachinePool initialization status.
	// +optional
	Initialization *GCPKCCMachinePoolInitializationStatus `json:"initialization,omitempty"`

	// Ready denotes that the node pool is fully provisioned and nodes are ready.
	// This is a v1beta1 compatibility field; prefer Initialization.Provisioned.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of nodes that have passed health checks.
	// This reflects the actual node count from GKE, not just the desired count.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Conditions represents the latest observations of the GCPKCCMachinePool's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GCPKCCMachinePoolInitializationStatus holds initialization fields per the CAPI v1beta2 InfraMachinePool contract.
type GCPKCCMachinePoolInitializationStatus struct {
	// Provisioned is true when the ContainerNodePool is fully provisioned in GCP.
	// This is the v1beta2 replacement for status.ready.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmachinepools,scope=Namespaced,categories=cluster-api,shortName=gcpkccmp
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"

// GCPKCCMachinePool is the Schema for the gcpkccmachinepools API.
// It manages a GKE node pool via a Config Connector ContainerNodePool resource.
type GCPKCCMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GCPKCCMachinePoolSpec   `json:"spec,omitempty"`
	Status GCPKCCMachinePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCMachinePoolList contains a list of GCPKCCMachinePool.
type GCPKCCMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCMachinePool{}, &GCPKCCMachinePoolList{})
}
