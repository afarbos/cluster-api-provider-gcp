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

package v1beta2

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ToUnstructured converts a raw KCC resource (containing metadata + spec) to
// an unstructured Kubernetes object with the given GVK.
func ToUnstructured(raw *runtime.RawExtension, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	if raw == nil || raw.Raw == nil {
		return nil, fmt.Errorf("nil raw extension for %s", gvk.Kind)
	}

	obj := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &obj); err != nil {
		return nil, fmt.Errorf("unmarshalling %s: %w", gvk.Kind, err)
	}

	u := &unstructured.Unstructured{Object: obj}
	u.SetGroupVersionKind(gvk)
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
