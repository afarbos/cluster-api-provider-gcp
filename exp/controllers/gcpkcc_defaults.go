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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// rawToMap converts a RawExtension to a map.
func rawToMap(raw *runtime.RawExtension) (map[string]interface{}, error) {
	if raw == nil || raw.Raw == nil {
		return map[string]interface{}{}, nil
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// mapToRaw converts a map to a RawExtension.
func mapToRaw(m map[string]interface{}) (*runtime.RawExtension, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{Raw: raw}, nil
}

// getRawMetadata returns the metadata map from a raw KCC object, or an empty map.
func getRawMetadata(m map[string]interface{}) map[string]interface{} {
	md, _ := m["metadata"].(map[string]interface{})
	if md == nil {
		md = map[string]interface{}{}
		m["metadata"] = md
	}
	return md
}

// getRawSpec returns the spec map from a raw KCC object, or an empty map.
func getRawSpec(m map[string]interface{}) map[string]interface{} {
	spec, _ := m["spec"].(map[string]interface{})
	if spec == nil {
		spec = map[string]interface{}{}
		m["spec"] = spec
	}
	return spec
}

// getRawMetadataString reads a string field from metadata.
func getRawMetadataString(m map[string]interface{}, key string) string {
	md := getRawMetadata(m)
	v, _ := md[key].(string)
	return v
}

// getRawSpecString reads a string field from spec.
func getRawSpecString(m map[string]interface{}, key string) string {
	spec := getRawSpec(m)
	v, _ := spec[key].(string)
	return v
}

// getRawName is a convenience function that extracts the metadata.name from a
// raw KCC resource. Used in delete paths where conversion hasn't occurred.
func getRawName(raw *runtime.RawExtension) string {
	m, _ := rawToMap(raw)
	return getRawMetadataString(m, "name")
}

func setIfAbsent(m map[string]interface{}, key string, value interface{}) {
	if _, ok := m[key]; !ok {
		m[key] = value
	}
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

// applyClusterDefaults sets defaults and CAPI overrides on the network and
// subnetwork KCC resources for the given GCPKCCManagedCluster.
func applyClusterDefaults(kccCluster *infrav1exp.GCPKCCManagedCluster, cluster *clusterv1.Cluster) error {
	// --- Network ---
	netMap, err := rawToMap(kccCluster.Spec.Network)
	if err != nil {
		return fmt.Errorf("parsing network: %w", err)
	}
	md := getRawMetadata(netMap)
	if md["name"] == nil || md["name"] == "" {
		md["name"] = cluster.Name
	}
	if md["namespace"] == nil || md["namespace"] == "" {
		md["namespace"] = kccCluster.Namespace
	}
	spec := getRawSpec(netMap)
	setIfAbsent(spec, "autoCreateSubnetworks", false)
	setIfAbsent(spec, "routingMode", "REGIONAL")
	kccCluster.Spec.Network, err = mapToRaw(netMap)
	if err != nil {
		return err
	}

	// --- Subnetwork ---
	subMap, err := rawToMap(kccCluster.Spec.Subnetwork)
	if err != nil {
		return fmt.Errorf("parsing subnetwork: %w", err)
	}
	md = getRawMetadata(subMap)
	if md["name"] == nil || md["name"] == "" {
		md["name"] = cluster.Name
	}
	if md["namespace"] == nil || md["namespace"] == "" {
		md["namespace"] = kccCluster.Namespace
	}
	spec = getRawSpec(subMap)
	networkName := getRawMetadataString(netMap, "name")
	if _, ok := spec["networkRef"]; !ok {
		spec["networkRef"] = map[string]interface{}{"name": networkName}
	}

	// CIDR overrides from Cluster.Spec.ClusterNetwork
	{
		ranges, _ := spec["secondaryIpRange"].([]interface{})
		if len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
			ranges = upsertSecondaryIPRange(ranges, "pods", cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0])
		}
		if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
			ranges = upsertSecondaryIPRange(ranges, "services", cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0])
		}
		if len(ranges) > 0 {
			spec["secondaryIpRange"] = ranges
		}
	}

	kccCluster.Spec.Subnetwork, err = mapToRaw(subMap)
	return err
}

