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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// KCC annotations used by the controllers.
	kccRemoveDefaultNodePoolAnnotation = "cnrm.cloud.google.com/remove-default-node-pool"
	kccStateIntoSpecAnnotation         = "cnrm.cloud.google.com/state-into-spec"
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

// getAutoscaling returns the autoscaling map from the node pool spec, or nil
// if autoscaling is not configured.
func getAutoscaling(spec map[string]interface{}) map[string]interface{} {
	raw, ok := spec["autoscaling"]
	if !ok {
		return nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
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
	// Force initialNodeCount to 1 — GKE requires >= 1 at creation time.
	// The default node pool is removed via the remove-default-node-pool
	// annotation; actual nodes are managed exclusively via MachinePool.
	spec["initialNodeCount"] = int64(1)
	setIfAbsent(spec, "networkingMode", "VPC_NATIVE")
	// VPC_NATIVE requires ipAllocationPolicy with secondary range names
	// matching those created on the subnetwork by CIDR overrides.
	// Only set when networkingMode is VPC_NATIVE.
	networkingMode, _ := spec["networkingMode"].(string)
	if networkingMode == "VPC_NATIVE" {
		if _, ok := spec["ipAllocationPolicy"]; !ok {
			spec["ipAllocationPolicy"] = map[string]interface{}{
				"clusterSecondaryRangeName":  "pods",
				"servicesSecondaryRangeName": "services",
			}
		}
	}

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

	// Set annotations: remove default node pool + enable state-into-spec merge
	// so KCC populates spec fields (like nodeLocations) from GCP state.
	annotations, _ := md["annotations"].(map[string]interface{})
	if annotations == nil {
		annotations = map[string]interface{}{}
		md["annotations"] = annotations
	}
	annotations[kccRemoveDefaultNodePoolAnnotation] = "true"
	annotations[kccStateIntoSpecAnnotation] = "merge"

	kccCP.Spec.ContainerCluster, err = mapToRaw(ccMap)
	return err
}

// applyMachinePoolDefaults sets defaults and CAPI overrides on the ContainerNodePool
// KCC resource for the given GCPKCCManagedMachinePool.
func applyMachinePoolDefaults(kccMMP *infrav1exp.GCPKCCManagedMachinePool, machinePool *clusterv1.MachinePool, controlPlane *infrav1exp.GCPKCCManagedControlPlane, existingCluster *unstructured.Unstructured, infraCluster *infrav1exp.GCPKCCManagedCluster) error {
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
	annotations[kccStateIntoSpecAnnotation] = "merge"

	spec := getRawSpec(npMap)
	if _, ok := spec["clusterRef"]; !ok {
		spec["clusterRef"] = map[string]interface{}{"name": controlPlane.Status.ClusterName}
	}

	// Default location from the live KCC ContainerCluster (populated via state-into-spec: merge).
	if _, ok := spec["location"]; !ok {
		loc, _, _ := unstructured.NestedString(existingCluster.Object, "spec", "location")
		if loc != "" {
			spec["location"] = loc
		}
	}

	// CAPI overrides: failureDomains → nodeLocations
	if len(machinePool.Spec.FailureDomains) > 0 {
		locs := make([]interface{}, len(machinePool.Spec.FailureDomains))
		for i, l := range machinePool.Spec.FailureDomains {
			locs[i] = l
		}
		spec["nodeLocations"] = locs
	}
	if machinePool.Spec.Template.Spec.Version != "" {
		spec["version"] = machinePool.Spec.Template.Spec.Version
	}

	// CAPI overrides: replicas → nodeCount (with autoscaling awareness)
	if machinePool.Spec.Replicas != nil {
		replicas := int64(*machinePool.Spec.Replicas)
		if autoscaling := getAutoscaling(spec); autoscaling != nil {
			// Autoscaling mode: map replicas to totalMinNodeCount.
			// The autoscaler manages actual node count; skip nodeCount override.
			autoscaling["totalMinNodeCount"] = replicas
		} else {
			// Determine zone count for per-zone division.
			// GKE's nodeCount/initialNodeCount is per-zone; CAPI replicas is total.
			var numZones int64
			switch {
			case len(machinePool.Spec.FailureDomains) > 0:
				numZones = int64(len(machinePool.Spec.FailureDomains))
			case len(infraCluster.Status.FailureDomains) > 0:
				numZones = int64(len(infraCluster.Status.FailureDomains))
			default:
				return fmt.Errorf("cannot determine zone count: neither MachinePool.spec.failureDomains nor infrastructure cluster failureDomains are set; waiting for infrastructure to be ready")
			}
			if replicas%numZones != 0 {
				return fmt.Errorf("replicas (%d) must be a multiple of zone count (%d)",
					replicas, numZones)
			}
			perZone := replicas / numZones
			spec["nodeCount"] = perZone
			setIfAbsent(spec, "initialNodeCount", perZone)
		}
	}

	kccMMP.Spec.NodePool, err = mapToRaw(npMap)
	return err
}
