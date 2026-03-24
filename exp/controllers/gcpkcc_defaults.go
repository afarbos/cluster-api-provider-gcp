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
	"k8s.io/utils/ptr"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
)

// --- Default functions (set only when field is empty/nil) ---

// applyNetworkDefaults sets default values on the KCC ComputeNetwork resource
// when fields are not already set by the user.
func applyNetworkDefaults(network *infrav1exp.GCPKCCNetworkResource, clusterName string) {
	if network.Metadata.Name == "" {
		network.Metadata.Name = clusterName
	}
	if network.Spec.AutoCreateSubnetworks == nil {
		network.Spec.AutoCreateSubnetworks = ptr.To(false)
	}
	if network.Spec.RoutingMode == "" {
		network.Spec.RoutingMode = "REGIONAL"
	}
}

// applySubnetworkDefaults sets default values on the KCC ComputeSubnetwork
// resource when fields are not already set by the user.
func applySubnetworkDefaults(subnet *infrav1exp.GCPKCCSubnetworkResource, clusterName, networkName string) {
	if subnet.Metadata.Name == "" {
		subnet.Metadata.Name = clusterName
	}
	if subnet.Spec.NetworkRef.Name == "" {
		subnet.Spec.NetworkRef.Name = networkName
	}
}

// applyContainerClusterDefaults sets default values on the KCC ContainerCluster
// resource when fields are not already set by the user.
func applyContainerClusterDefaults(cluster *infrav1exp.GCPKCCContainerClusterResource, clusterName, networkName, subnetworkName, subnetworkRegion string) {
	if cluster.Metadata.Name == "" {
		cluster.Metadata.Name = clusterName
	}
	if cluster.Spec.NetworkRef.Name == "" {
		cluster.Spec.NetworkRef.Name = networkName
	}
	if cluster.Spec.SubnetworkRef.Name == "" {
		cluster.Spec.SubnetworkRef.Name = subnetworkName
	}
	if cluster.Spec.InitialNodeCount == nil {
		cluster.Spec.InitialNodeCount = ptr.To[int32](1)
	}
	if cluster.Spec.NetworkingMode == "" {
		cluster.Spec.NetworkingMode = "VPC_NATIVE"
	}
	if cluster.Spec.RemoveDefaultNodePool == nil {
		cluster.Spec.RemoveDefaultNodePool = ptr.To(true)
	}
	if cluster.Spec.Location == "" && subnetworkRegion != "" {
		cluster.Spec.Location = subnetworkRegion
	}
}

// applyContainerNodePoolDefaults sets default values on the KCC
// ContainerNodePool resource when fields are not already set by the user.
func applyContainerNodePoolDefaults(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, name, clusterName, clusterLocation string) {
	if nodePool.Metadata.Name == "" {
		nodePool.Metadata.Name = name
	}
	if nodePool.Spec.ClusterRef.Name == "" {
		nodePool.Spec.ClusterRef.Name = clusterName
	}
	if nodePool.Spec.Location == "" && clusterLocation != "" {
		nodePool.Spec.Location = clusterLocation
	}
}

// --- Override functions (always applied, CAPI is source of truth) ---

// applySubnetworkCIDROverrides ensures that the pod and service CIDR secondary
// IP ranges are set on the subnetwork. CAPI-specified CIDRs always take
// precedence over user-configured values.
func applySubnetworkCIDROverrides(subnet *infrav1exp.GCPKCCSubnetworkResource, podCIDR, serviceCIDR string) {
	if podCIDR != "" {
		upsertSecondaryIPRange(&subnet.Spec.SecondaryIpRange, "pods", podCIDR)
	}
	if serviceCIDR != "" {
		upsertSecondaryIPRange(&subnet.Spec.SecondaryIpRange, "services", serviceCIDR)
	}
}

// upsertSecondaryIPRange updates the secondary IP range with the given
// rangeName if it already exists, or appends a new entry if it does not.
func upsertSecondaryIPRange(ranges *[]infrav1exp.KCCSecondaryIPRange, rangeName, ipCidrRange string) {
	for i, r := range *ranges {
		if r.RangeName == rangeName {
			(*ranges)[i].IpCidrRange = ipCidrRange
			return
		}
	}
	*ranges = append(*ranges, infrav1exp.KCCSecondaryIPRange{
		RangeName:   rangeName,
		IpCidrRange: ipCidrRange,
	})
}

// applyClusterVersionOverride sets the minimum master version on the KCC
// ContainerCluster. This is always applied when a version is specified by CAPI.
func applyClusterVersionOverride(cluster *infrav1exp.GCPKCCContainerClusterResource, version string) {
	if version != "" {
		cluster.Spec.MinMasterVersion = version
	}
}

// applyNodePoolReplicasOverride sets the initial node count on the KCC
// ContainerNodePool. This is always applied when replicas are specified by CAPI.
func applyNodePoolReplicasOverride(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, replicas *int32) {
	if replicas != nil {
		nodePool.Spec.InitialNodeCount = replicas
	}
}

// applyNodePoolVersionOverride sets the Kubernetes version on the KCC
// ContainerNodePool. This is always applied when a version is specified by CAPI.
func applyNodePoolVersionOverride(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, version string) {
	if version != "" {
		nodePool.Spec.Version = version
	}
}

// applyNodePoolFailureDomainOverride sets the node locations on the KCC
// ContainerNodePool. This is always applied when failure domains are specified.
func applyNodePoolFailureDomainOverride(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, failureDomains []string) {
	if len(failureDomains) > 0 {
		nodePool.Spec.NodeLocations = failureDomains
	}
}
