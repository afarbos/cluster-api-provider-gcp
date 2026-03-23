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

	kcccomputev1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/compute/v1beta1"
	kcccontainerv1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/container/v1beta1"
	kcck8sv1alpha1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// --- applyNetworkDefaults ---

func TestApplyNetworkDefaults(t *testing.T) {
	tests := []struct {
		name        string
		network     *kcccomputev1beta1.ComputeNetwork
		clusterName string
		wantName    string
		wantAutoSub *bool
		wantRouting *string
	}{
		{
			name:        "all empty — all defaults applied",
			network:     &kcccomputev1beta1.ComputeNetwork{},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantAutoSub: ptr.To(false),
			wantRouting: ptr.To("REGIONAL"),
		},
		{
			name: "name already set — not overridden",
			network: &kcccomputev1beta1.ComputeNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-net"},
			},
			clusterName: "my-cluster",
			wantName:    "custom-net",
			wantAutoSub: ptr.To(false),
			wantRouting: ptr.To("REGIONAL"),
		},
		{
			name: "autoCreateSubnetworks already set — not overridden",
			network: &kcccomputev1beta1.ComputeNetwork{
				Spec: kcccomputev1beta1.ComputeNetworkSpec{
					AutoCreateSubnetworks: ptr.To(true),
				},
			},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantAutoSub: ptr.To(true),
			wantRouting: ptr.To("REGIONAL"),
		},
		{
			name: "routingMode already set — not overridden",
			network: &kcccomputev1beta1.ComputeNetwork{
				Spec: kcccomputev1beta1.ComputeNetworkSpec{
					RoutingMode: ptr.To("GLOBAL"),
				},
			},
			clusterName: "my-cluster",
			wantName:    "my-cluster",
			wantAutoSub: ptr.To(false),
			wantRouting: ptr.To("GLOBAL"),
		},
		{
			name: "all fields already set — none overridden",
			network: &kcccomputev1beta1.ComputeNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-net"},
				Spec: kcccomputev1beta1.ComputeNetworkSpec{
					AutoCreateSubnetworks: ptr.To(true),
					RoutingMode:           ptr.To("GLOBAL"),
				},
			},
			clusterName: "my-cluster",
			wantName:    "custom-net",
			wantAutoSub: ptr.To(true),
			wantRouting: ptr.To("GLOBAL"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			applyNetworkDefaults(tc.network, tc.clusterName)
			g.Expect(tc.network.Name).To(Equal(tc.wantName))
			g.Expect(tc.network.Spec.AutoCreateSubnetworks).To(Equal(tc.wantAutoSub))
			g.Expect(tc.network.Spec.RoutingMode).To(Equal(tc.wantRouting))
		})
	}
}

// --- applySubnetworkDefaults ---

func TestApplySubnetworkDefaults(t *testing.T) {
	tests := []struct {
		name        string
		subnet      *kcccomputev1beta1.ComputeSubnetwork
		clusterName string
		networkName string
		wantName    string
		wantNetRef  kcck8sv1alpha1.ResourceRef
	}{
		{
			name:        "all empty — all defaults applied",
			subnet:      &kcccomputev1beta1.ComputeSubnetwork{},
			clusterName: "my-cluster",
			networkName: "my-network",
			wantName:    "my-cluster",
			wantNetRef:  kcck8sv1alpha1.ResourceRef{Name: "my-network"},
		},
		{
			name: "name already set — not overridden",
			subnet: &kcccomputev1beta1.ComputeSubnetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-subnet"},
			},
			clusterName: "my-cluster",
			networkName: "my-network",
			wantName:    "custom-subnet",
			wantNetRef:  kcck8sv1alpha1.ResourceRef{Name: "my-network"},
		},
		{
			name: "networkRef.Name already set — not overridden",
			subnet: &kcccomputev1beta1.ComputeSubnetwork{
				Spec: kcccomputev1beta1.ComputeSubnetworkSpec{
					NetworkRef: kcck8sv1alpha1.ResourceRef{Name: "existing-network"},
				},
			},
			clusterName: "my-cluster",
			networkName: "my-network",
			wantName:    "my-cluster",
			wantNetRef:  kcck8sv1alpha1.ResourceRef{Name: "existing-network"},
		},
		{
			name: "networkRef.External already set — not overridden",
			subnet: &kcccomputev1beta1.ComputeSubnetwork{
				Spec: kcccomputev1beta1.ComputeSubnetworkSpec{
					NetworkRef: kcck8sv1alpha1.ResourceRef{External: "projects/p/global/networks/ext-net"},
				},
			},
			clusterName: "my-cluster",
			networkName: "my-network",
			wantName:    "my-cluster",
			wantNetRef:  kcck8sv1alpha1.ResourceRef{External: "projects/p/global/networks/ext-net"},
		},
		{
			name: "all fields already set — none overridden",
			subnet: &kcccomputev1beta1.ComputeSubnetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-subnet"},
				Spec: kcccomputev1beta1.ComputeSubnetworkSpec{
					NetworkRef: kcck8sv1alpha1.ResourceRef{Name: "existing-network"},
				},
			},
			clusterName: "my-cluster",
			networkName: "my-network",
			wantName:    "custom-subnet",
			wantNetRef:  kcck8sv1alpha1.ResourceRef{Name: "existing-network"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			applySubnetworkDefaults(tc.subnet, tc.clusterName, tc.networkName)
			g.Expect(tc.subnet.Name).To(Equal(tc.wantName))
			g.Expect(tc.subnet.Spec.NetworkRef).To(Equal(tc.wantNetRef))
		})
	}
}

