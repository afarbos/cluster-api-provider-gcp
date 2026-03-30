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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func rawKCC(jsonStr string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(jsonStr)}
}

func rawMap(t *testing.T, raw *runtime.RawExtension) map[string]interface{} {
	t.Helper()
	if raw == nil || raw.Raw == nil {
		return nil
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	return m
}

func TestApplyClusterDefaults(t *testing.T) {
	tests := []struct {
		name            string
		network         string // raw JSON or ""
		subnetwork      string
		clusterName     string
		podCIDR         string
		svcCIDR         string
		wantNetName     string
		wantNetNS       string
		wantSubName     string
		wantRoutingMode string
		wantAutoSub     bool
	}{
		{
			name:            "empty resources get defaults",
			network:         "",
			subnetwork:      "",
			clusterName:     "my-cluster",
			wantNetName:     "my-cluster",
			wantNetNS:       "default",
			wantSubName:     "my-cluster",
			wantRoutingMode: "REGIONAL",
			wantAutoSub:     false,
		},
		{
			name:            "user-specified routing mode preserved",
			network:         `{"spec":{"routingMode":"GLOBAL"}}`,
			subnetwork:      "",
			clusterName:     "my-cluster",
			wantNetName:     "my-cluster",
			wantNetNS:       "default",
			wantSubName:     "my-cluster",
			wantRoutingMode: "GLOBAL",
			wantAutoSub:     false,
		},
		{
			name:            "user-specified name preserved",
			network:         `{"metadata":{"name":"custom-net"}}`,
			subnetwork:      `{"metadata":{"name":"custom-sub"}}`,
			clusterName:     "my-cluster",
			wantNetName:     "custom-net",
			wantNetNS:       "default",
			wantSubName:     "custom-sub",
			wantRoutingMode: "REGIONAL",
			wantAutoSub:     false,
		},
		{
			name:        "CIDR overrides applied",
			network:     "",
			subnetwork:  "",
			clusterName: "my-cluster",
			podCIDR:     "10.0.0.0/14",
			svcCIDR:     "10.4.0.0/20",
			wantNetName: "my-cluster",
			wantNetNS:   "default",
			wantSubName: "my-cluster",
			wantRoutingMode: "REGIONAL",
			wantAutoSub: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kccCluster := &infrav1exp.GCPKCCManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			}
			if tt.network != "" {
				kccCluster.Spec.Network = rawKCC(tt.network)
			}
			if tt.subnetwork != "" {
				kccCluster.Spec.Subnetwork = rawKCC(tt.subnetwork)
			}

			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: tt.clusterName},
			}
			if tt.podCIDR != "" {
				cluster.Spec.ClusterNetwork.Pods = clusterv1.NetworkRanges{CIDRBlocks: []string{tt.podCIDR}}
			}
			if tt.svcCIDR != "" {
				cluster.Spec.ClusterNetwork.Services = clusterv1.NetworkRanges{CIDRBlocks: []string{tt.svcCIDR}}
			}

			if err := applyClusterDefaults(kccCluster, cluster); err != nil {
				t.Fatalf("applyClusterDefaults() error = %v", err)
			}

			netMap := rawMap(t, kccCluster.Spec.Network)
			netMD, _ := netMap["metadata"].(map[string]interface{})
			netSpec, _ := netMap["spec"].(map[string]interface{})

			if got, _ := netMD["name"].(string); got != tt.wantNetName {
				t.Errorf("network metadata.name = %q, want %q", got, tt.wantNetName)
			}
			if got, _ := netMD["namespace"].(string); got != tt.wantNetNS {
				t.Errorf("network metadata.namespace = %q, want %q", got, tt.wantNetNS)
			}
			if got, _ := netSpec["autoCreateSubnetworks"].(bool); got != tt.wantAutoSub {
				t.Errorf("autoCreateSubnetworks = %v, want %v", got, tt.wantAutoSub)
			}
			if got, _ := netSpec["routingMode"].(string); got != tt.wantRoutingMode {
				t.Errorf("routingMode = %q, want %q", got, tt.wantRoutingMode)
			}

			subMap := rawMap(t, kccCluster.Spec.Subnetwork)
			subMD, _ := subMap["metadata"].(map[string]interface{})
			if got, _ := subMD["name"].(string); got != tt.wantSubName {
				t.Errorf("subnetwork metadata.name = %q, want %q", got, tt.wantSubName)
			}

			// Check CIDR overrides.
			if tt.podCIDR != "" || tt.svcCIDR != "" {
				subSpec, _ := subMap["spec"].(map[string]interface{})
				ranges, _ := subSpec["secondaryIpRange"].([]interface{})
				for _, r := range ranges {
					rm, _ := r.(map[string]interface{})
					name, _ := rm["rangeName"].(string)
					cidr, _ := rm["ipCidrRange"].(string)
					switch name {
					case "pods":
						if cidr != tt.podCIDR {
							t.Errorf("pods CIDR = %q, want %q", cidr, tt.podCIDR)
						}
					case "services":
						if cidr != tt.svcCIDR {
							t.Errorf("services CIDR = %q, want %q", cidr, tt.svcCIDR)
						}
					}
				}
			}
		})
	}
}

