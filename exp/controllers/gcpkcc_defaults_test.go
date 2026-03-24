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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
)

func rawExt(jsonStr string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(jsonStr)}
}

func specMap(t *testing.T, raw *runtime.RawExtension) map[string]interface{} {
	t.Helper()
	if raw == nil || raw.Raw == nil {
		return nil
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw.Raw, &m); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}
	return m
}

func TestApplyNetworkDefaults(t *testing.T) {
	tests := []struct {
		name        string
		network     infrav1exp.GCPKCCNetworkResource
		clusterName string
		wantName    string
		wantNS      string
		wantAutoSub bool
		wantRouting string
	}{
		{
			name:        "empty network",
			network:     infrav1exp.GCPKCCNetworkResource{},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantNS:      "default",
			wantAutoSub: false,
			wantRouting: "REGIONAL",
		},
		{
			name: "user values preserved",
			network: infrav1exp.GCPKCCNetworkResource{
				Spec: rawExt(`{"routingMode":"GLOBAL"}`),
			},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantNS:      "default",
			wantAutoSub: false,
			wantRouting: "GLOBAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applyNetworkDefaults(&tt.network, tt.clusterName, "default"); err != nil {
				t.Fatalf("applyNetworkDefaults() error = %v", err)
			}
			if got := tt.network.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			if got := tt.network.Metadata.Namespace; got != tt.wantNS {
				t.Errorf("Metadata.Namespace = %q, want %q", got, tt.wantNS)
			}
			m := specMap(t, tt.network.Spec)
			if got, _ := m["autoCreateSubnetworks"].(bool); got != tt.wantAutoSub {
				t.Errorf("autoCreateSubnetworks = %v, want %v", got, tt.wantAutoSub)
			}
			if got, _ := m["routingMode"].(string); got != tt.wantRouting {
				t.Errorf("routingMode = %q, want %q", got, tt.wantRouting)
			}
		})
	}
}

func TestApplySubnetworkDefaults(t *testing.T) {
	tests := []struct {
		name           string
		subnet         infrav1exp.GCPKCCSubnetworkResource
		clusterName    string
		networkName    string
		wantName       string
		wantNetworkRef string
	}{
		{
			name:           "empty subnetwork",
			subnet:         infrav1exp.GCPKCCSubnetworkResource{},
			clusterName:    "my-cluster",
			networkName:    "my-network",
			wantName:       "my-cluster",
			wantNetworkRef: "my-network",
		},
		{
			name: "user values preserved",
			subnet: infrav1exp.GCPKCCSubnetworkResource{
				Metadata: metav1.ObjectMeta{Name: "custom-subnet"},
			},
			clusterName:    "my-cluster",
			networkName:    "my-network",
			wantName:       "custom-subnet",
			wantNetworkRef: "my-network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applySubnetworkDefaults(&tt.subnet, tt.clusterName, tt.networkName, "default"); err != nil {
				t.Fatalf("applySubnetworkDefaults() error = %v", err)
			}
			if got := tt.subnet.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			m := specMap(t, tt.subnet.Spec)
			ref, _ := m["networkRef"].(map[string]interface{})
			if got, _ := ref["name"].(string); got != tt.wantNetworkRef {
				t.Errorf("networkRef.name = %q, want %q", got, tt.wantNetworkRef)
			}
		})
	}
}

func TestApplyContainerClusterDefaults(t *testing.T) {
	tests := []struct {
		name               string
		cluster            infrav1exp.GCPKCCContainerClusterResource
		clusterName        string
		networkName        string
		subnetworkName     string
		subnetworkRegion   string
		wantName           string
		wantNetworkingMode string
		wantInitialCount   float64
		wantRemoveDefault  bool
		wantLocation       string
	}{
		{
			name:               "empty cluster",
			cluster:            infrav1exp.GCPKCCContainerClusterResource{},
			clusterName:        "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			wantName:           "my-cluster",
			wantNetworkingMode: "VPC_NATIVE",
			wantInitialCount:   1,
			wantRemoveDefault:  true,
			wantLocation:       "us-central1",
		},
		{
			name: "user values preserved",
			cluster: infrav1exp.GCPKCCContainerClusterResource{
				Spec: rawExt(`{"networkingMode":"ROUTES"}`),
			},
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
			if err := applyContainerClusterDefaults(&tt.cluster, tt.clusterName, tt.networkName, tt.subnetworkName, tt.subnetworkRegion, "default"); err != nil {
				t.Fatalf("applyContainerClusterDefaults() error = %v", err)
			}
			if got := tt.cluster.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			m := specMap(t, tt.cluster.Spec)
			if got, _ := m["networkingMode"].(string); got != tt.wantNetworkingMode {
				t.Errorf("networkingMode = %q, want %q", got, tt.wantNetworkingMode)
			}
			if got, _ := m["initialNodeCount"].(float64); got != tt.wantInitialCount {
				t.Errorf("initialNodeCount = %v, want %v", got, tt.wantInitialCount)
			}
			if got, _ := m["removeDefaultNodePool"].(bool); got != tt.wantRemoveDefault {
				t.Errorf("removeDefaultNodePool = %v, want %v", got, tt.wantRemoveDefault)
			}
			if got, _ := m["location"].(string); got != tt.wantLocation {
				t.Errorf("location = %q, want %q", got, tt.wantLocation)
			}
		})
	}
}