// --- applyContainerClusterDefaults ---

func TestApplyContainerClusterDefaults(t *testing.T) {
	tests := []struct {
		name               string
		cluster            *kcccontainerv1beta1.ContainerCluster
		capiClusterName    string
		networkName        string
		subnetworkName     string
		subnetworkRegion   string
		hasSecondaryRanges bool
		wantName           string
		wantNetRef         *kcck8sv1alpha1.ResourceRef
		wantSubRef         *kcck8sv1alpha1.ResourceRef
		wantLocation       string
		wantInitialCount   *int64
		wantNetworkingMode *string
		wantIPPolicy       *kcccontainerv1beta1.ClusterIpAllocationPolicy
		wantAnnotation     string
	}{
		{
			name:               "all empty with secondary ranges — all defaults applied",
			cluster:            &kcccontainerv1beta1.ContainerCluster{},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: true,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy: &kcccontainerv1beta1.ClusterIpAllocationPolicy{
				ClusterSecondaryRangeName:  ptr.To("pods"),
				ServicesSecondaryRangeName: ptr.To("services"),
			},
			wantAnnotation: "true",
		},
		{
			name:               "all empty without secondary ranges — no IP allocation policy",
			cluster:            &kcccontainerv1beta1.ContainerCluster{},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "name already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-cluster"},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "custom-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "networkRef already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					NetworkRef: &kcck8sv1alpha1.ResourceRef{Name: "existing-net"},
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "existing-net"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "subnetworkRef already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					SubnetworkRef: &kcck8sv1alpha1.ResourceRef{Name: "existing-subnet"},
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "existing-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "location already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					Location: "europe-west1",
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "europe-west1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "initialNodeCount already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					InitialNodeCount: ptr.To(int64(3)),
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(3)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "networkingMode already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					NetworkingMode: ptr.To("ROUTES"),
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("ROUTES"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
		{
			name: "ipAllocationPolicy already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					IpAllocationPolicy: &kcccontainerv1beta1.ClusterIpAllocationPolicy{
						ClusterSecondaryRangeName:  ptr.To("custom-pods"),
						ServicesSecondaryRangeName: ptr.To("custom-services"),
					},
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: true,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy: &kcccontainerv1beta1.ClusterIpAllocationPolicy{
				ClusterSecondaryRangeName:  ptr.To("custom-pods"),
				ServicesSecondaryRangeName: ptr.To("custom-services"),
			},
			wantAnnotation: "true",
		},
		{
			name: "remove-default-node-pool annotation already set — not overridden",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cnrm.cloud.google.com/remove-default-node-pool": "false",
					},
				},
			},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "us-central1",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "us-central1",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "false",
		},
		{
			name: "empty subnetworkRegion — location not set",
			cluster: &kcccontainerv1beta1.ContainerCluster{},
			capiClusterName:    "my-cluster",
			networkName:        "my-network",
			subnetworkName:     "my-subnet",
			subnetworkRegion:   "",
			hasSecondaryRanges: false,
			wantName:           "my-cluster",
			wantNetRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-network"},
			wantSubRef:         &kcck8sv1alpha1.ResourceRef{Name: "my-subnet"},
			wantLocation:       "",
			wantInitialCount:   ptr.To(int64(1)),
			wantNetworkingMode: ptr.To("VPC_NATIVE"),
			wantIPPolicy:       nil,
			wantAnnotation:     "true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			applyContainerClusterDefaults(tc.cluster, tc.capiClusterName, tc.networkName, tc.subnetworkName, tc.subnetworkRegion, tc.hasSecondaryRanges)
			g.Expect(tc.cluster.Name).To(Equal(tc.wantName))
			g.Expect(tc.cluster.Spec.NetworkRef).To(Equal(tc.wantNetRef))
			g.Expect(tc.cluster.Spec.SubnetworkRef).To(Equal(tc.wantSubRef))
			g.Expect(tc.cluster.Spec.Location).To(Equal(tc.wantLocation))
			g.Expect(tc.cluster.Spec.InitialNodeCount).To(Equal(tc.wantInitialCount))
			g.Expect(tc.cluster.Spec.NetworkingMode).To(Equal(tc.wantNetworkingMode))
			g.Expect(tc.cluster.Spec.IpAllocationPolicy).To(Equal(tc.wantIPPolicy))
			g.Expect(tc.cluster.Annotations["cnrm.cloud.google.com/remove-default-node-pool"]).To(Equal(tc.wantAnnotation))
		})
	}
}