// applyControlPlaneDefaults sets defaults and CAPI overrides on the ContainerCluster
// KCC resource for the given GCPKCCManagedControlPlane.
func applyControlPlaneDefaults(kccCP *infrav1exp.GCPKCCManagedControlPlane, cluster *clusterv1.Cluster, infraCluster *infrav1exp.GCPKCCManagedCluster) error {
	ccMap, err := rawToMap(kccCP.Spec.ContainerCluster)
	if err != nil {
		return fmt.Errorf("parsing container cluster: %w", err)
	}
	md := getRawMetadata(ccMap)
	if md["name"] == nil || md["name"] == "" {
		md["name"] = cluster.Name
	}
	if md["namespace"] == nil || md["namespace"] == "" {
		md["namespace"] = kccCP.Namespace
	}

	spec := getRawSpec(ccMap)
	if _, ok := spec["networkRef"]; !ok {
		spec["networkRef"] = map[string]interface{}{"name": infraCluster.Status.NetworkName}
	}
	if _, ok := spec["subnetworkRef"]; !ok {
		spec["subnetworkRef"] = map[string]interface{}{"name": infraCluster.Status.SubnetworkName}
	}
	setIfAbsent(spec, "initialNodeCount", int64(1))
	setIfAbsent(spec, "networkingMode", "VPC_NATIVE")
	setIfAbsent(spec, "removeDefaultNodePool", true)

	// Default location from subnetwork region
	if _, ok := spec["location"]; !ok {
		subMap, _ := rawToMap(infraCluster.Spec.Subnetwork)
		if region := getRawSpecString(subMap, "region"); region != "" {
			spec["location"] = region
		}
	}

	// Version override: CAPI version -> minMasterVersion
	if kccCP.Spec.Version != nil && *kccCP.Spec.Version != "" {
		spec["minMasterVersion"] = *kccCP.Spec.Version
	}

	// Set remove-default-node-pool annotation
	annotations, _ := md["annotations"].(map[string]interface{})
	if annotations == nil {
		annotations = map[string]interface{}{}
		md["annotations"] = annotations
	}
	annotations["cnrm.cloud.google.com/remove-default-node-pool"] = "true"

	kccCP.Spec.ContainerCluster, err = mapToRaw(ccMap)
	return err
}

// applyMachinePoolDefaults sets defaults and CAPI overrides on the ContainerNodePool
// KCC resource for the given GCPKCCManagedMachinePool.
func applyMachinePoolDefaults(kccMMP *infrav1exp.GCPKCCManagedMachinePool, machinePool *clusterv1.MachinePool, controlPlane *infrav1exp.GCPKCCManagedControlPlane) error {
	npMap, err := rawToMap(kccMMP.Spec.NodePool)
	if err != nil {
		return fmt.Errorf("parsing nodepool: %w", err)
	}
	md := getRawMetadata(npMap)
	if md["name"] == nil || md["name"] == "" {
		md["name"] = kccMMP.Name
	}
	if md["namespace"] == nil || md["namespace"] == "" {
		md["namespace"] = kccMMP.Namespace
	}

	// Enable state-into-spec merge for actual node count in status.replicas
	annotations, _ := md["annotations"].(map[string]interface{})
	if annotations == nil {
		annotations = map[string]interface{}{}
		md["annotations"] = annotations
	}
	annotations["cnrm.cloud.google.com/state-into-spec"] = "merge"

	spec := getRawSpec(npMap)
	if _, ok := spec["clusterRef"]; !ok {
		spec["clusterRef"] = map[string]interface{}{"name": controlPlane.Status.ClusterName}
	}

	// Default location from cluster
	if _, ok := spec["location"]; !ok {
		cpMap, _ := rawToMap(controlPlane.Spec.ContainerCluster)
		if loc := getRawSpecString(cpMap, "location"); loc != "" {
			spec["location"] = loc
		}
	}

	// CAPI overrides
	if machinePool.Spec.Replicas != nil {
		spec["initialNodeCount"] = int64(*machinePool.Spec.Replicas)
	}
	if machinePool.Spec.Template.Spec.Version != "" {
		spec["version"] = machinePool.Spec.Template.Spec.Version
	}
	if len(machinePool.Spec.FailureDomains) > 0 {
		locs := make([]interface{}, len(machinePool.Spec.FailureDomains))
		for i, l := range machinePool.Spec.FailureDomains {
			locs[i] = l
		}
		spec["nodeLocations"] = locs
	}

	kccMMP.Spec.NodePool, err = mapToRaw(npMap)
	return err
}
