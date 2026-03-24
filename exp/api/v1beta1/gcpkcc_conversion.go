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
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// rawExtensionToMap unmarshals a RawExtension into a map.
func rawExtensionToMap(raw *runtime.RawExtension) (map[string]interface{}, error) {
	if raw == nil || raw.Raw == nil {
		return map[string]interface{}{}, nil
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshalling spec: %w", err)
	}
	return m, nil
}

// ToUnstructuredComputeNetwork converts a GCPKCCNetworkResource to an
// unstructured KCC ComputeNetwork resource.
func ToUnstructuredComputeNetwork(res GCPKCCNetworkResource, namespace string) (*unstructured.Unstructured, error) {
	spec, err := rawExtensionToMap(res.Spec)
	if err != nil {
		return nil, fmt.Errorf("parsing network spec: %w", err)
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(ComputeNetworkGVK)
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if len(res.Metadata.Annotations) > 0 {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if len(res.Metadata.Labels) > 0 {
		u.SetLabels(res.Metadata.Labels)
	}
	if len(spec) > 0 {
		if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
			return nil, fmt.Errorf("setting spec: %w", err)
		}
	}

	return u, nil
}

// ToUnstructuredComputeSubnetwork converts a GCPKCCSubnetworkResource to an
// unstructured KCC ComputeSubnetwork resource.
func ToUnstructuredComputeSubnetwork(res GCPKCCSubnetworkResource, namespace string) (*unstructured.Unstructured, error) {
	spec, err := rawExtensionToMap(res.Spec)
	if err != nil {
		return nil, fmt.Errorf("parsing subnetwork spec: %w", err)
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(ComputeSubnetworkGVK)
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if len(res.Metadata.Annotations) > 0 {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if len(res.Metadata.Labels) > 0 {
		u.SetLabels(res.Metadata.Labels)
	}
	if len(spec) > 0 {
		if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
			return nil, fmt.Errorf("setting spec: %w", err)
		}
	}

	return u, nil
}

// ToUnstructuredContainerCluster converts a GCPKCCContainerClusterResource to
// an unstructured KCC ContainerCluster resource.
func ToUnstructuredContainerCluster(res GCPKCCContainerClusterResource, namespace string) (*unstructured.Unstructured, error) {
	spec, err := rawExtensionToMap(res.Spec)
	if err != nil {
		return nil, fmt.Errorf("parsing cluster spec: %w", err)
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(ContainerClusterGVK)
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if len(res.Metadata.Annotations) > 0 {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if len(res.Metadata.Labels) > 0 {
		u.SetLabels(res.Metadata.Labels)
	}
	if len(spec) > 0 {
		if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
			return nil, fmt.Errorf("setting spec: %w", err)
		}
	}

	return u, nil
}

// ToUnstructuredContainerNodePool converts a GCPKCCContainerNodePoolResource to
// an unstructured KCC ContainerNodePool resource.
func ToUnstructuredContainerNodePool(res GCPKCCContainerNodePoolResource, namespace string) (*unstructured.Unstructured, error) {
	spec, err := rawExtensionToMap(res.Spec)
	if err != nil {
		return nil, fmt.Errorf("parsing nodepool spec: %w", err)
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(ContainerNodePoolGVK)
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if len(res.Metadata.Annotations) > 0 {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if len(res.Metadata.Labels) > 0 {
		u.SetLabels(res.Metadata.Labels)
	}
	if len(spec) > 0 {
		if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
			return nil, fmt.Errorf("setting spec: %w", err)
		}
	}

	return u, nil
}

// Hub marks GCPKCCManagedCluster as a conversion hub.
func (*GCPKCCManagedCluster) Hub() {}

// Hub marks GCPKCCManagedClusterList as a conversion hub.
func (*GCPKCCManagedClusterList) Hub() {}

// Hub marks GCPKCCManagedControlPlane as a conversion hub.
func (*GCPKCCManagedControlPlane) Hub() {}

// Hub marks GCPKCCManagedControlPlaneList as a conversion hub.
func (*GCPKCCManagedControlPlaneList) Hub() {}

// Hub marks GCPKCCManagedMachinePool as a conversion hub.
func (*GCPKCCManagedMachinePool) Hub() {}

// Hub marks GCPKCCManagedMachinePoolList as a conversion hub.
func (*GCPKCCManagedMachinePoolList) Hub() {}