// --- applyContainerClusterOverrides ---

func TestApplyContainerClusterOverrides(t *testing.T) {
	tests := []struct {
		name           string
		cluster        *kcccontainerv1beta1.ContainerCluster
		version        *string
		wantMinVersion *string
	}{
		{
			name:           "version provided — always applied",
			cluster:        &kcccontainerv1beta1.ContainerCluster{},
			version:        ptr.To("1.29"),
			wantMinVersion: ptr.To("1.29"),
		},
		{
			name: "version provided — overrides existing",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					MinMasterVersion: ptr.To("1.27"),
				},
			},
			version:        ptr.To("1.29"),
			wantMinVersion: ptr.To("1.29"),
		},
		{
			name:           "nil version — original preserved",
			cluster:        &kcccontainerv1beta1.ContainerCluster{},
			version:        nil,
			wantMinVersion: nil,
		},
		{
			name: "nil version — existing preserved",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					MinMasterVersion: ptr.To("1.27"),
				},
			},
			version:        nil,
			wantMinVersion: ptr.To("1.27"),
		},
		{
			name: "empty version — existing preserved",
			cluster: &kcccontainerv1beta1.ContainerCluster{
				Spec: kcccontainerv1beta1.ContainerClusterSpec{
					MinMasterVersion: ptr.To("1.27"),
				},
			},
			version:        ptr.To(""),
			wantMinVersion: ptr.To("1.27"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			applyContainerClusterOverrides(tc.cluster, tc.version)
			g.Expect(tc.cluster.Spec.MinMasterVersion).To(Equal(tc.wantMinVersion))
		})
	}
}

// --- applyContainerNodePoolDefaults ---

func TestApplyContainerNodePoolDefaults(t *testing.T) {
	tests := []struct {
		name            string
		nodePool        *kcccontainerv1beta1.ContainerNodePool
		machinePoolName string
		capiClusterName string
		clusterLocation string
		wantName        string
		wantClusterRef  kcck8sv1alpha1.ResourceRef
		wantLocation    string
	}{
		{
			name:            "all empty — all defaults applied",
			nodePool:        &kcccontainerv1beta1.ContainerNodePool{},
			machinePoolName: "my-pool",
			capiClusterName: "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "my-pool",
			wantClusterRef:  kcck8sv1alpha1.ResourceRef{Name: "my-cluster"},
			wantLocation:    "us-central1",
		},
		{
			name: "name already set — not overridden",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-pool"},
			},
			machinePoolName: "my-pool",
			capiClusterName: "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "custom-pool",
			wantClusterRef:  kcck8sv1alpha1.ResourceRef{Name: "my-cluster"},
			wantLocation:    "us-central1",
		},
		{
			name: "clusterRef.Name already set — not overridden",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					ClusterRef: kcck8sv1alpha1.ResourceRef{Name: "existing-cluster"},
				},
			},
			machinePoolName: "my-pool",
			capiClusterName: "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "my-pool",
			wantClusterRef:  kcck8sv1alpha1.ResourceRef{Name: "existing-cluster"},
			wantLocation:    "us-central1",
		},
		{
			name: "clusterRef.External already set — not overridden",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					ClusterRef: kcck8sv1alpha1.ResourceRef{External: "projects/p/locations/l/clusters/c"},
				},
			},
			machinePoolName: "my-pool",
			capiClusterName: "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "my-pool",
			wantClusterRef:  kcck8sv1alpha1.ResourceRef{External: "projects/p/locations/l/clusters/c"},
			wantLocation:    "us-central1",
		},
		{
			name: "location already set — not overridden",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					Location: "europe-west1",
				},
			},
			machinePoolName: "my-pool",
			capiClusterName: "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "my-pool",
			wantClusterRef:  kcck8sv1alpha1.ResourceRef{Name: "my-cluster"},
			wantLocation:    "europe-west1",
		},
		{
			name: "all fields already set — none overridden",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-pool"},
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					ClusterRef: kcck8sv1alpha1.ResourceRef{Name: "existing-cluster"},
					Location:   "europe-west1",
				},
			},
			machinePoolName: "my-pool",
			capiClusterName: "my-cluster",
			clusterLocation: "us-central1",
			wantName:        "custom-pool",
			wantClusterRef:  kcck8sv1alpha1.ResourceRef{Name: "existing-cluster"},
			wantLocation:    "europe-west1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			applyContainerNodePoolDefaults(tc.nodePool, tc.machinePoolName, tc.capiClusterName, tc.clusterLocation)
			g.Expect(tc.nodePool.Name).To(Equal(tc.wantName))
			g.Expect(tc.nodePool.Spec.ClusterRef).To(Equal(tc.wantClusterRef))
			g.Expect(tc.nodePool.Spec.Location).To(Equal(tc.wantLocation))
		})
	}
}

