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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mergeAdditionalConfig unmarshals a RawExtension and merges each key into the
// base map. Values from AdditionalConfig take precedence, acting as a
// passthrough mechanism for KCC fields not covered by the typed schema.
func mergeAdditionalConfig(base map[string]interface{}, raw *runtime.RawExtension) error {
	if raw == nil || raw.Raw == nil {
		return nil
	}
	overlay := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &overlay); err != nil {
		return fmt.Errorf("unmarshalling additional config: %w", err)
	}
	for k, v := range overlay {
		base[k] = v
	}
	return nil
}

// ToUnstructuredComputeNetwork converts a GCPKCCNetworkResource to an
// unstructured KCC ComputeNetwork resource.
func ToUnstructuredComputeNetwork(res GCPKCCNetworkResource, namespace string) (*unstructured.Unstructured, error) {
	spec := map[string]interface{}{}
	if res.Spec.AutoCreateSubnetworks != nil {
		spec["autoCreateSubnetworks"] = *res.Spec.AutoCreateSubnetworks
	}
	if res.Spec.RoutingMode != "" {
		spec["routingMode"] = res.Spec.RoutingMode
	}
	if res.Spec.Description != "" {
		spec["description"] = res.Spec.Description
	}

	if res.Spec.AdditionalConfig != nil {
		if err := mergeAdditionalConfig(spec, res.Spec.AdditionalConfig); err != nil {
			return nil, fmt.Errorf("merging additional config: %w", err)
		}
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "compute.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ComputeNetwork",
	})
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if res.Metadata.Annotations != nil {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if res.Metadata.Labels != nil {
		u.SetLabels(res.Metadata.Labels)
	}
	if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
		return nil, fmt.Errorf("setting spec: %w", err)
	}

	return u, nil
}

// ToUnstructuredComputeSubnetwork converts a GCPKCCSubnetworkResource to an
// unstructured KCC ComputeSubnetwork resource.
func ToUnstructuredComputeSubnetwork(res GCPKCCSubnetworkResource, namespace string) (*unstructured.Unstructured, error) {
	spec := map[string]interface{}{}
	if res.Spec.IpCidrRange != "" {
		spec["ipCidrRange"] = res.Spec.IpCidrRange
	}
	if res.Spec.Region != "" {
		spec["region"] = res.Spec.Region
	}
	if res.Spec.Description != "" {
		spec["description"] = res.Spec.Description
	}
	if res.Spec.NetworkRef.Name != "" {
		spec["networkRef"] = map[string]interface{}{
			"name": res.Spec.NetworkRef.Name,
		}
	}
	if len(res.Spec.SecondaryIpRange) > 0 {
		ranges := make([]interface{}, 0, len(res.Spec.SecondaryIpRange))
		for _, r := range res.Spec.SecondaryIpRange {
			ranges = append(ranges, map[string]interface{}{
				"rangeName":   r.RangeName,
				"ipCidrRange": r.IpCidrRange,
			})
		}
		spec["secondaryIpRange"] = ranges
	}

	if res.Spec.AdditionalConfig != nil {
		if err := mergeAdditionalConfig(spec, res.Spec.AdditionalConfig); err != nil {
			return nil, fmt.Errorf("merging additional config: %w", err)
		}
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "compute.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ComputeSubnetwork",
	})
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if res.Metadata.Annotations != nil {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if res.Metadata.Labels != nil {
		u.SetLabels(res.Metadata.Labels)
	}
	if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
		return nil, fmt.Errorf("setting spec: %w", err)
	}

	return u, nil
}

