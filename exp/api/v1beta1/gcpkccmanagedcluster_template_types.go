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

// GCPKCCManagedClusterTemplateSpec defines the desired state of GCPKCCManagedClusterTemplate.
type GCPKCCManagedClusterTemplateSpec struct {
	Template GCPKCCManagedClusterTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedclustertemplates,scope=Namespaced,categories=cluster-api,shortName=gcpkccmct
// +kubebuilder:storageversion

// GCPKCCManagedClusterTemplate is the Schema for the GCPKCCManagedClusterTemplates API.
type GCPKCCManagedClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPKCCManagedClusterTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCManagedClusterTemplateList contains a list of GCPKCCManagedClusterTemplates.
type GCPKCCManagedClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCManagedClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCManagedClusterTemplate{}, &GCPKCCManagedClusterTemplateList{})
}

// GCPKCCManagedClusterTemplateResource describes the data needed to create an GCPKCCManagedCluster from a template.
type GCPKCCManagedClusterTemplateResource struct {
	Spec GCPKCCManagedClusterTemplateResourceSpec `json:"spec"`
}

// GCPKCCManagedClusterTemplateResourceSpec defines the desired state of GCPKCCManagedCluster for use in a template.
type GCPKCCManagedClusterTemplateResourceSpec struct {
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Network *runtime.RawExtension `json:"network,omitempty"`

	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Subnetwork *runtime.RawExtension `json:"subnetwork,omitempty"`
}