func TestApplyContainerNodePoolDefaults(t *testing.T) {
	tests := []struct {
		name            string
		nodePool        infrav1exp.GCPKCCContainerNodePoolResource
		poolName        string
		clusterName     string
		clusterLocation string
		wantName        string
		wantClusterRef  string
		wantLocation    string
	}{
		{
			name:            "empty node pool",
			nodePool:        infrav1exp.GCPKCCContainerNodePoolResource{},
			poolName:        "pool-0",
			clusterName:     "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "pool-0",
			wantClusterRef:  "my-cluster",
			wantLocation:    "us-central1",
		},
		{
			name: "user values preserved",
			nodePool: infrav1exp.GCPKCCContainerNodePoolResource{
				Spec: rawExt(`{"location":"europe-west1"}`),
			},
			poolName:        "pool-0",
			clusterName:     "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "pool-0",
			wantClusterRef:  "my-cluster",
			wantLocation:    "europe-west1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applyContainerNodePoolDefaults(&tt.nodePool, tt.poolName, tt.clusterName, tt.clusterLocation, "default"); err != nil {
				t.Fatalf("applyContainerNodePoolDefaults() error = %v", err)
			}
			if got := tt.nodePool.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			m := specMap(t, tt.nodePool.Spec)
			ref, _ := m["clusterRef"].(map[string]interface{})
			if got, _ := ref["name"].(string); got != tt.wantClusterRef {
				t.Errorf("clusterRef.name = %q, want %q", got, tt.wantClusterRef)
			}
			if got, _ := m["location"].(string); got != tt.wantLocation {
				t.Errorf("location = %q, want %q", got, tt.wantLocation)
			}
		})
	}
}

func TestApplySubnetworkCIDROverrides(t *testing.T) {
	tests := []struct {
		name      string
		subnet    infrav1exp.GCPKCCSubnetworkResource
		podCIDR   string
		svcCIDR   string
		wantCount int
		wantPods  string
		wantSvcs  string
	}{
		{
			name:      "both CIDRs",
			subnet:    infrav1exp.GCPKCCSubnetworkResource{},
			podCIDR:   "10.0.0.0/14",
			svcCIDR:   "10.4.0.0/20",
			wantCount: 2,
			wantPods:  "10.0.0.0/14",
			wantSvcs:  "10.4.0.0/20",
		},
		{
			name:      "no CIDRs",
			subnet:    infrav1exp.GCPKCCSubnetworkResource{},
			podCIDR:   "",
			svcCIDR:   "",
			wantCount: 0,
		},
		{
			name: "existing ranges updated",
			subnet: infrav1exp.GCPKCCSubnetworkResource{
				Spec: rawExt(`{"secondaryIpRange":[{"rangeName":"pods","ipCidrRange":"10.0.0.0/16"}]}`),
			},
			podCIDR:   "10.0.0.0/14",
			svcCIDR:   "",
			wantCount: 1,
			wantPods:  "10.0.0.0/14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := applySubnetworkCIDROverrides(&tt.subnet, tt.podCIDR, tt.svcCIDR); err != nil {
				t.Fatalf("applySubnetworkCIDROverrides() error = %v", err)
			}
			m := specMap(t, tt.subnet.Spec)
			ranges, _ := m["secondaryIpRange"].([]interface{})
			if len(ranges) != tt.wantCount {
				t.Fatalf("secondaryIpRange length = %d, want %d", len(ranges), tt.wantCount)
			}
			for _, r := range ranges {
				rm, _ := r.(map[string]interface{})
				name, _ := rm["rangeName"].(string)
				cidr, _ := rm["ipCidrRange"].(string)
				switch name {
				case "pods":
					if cidr != tt.wantPods {
						t.Errorf("pods CIDR = %q, want %q", cidr, tt.wantPods)
					}
				case "services":
					if cidr != tt.wantSvcs {
						t.Errorf("services CIDR = %q, want %q", cidr, tt.wantSvcs)
					}
				}
			}
		})
	}
}

func TestApplyClusterVersionOverride(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "version set", version: "1.31", want: "1.31"},
		{name: "empty version", version: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &infrav1exp.GCPKCCContainerClusterResource{}
			if err := applyClusterVersionOverride(cluster, tt.version); err != nil {
				t.Fatalf("applyClusterVersionOverride() error = %v", err)
			}
			m := specMap(t, cluster.Spec)
			got, _ := m["minMasterVersion"].(string)
			if got != tt.want {
				t.Errorf("minMasterVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyNodePoolReplicasOverride(t *testing.T) {
	tests := []struct {
		name     string
		replicas *int32
		want     float64
		wantNil  bool
	}{
		{name: "replicas set", replicas: ptr.To[int32](3), want: 3},
		{name: "nil replicas", replicas: nil, wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodePool := &infrav1exp.GCPKCCContainerNodePoolResource{}
			if err := applyNodePoolReplicasOverride(nodePool, tt.replicas); err != nil {
				t.Fatalf("applyNodePoolReplicasOverride() error = %v", err)
			}
			m := specMap(t, nodePool.Spec)
			if tt.wantNil {
				if m != nil {
					if _, ok := m["initialNodeCount"]; ok {
						t.Errorf("initialNodeCount should not be set")
					}
				}
			} else {
				got, _ := m["initialNodeCount"].(float64)
				if got != tt.want {
					t.Errorf("initialNodeCount = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