// ToUnstructuredContainerCluster converts a GCPKCCContainerClusterResource to
// an unstructured KCC ContainerCluster resource.
func ToUnstructuredContainerCluster(res GCPKCCContainerClusterResource, namespace string) (*unstructured.Unstructured, error) {
	spec := map[string]interface{}{}
	if res.Spec.Location != "" {
		spec["location"] = res.Spec.Location
	}
	if res.Spec.NetworkingMode != "" {
		spec["networkingMode"] = res.Spec.NetworkingMode
	}
	if res.Spec.InitialNodeCount != nil {
		spec["initialNodeCount"] = int64(*res.Spec.InitialNodeCount)
	}
	if res.Spec.RemoveDefaultNodePool != nil {
		spec["removeDefaultNodePool"] = *res.Spec.RemoveDefaultNodePool
	}
	if res.Spec.NetworkRef.Name != "" {
		spec["networkRef"] = map[string]interface{}{
			"name": res.Spec.NetworkRef.Name,
		}
	}
	if res.Spec.SubnetworkRef.Name != "" {
		spec["subnetworkRef"] = map[string]interface{}{
			"name": res.Spec.SubnetworkRef.Name,
		}
	}
	if res.Spec.IpAllocationPolicy != nil {
		ipPolicy := map[string]interface{}{}
		if res.Spec.IpAllocationPolicy.ClusterSecondaryRangeName != "" {
			ipPolicy["clusterSecondaryRangeName"] = res.Spec.IpAllocationPolicy.ClusterSecondaryRangeName
		}
		if res.Spec.IpAllocationPolicy.ServicesSecondaryRangeName != "" {
			ipPolicy["servicesSecondaryRangeName"] = res.Spec.IpAllocationPolicy.ServicesSecondaryRangeName
		}
		spec["ipAllocationPolicy"] = ipPolicy
	}
	if res.Spec.MinMasterVersion != "" {
		spec["minMasterVersion"] = res.Spec.MinMasterVersion
	}
	if res.Spec.ReleaseChannel != nil {
		rc := map[string]interface{}{}
		if res.Spec.ReleaseChannel.Channel != "" {
			rc["channel"] = res.Spec.ReleaseChannel.Channel
		}
		spec["releaseChannel"] = rc
	}
	if res.Spec.LoggingService != "" {
		spec["loggingService"] = res.Spec.LoggingService
	}
	if res.Spec.MonitoringService != "" {
		spec["monitoringService"] = res.Spec.MonitoringService
	}
	if res.Spec.WorkloadIdentityConfig != nil {
		wic := map[string]interface{}{}
		if res.Spec.WorkloadIdentityConfig.WorkloadPool != "" {
			wic["workloadPool"] = res.Spec.WorkloadIdentityConfig.WorkloadPool
		}
		spec["workloadIdentityConfig"] = wic
	}
	if res.Spec.PrivateClusterConfig != nil {
		pcc := map[string]interface{}{}
		if res.Spec.PrivateClusterConfig.EnablePrivateEndpoint != nil {
			pcc["enablePrivateEndpoint"] = *res.Spec.PrivateClusterConfig.EnablePrivateEndpoint
		}
		if res.Spec.PrivateClusterConfig.EnablePrivateNodes != nil {
			pcc["enablePrivateNodes"] = *res.Spec.PrivateClusterConfig.EnablePrivateNodes
		}
		if res.Spec.PrivateClusterConfig.MasterIpv4CidrBlock != "" {
			pcc["masterIpv4CidrBlock"] = res.Spec.PrivateClusterConfig.MasterIpv4CidrBlock
		}
		spec["privateClusterConfig"] = pcc
	}
	if res.Spec.MasterAuthorizedNetworksConfig != nil {
		manc := map[string]interface{}{}
		if len(res.Spec.MasterAuthorizedNetworksConfig.CidrBlocks) > 0 {
			blocks := make([]interface{}, 0, len(res.Spec.MasterAuthorizedNetworksConfig.CidrBlocks))
			for _, b := range res.Spec.MasterAuthorizedNetworksConfig.CidrBlocks {
				block := map[string]interface{}{}
				if b.DisplayName != "" {
					block["displayName"] = b.DisplayName
				}
				if b.CidrBlock != "" {
					block["cidrBlock"] = b.CidrBlock
				}
				blocks = append(blocks, block)
			}
			manc["cidrBlocks"] = blocks
		}
		spec["masterAuthorizedNetworksConfig"] = manc
	}

	if res.Spec.AdditionalConfig != nil {
		if err := mergeAdditionalConfig(spec, res.Spec.AdditionalConfig); err != nil {
			return nil, fmt.Errorf("merging additional config: %w", err)
		}
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "container.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ContainerCluster",
	})
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if res.Metadata.Annotations != nil {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if res.Metadata.Labels != nil {
		u.SetLabels(res.Metadata.Labels)
	}
	if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
		return nil, fmt.Errorf("setting spec: %w", err)
	}

	return u, nil
}

