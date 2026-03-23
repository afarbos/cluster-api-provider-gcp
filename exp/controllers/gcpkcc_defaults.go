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

package controllers

import (
	kcccomputev1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/compute/v1beta1"
	kcccontainerv1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/container/v1beta1"
	kcck8sv1alpha1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	"k8s.io/utils/ptr"
)

// applyNetworkDefaults fills empty fields on a ComputeNetwork.
func applyNetworkDefaults(network *kcccomputev1beta1.ComputeNetwork, clusterName string) {
	if network.Name == "" {
		network.Name = clusterName
	}
	if network.Spec.AutoCreateSubnetworks == nil {
		network.Spec.AutoCreateSubnetworks = ptr.To(false)
	}
	if network.Spec.RoutingMode == nil {
		network.Spec.RoutingMode = ptr.To("REGIONAL")
	}
}

// applySubnetworkDefaults fills empty fields on a ComputeSubnetwork.
func applySubnetworkDefaults(subnet *kcccomputev1beta1.ComputeSubnetwork, clusterName string, networkName string) {
	if subnet.Name == "" {
		subnet.Name = clusterName
	}
	if subnet.Spec.NetworkRef.Name == "" && subnet.Spec.NetworkRef.External == "" {
		subnet.Spec.NetworkRef.Name = networkName
	}
}

// applyContainerClusterDefaults fills empty fields on a ContainerCluster.
func applyContainerClusterDefaults(
	cluster *kcccontainerv1beta1.ContainerCluster,
	capiClusterName string,
	networkName string,
	subnetworkName string,
	subnetworkRegion string,
	hasSecondaryRanges bool,
) {
	if cluster.Name == "" {
		cluster.Name = capiClusterName
	}
	if cluster.Spec.NetworkRef == nil {
		cluster.Spec.NetworkRef = &kcck8sv1alpha1.ResourceRef{Name: networkName}
	}
	if cluster.Spec.SubnetworkRef == nil {
		cluster.Spec.SubnetworkRef = &kcck8sv1alpha1.ResourceRef{Name: subnetworkName}
	}
	if cluster.Spec.Location == "" && subnetworkRegion != "" {
		cluster.Spec.Location = subnetworkRegion
	}
	if cluster.Spec.InitialNodeCount == nil {
		cluster.Spec.InitialNodeCount = ptr.To(int64(1))
	}
	if cluster.Spec.NetworkingMode == nil {
		cluster.Spec.NetworkingMode = ptr.To("VPC_NATIVE")
	}
	if cluster.Spec.IpAllocationPolicy == nil && hasSecondaryRanges {
		cluster.Spec.IpAllocationPolicy = &kcccontainerv1beta1.ClusterIpAllocationPolicy{
			ClusterSecondaryRangeName:  ptr.To("pods"),
			ServicesSecondaryRangeName: ptr.To("services"),
		}
	}
	// Set annotation to remove default node pool.
	const removeDefaultNodePoolAnnotation = "cnrm.cloud.google.com/remove-default-node-pool"
	if cluster.Annotations == nil {
		cluster.Annotations = map[string]string{}
	}
	if _, exists := cluster.Annotations[removeDefaultNodePoolAnnotation]; !exists {
		cluster.Annotations[removeDefaultNodePoolAnnotation] = "true"
	}
}

// applyContainerClusterOverrides forces CAPI fields onto the ContainerCluster.
func applyContainerClusterOverrides(cluster *kcccontainerv1beta1.ContainerCluster, version *string) {
	if version != nil && *version != "" {
		cluster.Spec.MinMasterVersion = version
	}
}

// applyContainerNodePoolDefaults fills empty fields on a ContainerNodePool.
func applyContainerNodePoolDefaults(
	nodePool *kcccontainerv1beta1.ContainerNodePool,
	machinePoolName string,
	capiClusterName string,
	clusterLocation string,
) {
	if nodePool.Name == "" {
		nodePool.Name = machinePoolName
	}
	if nodePool.Spec.ClusterRef.Name == "" && nodePool.Spec.ClusterRef.External == "" {
		nodePool.Spec.ClusterRef.Name = capiClusterName
	}
	if nodePool.Spec.Location == "" {
		nodePool.Spec.Location = clusterLocation
	}
}

// applyContainerNodePoolOverrides forces CAPI MachinePool fields onto the ContainerNodePool.
func applyContainerNodePoolOverrides(
	nodePool *kcccontainerv1beta1.ContainerNodePool,
	replicas *int32,
	version *string,
	failureDomains []string,
) {
	if replicas != nil {
		nodePool.Spec.InitialNodeCount = ptr.To(int64(*replicas))
	}
	if version != nil && *version != "" {
		nodePool.Spec.Version = version
	}
	if len(failureDomains) > 0 {
		nodePool.Spec.NodeLocations = failureDomains
	}
}