func TestApplyControlPlaneDefaults(t *testing.T) {
	tests := []struct {
		name               string
		containerCluster   string // raw JSON or ""
		clusterName        string
		version            *string
		networkName        string
		subnetworkName     string
		subnetworkRegion   string
		wantName           string
		wantNetworkingMode string
		wantInitialCount   float64
		wantRemoveDefault  bool
		wantLocation       string
		wantVersion        string
	}{
		{
			name:               "empty cluster gets defaults",
			containerCluster:   "",
			clusterName:        "my-cluster",
			version:            ptr.To("1.31"),
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			wantName:           "my-cluster",
			wantNetworkingMode: "VPC_NATIVE",
			wantInitialCount:   1,
			wantRemoveDefault:  true,
			wantLocation:       "us-central1",
			wantVersion:        "1.31",
		},
		{
			name:               "user values preserved",
			containerCluster:   `{"spec":{"networkingMode":"ROUTES"}}`,
			clusterName:        "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			wantName:           "my-cluster",
			wantNetworkingMode: "ROUTES",
			wantInitialCount:   1,
			wantRemoveDefault:  true,
			wantLocation:       "us-central1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kccCP := &infrav1exp.GCPKCCManagedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cp",
					Namespace: "default",
				},
				Spec: infrav1exp.GCPKCCManagedControlPlaneSpec{
					Version: tt.version,
				},
			}
			if tt.containerCluster != "" {
				kccCP.Spec.ContainerCluster = rawKCC(tt.containerCluster)
			}

			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: tt.clusterName},
			}

			infraCluster := &infrav1exp.GCPKCCManagedCluster{
				Spec: infrav1exp.GCPKCCManagedClusterSpec{
					Subnetwork: rawKCC(`{"spec":{"region":"` + tt.subnetworkRegion + `"}}`),
				},
				Status: infrav1exp.GCPKCCManagedClusterStatus{
					NetworkName:    tt.networkName,
					SubnetworkName: tt.subnetworkName,
				},
			}

			if err := applyControlPlaneDefaults(kccCP, cluster, infraCluster); err != nil {
				t.Fatalf("applyControlPlaneDefaults() error = %v", err)
			}

			ccMap := rawMap(t, kccCP.Spec.ContainerCluster)
			md, _ := ccMap["metadata"].(map[string]interface{})
			spec, _ := ccMap["spec"].(map[string]interface{})

			if got, _ := md["name"].(string); got != tt.wantName {
				t.Errorf("metadata.name = %q, want %q", got, tt.wantName)
			}
			if got, _ := spec["networkingMode"].(string); got != tt.wantNetworkingMode {
				t.Errorf("networkingMode = %q, want %q", got, tt.wantNetworkingMode)
			}
			if got, _ := spec["initialNodeCount"].(float64); got != tt.wantInitialCount {
				t.Errorf("initialNodeCount = %v, want %v", got, tt.wantInitialCount)
			}
			if got, _ := spec["location"].(string); got != tt.wantLocation {
				t.Errorf("location = %q, want %q", got, tt.wantLocation)
			}
			if tt.wantVersion != "" {
				if got, _ := spec["minMasterVersion"].(string); got != tt.wantVersion {
					t.Errorf("minMasterVersion = %q, want %q", got, tt.wantVersion)
				}
			}

			// Check remove-default-node-pool annotation.
			annotations, _ := md["annotations"].(map[string]interface{})
			if got, _ := annotations["cnrm.cloud.google.com/remove-default-node-pool"].(string); got != "true" {
				t.Errorf("remove-default-node-pool annotation = %q, want %q", got, "true")
			}
		})
	}
}