// --- applyContainerNodePoolOverrides ---

func TestApplyContainerNodePoolOverrides(t *testing.T) {
	tests := []struct {
		name               string
		nodePool           *kcccontainerv1beta1.ContainerNodePool
		replicas           *int32
		version            *string
		failureDomains     []string
		wantInitialCount   *int64
		wantVersion        *string
		wantNodeLocations  []string
	}{
		{
			name:              "all overrides provided — always applied",
			nodePool:          &kcccontainerv1beta1.ContainerNodePool{},
			replicas:          ptr.To(int32(3)),
			version:           ptr.To("1.29"),
			failureDomains:    []string{"us-central1-a", "us-central1-b"},
			wantInitialCount:  ptr.To(int64(3)),
			wantVersion:       ptr.To("1.29"),
			wantNodeLocations: []string{"us-central1-a", "us-central1-b"},
		},
		{
			name: "all overrides provided — overrides existing values",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					InitialNodeCount: ptr.To(int64(1)),
					Version:          ptr.To("1.27"),
					NodeLocations:    []string{"us-east1-a"},
				},
			},
			replicas:          ptr.To(int32(5)),
			version:           ptr.To("1.29"),
			failureDomains:    []string{"us-central1-a", "us-central1-b"},
			wantInitialCount:  ptr.To(int64(5)),
			wantVersion:       ptr.To("1.29"),
			wantNodeLocations: []string{"us-central1-a", "us-central1-b"},
		},
		{
			name: "nil/empty overrides — original values preserved",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					InitialNodeCount: ptr.To(int64(2)),
					Version:          ptr.To("1.28"),
					NodeLocations:    []string{"us-east1-a"},
				},
			},
			replicas:          nil,
			version:           nil,
			failureDomains:    nil,
			wantInitialCount:  ptr.To(int64(2)),
			wantVersion:       ptr.To("1.28"),
			wantNodeLocations: []string{"us-east1-a"},
		},
		{
			name: "empty version string — original preserved",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					Version: ptr.To("1.28"),
				},
			},
			replicas:          nil,
			version:           ptr.To(""),
			failureDomains:    nil,
			wantInitialCount:  nil,
			wantVersion:       ptr.To("1.28"),
			wantNodeLocations: nil,
		},
		{
			name: "empty failureDomains slice — original preserved",
			nodePool: &kcccontainerv1beta1.ContainerNodePool{
				Spec: kcccontainerv1beta1.ContainerNodePoolSpec{
					NodeLocations: []string{"us-east1-a"},
				},
			},
			replicas:          nil,
			version:           nil,
			failureDomains:    []string{},
			wantInitialCount:  nil,
			wantVersion:       nil,
			wantNodeLocations: []string{"us-east1-a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			applyContainerNodePoolOverrides(tc.nodePool, tc.replicas, tc.version, tc.failureDomains)
			g.Expect(tc.nodePool.Spec.InitialNodeCount).To(Equal(tc.wantInitialCount))
			g.Expect(tc.nodePool.Spec.Version).To(Equal(tc.wantVersion))
			if tc.wantNodeLocations == nil {
				g.Expect(tc.nodePool.Spec.NodeLocations).To(Equal(tc.wantNodeLocations))
			} else {
				g.Expect(tc.nodePool.Spec.NodeLocations).To(Equal(tc.wantNodeLocations))
			}
		})
	}
}
