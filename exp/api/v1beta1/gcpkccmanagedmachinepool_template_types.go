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

// GCPKCCManagedMachinePoolTemplateSpec defines the desired state of GCPKCCManagedMachinePoolTemplate.
type GCPKCCManagedMachinePoolTemplateSpec struct {
	Template GCPKCCManagedMachinePoolTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedmachinepooltemplates,scope=Namespaced,categories=cluster-api,shortName=gcpkccmmpt
// +kubebuilder:storageversion

// GCPKCCManagedMachinePoolTemplate is the Schema for the GCPKCCManagedMachinePoolTemplates API.
type GCPKCCManagedMachinePoolTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPKCCManagedMachinePoolTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCManagedMachinePoolTemplateList contains a list of GCPKCCManagedMachinePoolTemplates.
type GCPKCCManagedMachinePoolTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCManagedMachinePoolTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCManagedMachinePoolTemplate{}, &GCPKCCManagedMachinePoolTemplateList{})
}

// GCPKCCManagedMachinePoolTemplateResource describes the data needed to create an GCPKCCManagedMachinePool from a template.
type GCPKCCManagedMachinePoolTemplateResource struct {
	Spec GCPKCCManagedMachinePoolTemplateResourceSpec `json:"spec"`
}

// GCPKCCManagedMachinePoolTemplateResourceSpec defines the desired state of GCPKCCManagedMachinePool for use in a template.
type GCPKCCManagedMachinePoolTemplateResourceSpec struct {
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	NodePool *runtime.RawExtension `json:"nodePool,omitempty"`
}