// ToUnstructuredContainerNodePool converts a GCPKCCContainerNodePoolResource to
// an unstructured KCC ContainerNodePool resource.
func ToUnstructuredContainerNodePool(res GCPKCCContainerNodePoolResource, namespace string) (*unstructured.Unstructured, error) {
	spec := map[string]interface{}{}
	if res.Spec.Location != "" {
		spec["location"] = res.Spec.Location
	}
	if res.Spec.InitialNodeCount != nil {
		spec["initialNodeCount"] = int64(*res.Spec.InitialNodeCount)
	}
	if res.Spec.Version != "" {
		spec["version"] = res.Spec.Version
	}
	if len(res.Spec.NodeLocations) > 0 {
		locs := make([]interface{}, 0, len(res.Spec.NodeLocations))
		for _, l := range res.Spec.NodeLocations {
			locs = append(locs, l)
		}
		spec["nodeLocations"] = locs
	}
	if res.Spec.ClusterRef.Name != "" {
		spec["clusterRef"] = map[string]interface{}{
			"name": res.Spec.ClusterRef.Name,
		}
	}
	if res.Spec.NodeConfig != nil {
		nc := map[string]interface{}{}
		if res.Spec.NodeConfig.MachineType != "" {
			nc["machineType"] = res.Spec.NodeConfig.MachineType
		}
		if res.Spec.NodeConfig.DiskSizeGb != nil {
			nc["diskSizeGb"] = int64(*res.Spec.NodeConfig.DiskSizeGb)
		}
		if res.Spec.NodeConfig.DiskType != "" {
			nc["diskType"] = res.Spec.NodeConfig.DiskType
		}
		if res.Spec.NodeConfig.ImageType != "" {
			nc["imageType"] = res.Spec.NodeConfig.ImageType
		}
		if res.Spec.NodeConfig.Labels != nil {
			labels := map[string]interface{}{}
			for k, v := range res.Spec.NodeConfig.Labels {
				labels[k] = v
			}
			nc["labels"] = labels
		}
		if len(res.Spec.NodeConfig.Tags) > 0 {
			tags := make([]interface{}, 0, len(res.Spec.NodeConfig.Tags))
			for _, t := range res.Spec.NodeConfig.Tags {
				tags = append(tags, t)
			}
			nc["tags"] = tags
		}
		if len(res.Spec.NodeConfig.OauthScopes) > 0 {
			scopes := make([]interface{}, 0, len(res.Spec.NodeConfig.OauthScopes))
			for _, s := range res.Spec.NodeConfig.OauthScopes {
				scopes = append(scopes, s)
			}
			nc["oauthScopes"] = scopes
		}
		if res.Spec.NodeConfig.ServiceAccountRef != nil && res.Spec.NodeConfig.ServiceAccountRef.Name != "" {
			nc["serviceAccountRef"] = map[string]interface{}{
				"name": res.Spec.NodeConfig.ServiceAccountRef.Name,
			}
		}
		spec["nodeConfig"] = nc
	}
	if res.Spec.Autoscaling != nil {
		as := map[string]interface{}{}
		if res.Spec.Autoscaling.MinNodeCount != nil {
			as["minNodeCount"] = int64(*res.Spec.Autoscaling.MinNodeCount)
		}
		if res.Spec.Autoscaling.MaxNodeCount != nil {
			as["maxNodeCount"] = int64(*res.Spec.Autoscaling.MaxNodeCount)
		}
		if res.Spec.Autoscaling.Enabled != nil {
			as["enabled"] = *res.Spec.Autoscaling.Enabled
		}
		spec["autoscaling"] = as
	}
	if res.Spec.Management != nil {
		mgmt := map[string]interface{}{}
		if res.Spec.Management.AutoUpgrade != nil {
			mgmt["autoUpgrade"] = *res.Spec.Management.AutoUpgrade
		}
		if res.Spec.Management.AutoRepair != nil {
			mgmt["autoRepair"] = *res.Spec.Management.AutoRepair
		}
		spec["management"] = mgmt
	}

	if res.Spec.AdditionalConfig != nil {
		if err := mergeAdditionalConfig(spec, res.Spec.AdditionalConfig); err != nil {
			return nil, fmt.Errorf("merging additional config: %w", err)
		}
	}

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "container.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ContainerNodePool",
	})
	u.SetName(res.Metadata.Name)
	u.SetNamespace(namespace)
	if res.Metadata.Annotations != nil {
		u.SetAnnotations(res.Metadata.Annotations)
	}
	if res.Metadata.Labels != nil {
		u.SetLabels(res.Metadata.Labels)
	}
	if err := unstructured.SetNestedField(u.Object, spec, "spec"); err != nil {
		return nil, fmt.Errorf("setting spec: %w", err)
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
