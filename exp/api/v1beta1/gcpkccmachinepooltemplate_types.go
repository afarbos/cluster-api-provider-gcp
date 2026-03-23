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
)

// GCPKCCMachinePoolTemplateSpec defines the desired state of GCPKCCMachinePoolTemplate.
type GCPKCCMachinePoolTemplateSpec struct {
	Template GCPKCCMachinePoolTemplateResource `json:"template"`
}

// GCPKCCMachinePoolTemplateResource describes the data needed to create a GCPKCCMachinePool from a template.
type GCPKCCMachinePoolTemplateResource struct {
	Spec GCPKCCMachinePoolSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmachinepooltemplates,scope=Namespaced,categories=cluster-api,shortName=gcpkccmpt
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"

// GCPKCCMachinePoolTemplate is the Schema for the gcpkccmachinepooltemplates API.
type GCPKCCMachinePoolTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPKCCMachinePoolTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCMachinePoolTemplateList contains a list of GCPKCCMachinePoolTemplate.
type GCPKCCMachinePoolTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCMachinePoolTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCMachinePoolTemplate{}, &GCPKCCMachinePoolTemplateList{})
}
