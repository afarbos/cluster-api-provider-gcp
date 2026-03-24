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
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
)

func specToMap(raw *runtime.RawExtension) (map[string]interface{}, error) {
	if raw == nil || raw.Raw == nil {
		return map[string]interface{}{}, nil
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func mapToRawExtension(m map[string]interface{}) (*runtime.RawExtension, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: raw}, nil
}

func setIfAbsent(m map[string]interface{}, key string, value interface{}) {
	if _, ok := m[key]; !ok {
		m[key] = value
	}
}

// getSpecString extracts a string value from a RawExtension spec map.
func getSpecString(raw *runtime.RawExtension, key string) string {
	m, err := specToMap(raw)
	if err != nil {
		return ""
	}
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

// --- Default functions (set only when field is empty/nil) ---

// applyNetworkDefaults sets default values on the KCC ComputeNetwork resource
// when fields are not already set by the user.
func applyNetworkDefaults(network *infrav1exp.GCPKCCNetworkResource, clusterName, ownerNamespace string) error {
	if network.Metadata.Name == "" {
		network.Metadata.Name = clusterName
	}
	if network.Metadata.Namespace == "" {
		network.Metadata.Namespace = ownerNamespace
	}
	spec, err := specToMap(network.Spec)
	if err != nil {
		return fmt.Errorf("parsing network spec: %w", err)
	}
	setIfAbsent(spec, "autoCreateSubnetworks", false)
	setIfAbsent(spec, "routingMode", "REGIONAL")
	network.Spec, err = mapToRawExtension(spec)
	return err
}

// applySubnetworkDefaults sets default values on the KCC ComputeSubnetwork
// resource when fields are not already set by the user.
func applySubnetworkDefaults(subnet *infrav1exp.GCPKCCSubnetworkResource, clusterName, networkName, ownerNamespace string) error {
	if subnet.Metadata.Name == "" {
		subnet.Metadata.Name = clusterName
	}
	if subnet.Metadata.Namespace == "" {
		subnet.Metadata.Namespace = ownerNamespace
	}
	spec, err := specToMap(subnet.Spec)
	if err != nil {
		return fmt.Errorf("parsing subnetwork spec: %w", err)
	}
	if _, ok := spec["networkRef"]; !ok {
		spec["networkRef"] = map[string]interface{}{"name": networkName}
	}
	subnet.Spec, err = mapToRawExtension(spec)
	return err
}

// applyContainerClusterDefaults sets default values on the KCC ContainerCluster
// resource when fields are not already set by the user.
func applyContainerClusterDefaults(cluster *infrav1exp.GCPKCCContainerClusterResource, clusterName, networkName, subnetworkName, subnetworkRegion, ownerNamespace string) error {
	if cluster.Metadata.Name == "" {
		cluster.Metadata.Name = clusterName
	}
	if cluster.Metadata.Namespace == "" {
		cluster.Metadata.Namespace = ownerNamespace
	}
	spec, err := specToMap(cluster.Spec)
	if err != nil {
		return fmt.Errorf("parsing cluster spec: %w", err)
	}
	if _, ok := spec["networkRef"]; !ok {
		spec["networkRef"] = map[string]interface{}{"name": networkName}
	}
	if _, ok := spec["subnetworkRef"]; !ok {
		spec["subnetworkRef"] = map[string]interface{}{"name": subnetworkName}
	}
	setIfAbsent(spec, "initialNodeCount", int64(1))
	setIfAbsent(spec, "networkingMode", "VPC_NATIVE")
	setIfAbsent(spec, "removeDefaultNodePool", true)
	if _, ok := spec["location"]; !ok && subnetworkRegion != "" {
		spec["location"] = subnetworkRegion
	}
	cluster.Spec, err = mapToRawExtension(spec)
	return err
}

// applyContainerNodePoolDefaults sets default values on the KCC
// ContainerNodePool resource when fields are not already set by the user.
func applyContainerNodePoolDefaults(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, name, clusterName, clusterLocation, ownerNamespace string) error {
	if nodePool.Metadata.Name == "" {
		nodePool.Metadata.Name = name
	}
	if nodePool.Metadata.Namespace == "" {
		nodePool.Metadata.Namespace = ownerNamespace
	}
	// Enable state-into-spec merge so KCC populates spec.initialNodeCount
	// with the actual node count from GCP (used for status.replicas).
	if nodePool.Metadata.Annotations == nil {
		nodePool.Metadata.Annotations = map[string]string{}
	}
	nodePool.Metadata.Annotations["cnrm.cloud.google.com/state-into-spec"] = "merge"
	spec, err := specToMap(nodePool.Spec)
	if err != nil {
		return fmt.Errorf("parsing nodepool spec: %w", err)
	}
	if _, ok := spec["clusterRef"]; !ok {
		spec["clusterRef"] = map[string]interface{}{"name": clusterName}
	}
	if _, ok := spec["location"]; !ok && clusterLocation != "" {
		spec["location"] = clusterLocation
	}
	nodePool.Spec, err = mapToRawExtension(spec)
	return err
}

// --- Override functions (always applied, CAPI is source of truth) ---

// applySubnetworkCIDROverrides ensures that the pod and service CIDR secondary
// IP ranges are set on the subnetwork. CAPI-specified CIDRs always take
// precedence over user-configured values.
func applySubnetworkCIDROverrides(subnet *infrav1exp.GCPKCCSubnetworkResource, podCIDR, serviceCIDR string) error {
	if podCIDR == "" && serviceCIDR == "" {
		return nil
	}
	spec, err := specToMap(subnet.Spec)
	if err != nil {
		return fmt.Errorf("parsing subnetwork spec: %w", err)
	}
	ranges, _ := spec["secondaryIpRange"].([]interface{})
	if podCIDR != "" {
		ranges = upsertSecondaryIPRange(ranges, "pods", podCIDR)
	}
	if serviceCIDR != "" {
		ranges = upsertSecondaryIPRange(ranges, "services", serviceCIDR)
	}
	spec["secondaryIpRange"] = ranges
	subnet.Spec, err = mapToRawExtension(spec)
	return err
}

// upsertSecondaryIPRange updates the secondary IP range with the given
// rangeName if it already exists, or appends a new entry if it does not.
func upsertSecondaryIPRange(ranges []interface{}, rangeName, ipCidrRange string) []interface{} {
	for i, r := range ranges {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := m["rangeName"].(string); name == rangeName {
			m["ipCidrRange"] = ipCidrRange
			ranges[i] = m
			return ranges
		}
	}
	return append(ranges, map[string]interface{}{
		"rangeName":   rangeName,
		"ipCidrRange": ipCidrRange,
	})
}

// applyClusterVersionOverride sets the minimum master version on the KCC
// ContainerCluster. This is always applied when a version is specified by CAPI.
func applyClusterVersionOverride(cluster *infrav1exp.GCPKCCContainerClusterResource, version string) error {
	if version == "" {
		return nil
	}
	spec, err := specToMap(cluster.Spec)
	if err != nil {
		return fmt.Errorf("parsing cluster spec: %w", err)
	}
	spec["minMasterVersion"] = version
	cluster.Spec, err = mapToRawExtension(spec)
	return err
}

// applyNodePoolReplicasOverride sets the initial node count on the KCC
// ContainerNodePool. This is always applied when replicas are specified by CAPI.
func applyNodePoolReplicasOverride(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, replicas *int32) error {
	if replicas == nil {
		return nil
	}
	spec, err := specToMap(nodePool.Spec)
	if err != nil {
		return fmt.Errorf("parsing nodepool spec: %w", err)
	}
	spec["initialNodeCount"] = int64(*replicas)
	nodePool.Spec, err = mapToRawExtension(spec)
	return err
}

// applyNodePoolVersionOverride sets the Kubernetes version on the KCC
// ContainerNodePool. This is always applied when a version is specified by CAPI.
func applyNodePoolVersionOverride(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, version string) error {
	if version == "" {
		return nil
	}
	spec, err := specToMap(nodePool.Spec)
	if err != nil {
		return fmt.Errorf("parsing nodepool spec: %w", err)
	}
	spec["version"] = version
	nodePool.Spec, err = mapToRawExtension(spec)
	return err
}

// applyNodePoolFailureDomainOverride sets the node locations on the KCC
// ContainerNodePool. This is always applied when failure domains are specified.
func applyNodePoolFailureDomainOverride(nodePool *infrav1exp.GCPKCCContainerNodePoolResource, failureDomains []string) error {
	if len(failureDomains) == 0 {
		return nil
	}
	spec, err := specToMap(nodePool.Spec)
	if err != nil {
		return fmt.Errorf("parsing nodepool spec: %w", err)
	}
	locs := make([]interface{}, len(failureDomains))
	for i, l := range failureDomains {
		locs[i] = l
	}
	spec["nodeLocations"] = locs
	nodePool.Spec, err = mapToRawExtension(spec)
	return err
}
