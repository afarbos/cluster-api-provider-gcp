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

// GCPKCCManagedControlPlaneTemplateSpec defines the desired state of GCPKCCManagedControlPlaneTemplate.
type GCPKCCManagedControlPlaneTemplateSpec struct {
	Template GCPKCCManagedControlPlaneTemplateResource `json:"template"`
}

// GCPKCCManagedControlPlaneTemplateResource describes the data needed to create a GCPKCCManagedControlPlane from a template.
type GCPKCCManagedControlPlaneTemplateResource struct {
	// MachineTemplate is required by the CAPI topology contract for control plane providers.
	// It is intentionally empty as GKE manages the control plane machines externally.
	// +optional
	MachineTemplate *GCPKCCManagedControlPlaneMachineTemplate `json:"machineTemplate,omitempty"`
	Spec            GCPKCCManagedControlPlaneSpec              `json:"spec"`
}

// GCPKCCManagedControlPlaneMachineTemplate fulfills the CAPI topology contract which expects a
// MachineTemplate field for any control plane ref. GKE manages control plane nodes externally
// so this struct is intentionally empty.
type GCPKCCManagedControlPlaneMachineTemplate struct{}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=gcpkccmanagedcontrolplanetemplates,scope=Namespaced,categories=cluster-api,shortName=gcpkccmcpt
// +kubebuilder:storageversion
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"

// GCPKCCManagedControlPlaneTemplate is the Schema for the gcpkccmanagedcontrolplanetemplates API.
type GCPKCCManagedControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GCPKCCManagedControlPlaneTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// GCPKCCManagedControlPlaneTemplateList contains a list of GCPKCCManagedControlPlaneTemplate.
type GCPKCCManagedControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GCPKCCManagedControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GCPKCCManagedControlPlaneTemplate{}, &GCPKCCManagedControlPlaneTemplateList{})
}
