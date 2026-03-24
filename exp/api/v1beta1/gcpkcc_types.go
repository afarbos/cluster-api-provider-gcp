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

// KCCResourceRef is a reference to a KCC resource by name.
type KCCResourceRef struct {
	// Name is the name of the referenced KCC resource.
	// +optional
	Name string `json:"name,omitempty"`
}

// KCCSecondaryIPRange defines a secondary IP range for a subnetwork.
type KCCSecondaryIPRange struct {
	// RangeName is the name identifying the secondary range.
	RangeName string `json:"rangeName"`

	// IpCidrRange is the range of IP addresses for this secondary range.
	IpCidrRange string `json:"ipCidrRange"`
}

// --- Resource wrappers for KCC resources ---

// GCPKCCNetworkResource wraps a KCC ComputeNetwork spec with metadata.
type GCPKCCNetworkResource struct {
	// Metadata for the KCC resource.
	// +optional
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired KCC resource spec, passed through as raw JSON.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// GCPKCCSubnetworkResource wraps a KCC ComputeSubnetwork spec with metadata.
type GCPKCCSubnetworkResource struct {
	// Metadata for the KCC resource.
	// +optional
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired KCC resource spec, passed through as raw JSON.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// GCPKCCContainerClusterResource wraps a KCC ContainerCluster spec with metadata.
type GCPKCCContainerClusterResource struct {
	// Metadata for the KCC resource.
	// +optional
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired KCC resource spec, passed through as raw JSON.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}

// GCPKCCContainerNodePoolResource wraps a KCC ContainerNodePool spec with metadata.
type GCPKCCContainerNodePoolResource struct {
	// Metadata for the KCC resource.
	// +optional
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the desired KCC resource spec, passed through as raw JSON.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Spec *runtime.RawExtension `json:"spec,omitempty"`
}
