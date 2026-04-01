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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// KCC resource GroupVersionKinds. Defined once in the API package and
// referenced by both conversion functions and controllers.
var (
	// ComputeNetworkGVK is the GroupVersionKind for KCC ComputeNetwork resources.
	ComputeNetworkGVK = schema.GroupVersionKind{
		Group:   "compute.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ComputeNetwork",
	}

	// ComputeSubnetworkGVK is the GroupVersionKind for KCC ComputeSubnetwork resources.
	ComputeSubnetworkGVK = schema.GroupVersionKind{
		Group:   "compute.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ComputeSubnetwork",
	}

	// ContainerClusterGVK is the GroupVersionKind for KCC ContainerCluster resources.
	ContainerClusterGVK = schema.GroupVersionKind{
		Group:   "container.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ContainerCluster",
	}

	// ContainerNodePoolGVK is the GroupVersionKind for KCC ContainerNodePool resources.
	ContainerNodePoolGVK = schema.GroupVersionKind{
		Group:   "container.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ContainerNodePool",
	}
)
