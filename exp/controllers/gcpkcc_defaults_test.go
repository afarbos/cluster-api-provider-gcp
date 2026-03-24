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
	"testing"

	"k8s.io/utils/ptr"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
)

func TestApplyNetworkDefaults(t *testing.T) {
	tests := []struct {
		name        string
		network     infrav1exp.GCPKCCNetworkResource
		clusterName string
		wantName    string
		wantAutoSub bool
		wantRouting string
	}{
		{
			name:        "empty network",
			network:     infrav1exp.GCPKCCNetworkResource{},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantAutoSub: false,
			wantRouting: "REGIONAL",
		},
		{
			name: "user values preserved",
			network: infrav1exp.GCPKCCNetworkResource{
				Spec: infrav1exp.GCPKCCComputeNetworkSpec{
					RoutingMode: "GLOBAL",
				},
			},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantAutoSub: false,
			wantRouting: "GLOBAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyNetworkDefaults(&tt.network, tt.clusterName)
			if got := tt.network.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			if got := *tt.network.Spec.AutoCreateSubnetworks; got != tt.wantAutoSub {
				t.Errorf("Spec.AutoCreateSubnetworks = %v, want %v", got, tt.wantAutoSub)
			}
			if got := tt.network.Spec.RoutingMode; got != tt.wantRouting {
				t.Errorf("Spec.RoutingMode = %q, want %q", got, tt.wantRouting)
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
				Metadata: infrav1exp.KCCObjectMeta{Name: "custom-subnet"},
			},
			clusterName:    "my-cluster",
			networkName:    "my-network",
			wantName:       "custom-subnet",
			wantNetworkRef: "my-network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applySubnetworkDefaults(&tt.subnet, tt.clusterName, tt.networkName)
			if got := tt.subnet.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			if got := tt.subnet.Spec.NetworkRef.Name; got != tt.wantNetworkRef {
				t.Errorf("Spec.NetworkRef.Name = %q, want %q", got, tt.wantNetworkRef)
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
		wantInitialCount   int32
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
				Spec: infrav1exp.GCPKCCContainerClusterSpec{
					NetworkingMode: "ROUTES",
				},
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
			applyContainerClusterDefaults(&tt.cluster, tt.clusterName, tt.networkName, tt.subnetworkName, tt.subnetworkRegion)
			if got := tt.cluster.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			if got := tt.cluster.Spec.NetworkingMode; got != tt.wantNetworkingMode {
				t.Errorf("Spec.NetworkingMode = %q, want %q", got, tt.wantNetworkingMode)
			}
			if got := *tt.cluster.Spec.InitialNodeCount; got != tt.wantInitialCount {
				t.Errorf("Spec.InitialNodeCount = %d, want %d", got, tt.wantInitialCount)
			}
			if got := *tt.cluster.Spec.RemoveDefaultNodePool; got != tt.wantRemoveDefault {
				t.Errorf("Spec.RemoveDefaultNodePool = %v, want %v", got, tt.wantRemoveDefault)
			}
			if got := tt.cluster.Spec.Location; got != tt.wantLocation {
				t.Errorf("Spec.Location = %q, want %q", got, tt.wantLocation)
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
				Spec: infrav1exp.GCPKCCContainerNodePoolSpec{
					Location: "europe-west1",
				},
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
			applyContainerNodePoolDefaults(&tt.nodePool, tt.poolName, tt.clusterName, tt.clusterLocation)
			if got := tt.nodePool.Metadata.Name; got != tt.wantName {
				t.Errorf("Metadata.Name = %q, want %q", got, tt.wantName)
			}
			if got := tt.nodePool.Spec.ClusterRef.Name; got != tt.wantClusterRef {
				t.Errorf("Spec.ClusterRef.Name = %q, want %q", got, tt.wantClusterRef)
			}
			if got := tt.nodePool.Spec.Location; got != tt.wantLocation {
				t.Errorf("Spec.Location = %q, want %q", got, tt.wantLocation)
			}
		})
	}
}

func TestApplySubnetworkCIDROverrides(t *testing.T) {
	tests := []struct {
		name       string
		subnet     infrav1exp.GCPKCCSubnetworkResource
		podCIDR    string
		svcCIDR    string
		wantRanges []infrav1exp.KCCSecondaryIPRange
	}{
		{
			name:    "both CIDRs",
			subnet:  infrav1exp.GCPKCCSubnetworkResource{},
			podCIDR: "10.0.0.0/14",
			svcCIDR: "10.4.0.0/20",
			wantRanges: []infrav1exp.KCCSecondaryIPRange{
				{RangeName: "pods", IpCidrRange: "10.0.0.0/14"},
				{RangeName: "services", IpCidrRange: "10.4.0.0/20"},
			},
		},
		{
			name:       "no CIDRs",
			subnet:     infrav1exp.GCPKCCSubnetworkResource{},
			podCIDR:    "",
			svcCIDR:    "",
			wantRanges: nil,
		},
		{
			name: "existing ranges updated",
			subnet: infrav1exp.GCPKCCSubnetworkResource{
				Spec: infrav1exp.GCPKCCComputeSubnetworkSpec{
					SecondaryIpRange: []infrav1exp.KCCSecondaryIPRange{
						{RangeName: "pods", IpCidrRange: "10.0.0.0/16"},
					},
				},
			},
			podCIDR: "10.0.0.0/14",
			svcCIDR: "",
			wantRanges: []infrav1exp.KCCSecondaryIPRange{
				{RangeName: "pods", IpCidrRange: "10.0.0.0/14"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applySubnetworkCIDROverrides(&tt.subnet, tt.podCIDR, tt.svcCIDR)
			got := tt.subnet.Spec.SecondaryIpRange
			if len(got) != len(tt.wantRanges) {
				t.Fatalf("SecondaryIpRange length = %d, want %d", len(got), len(tt.wantRanges))
			}
			for i, want := range tt.wantRanges {
				if got[i].RangeName != want.RangeName {
					t.Errorf("SecondaryIpRange[%d].RangeName = %q, want %q", i, got[i].RangeName, want.RangeName)
				}
				if got[i].IpCidrRange != want.IpCidrRange {
					t.Errorf("SecondaryIpRange[%d].IpCidrRange = %q, want %q", i, got[i].IpCidrRange, want.IpCidrRange)
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
		{
			name:    "version set",
			version: "1.31",
			want:    "1.31",
		},
		{
			name:    "empty version",
			version: "",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &infrav1exp.GCPKCCContainerClusterResource{}
			applyClusterVersionOverride(cluster, tt.version)
			if got := cluster.Spec.MinMasterVersion; got != tt.want {
				t.Errorf("Spec.MinMasterVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyNodePoolReplicasOverride(t *testing.T) {
	tests := []struct {
		name     string
		replicas *int32
		want     *int32
	}{
		{
			name:     "replicas set",
			replicas: ptr.To[int32](3),
			want:     ptr.To[int32](3),
		},
		{
			name:     "nil replicas",
			replicas: nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodePool := &infrav1exp.GCPKCCContainerNodePoolResource{}
			applyNodePoolReplicasOverride(nodePool, tt.replicas)
			got := nodePool.Spec.InitialNodeCount
			if tt.want == nil {
				if got != nil {
					t.Errorf("Spec.InitialNodeCount = %v, want nil", *got)
				}
			} else {
				if got == nil {
					t.Errorf("Spec.InitialNodeCount = nil, want %d", *tt.want)
				} else if *got != *tt.want {
					t.Errorf("Spec.InitialNodeCount = %d, want %d", *got, *tt.want)
				}
			}
		})
	}
}