func TestApplyMachinePoolDefaults(t *testing.T) {
	tests := []struct {
		name            string
		nodePool        string // raw JSON or ""
		mmpName         string
		clusterName     string
		clusterLocation string
		replicas        *int32
		version         string
		failureDomains  []string
		wantName        string
		wantClusterRef  string
		wantLocation    string
		wantReplicas    float64
		wantVersion     string
	}{
		{
			name:            "empty node pool gets defaults",
			nodePool:        "",
			mmpName:         "pool-0",
			clusterName:     "my-cluster",
			clusterLocation: "us-central1",
			replicas:        ptr.To[int32](3),
			version:         "1.31",
			wantName:        "pool-0",
			wantClusterRef:  "my-cluster",
			wantLocation:    "us-central1",
			wantReplicas:    3,
			wantVersion:     "1.31",
		},
		{
			name:            "user location preserved",
			nodePool:        `{"spec":{"location":"europe-west1"}}`,
			mmpName:         "pool-0",
			clusterName:     "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "pool-0",
			wantClusterRef:  "my-cluster",
			wantLocation:    "europe-west1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kccMMP := &infrav1exp.GCPKCCManagedMachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.mmpName,
					Namespace: "default",
				},
			}
			if tt.nodePool != "" {
				kccMMP.Spec.NodePool = rawKCC(tt.nodePool)
			}

			machinePool := &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					Replicas:       tt.replicas,
					FailureDomains: tt.failureDomains,
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: tt.version,
						},
					},
				},
			}

			kccCP := &infrav1exp.GCPKCCManagedControlPlane{
				Status: infrav1exp.GCPKCCManagedControlPlaneStatus{
					ClusterName: tt.clusterName,
				},
			}

			existingCluster := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"location": tt.clusterLocation,
					},
				},
			}

			infraCluster := &infrav1exp.GCPKCCManagedCluster{}
			if err := applyMachinePoolDefaults(kccMMP, machinePool, kccCP, existingCluster, infraCluster); err != nil {
				t.Fatalf("applyMachinePoolDefaults() error = %v", err)
			}

			npMap := rawMap(t, kccMMP.Spec.NodePool)
			md, _ := npMap["metadata"].(map[string]interface{})
			spec, _ := npMap["spec"].(map[string]interface{})

			if got, _ := md["name"].(string); got != tt.wantName {
				t.Errorf("metadata.name = %q, want %q", got, tt.wantName)
			}

			ref, _ := spec["clusterRef"].(map[string]interface{})
			if got, _ := ref["name"].(string); got != tt.wantClusterRef {
				t.Errorf("clusterRef.name = %q, want %q", got, tt.wantClusterRef)
			}
			if got, _ := spec["location"].(string); got != tt.wantLocation {
				t.Errorf("location = %q, want %q", got, tt.wantLocation)
			}
			if tt.wantReplicas > 0 {
				if got, _ := spec["initialNodeCount"].(float64); got != tt.wantReplicas {
					t.Errorf("initialNodeCount = %v, want %v", got, tt.wantReplicas)
				}
			}
			if tt.wantVersion != "" {
				if got, _ := spec["version"].(string); got != tt.wantVersion {
					t.Errorf("version = %q, want %q", got, tt.wantVersion)
				}
			}

			// Check state-into-spec annotation.
			annotations, _ := md["annotations"].(map[string]interface{})
			if got, _ := annotations["cnrm.cloud.google.com/state-into-spec"].(string); got != "merge" {
				t.Errorf("state-into-spec annotation = %q, want %q", got, "merge")
			}
		})
	}
}

func TestGetRawName(t *testing.T) {
	tests := []struct {
		name string
		raw  *runtime.RawExtension
		want string
	}{
		{
			name: "has name",
			raw:  rawKCC(`{"metadata":{"name":"test-resource"}}`),
			want: "test-resource",
		},
		{
			name: "nil raw",
			raw:  nil,
			want: "",
		},
		{
			name: "no metadata",
			raw:  rawKCC(`{"spec":{"foo":"bar"}}`),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRawName(tt.raw)
			if got != tt.want {
				t.Errorf("getRawName() = %q, want %q", got, tt.want)
			}
		})
	}
}
