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
	kcck8sv1alpha1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// --- isKCCConditionTrue ---

func TestIsKCCConditionTrue(t *testing.T) {
	tests := []struct {
		name       string
		conditions []kcck8sv1alpha1.Condition
		condType   string
		want       bool
	}{
		{
			name: "Ready=True",
			conditions: []kcck8sv1alpha1.Condition{
				{Type: kcck8sv1alpha1.ReadyConditionType, Status: corev1.ConditionTrue},
			},
			condType: kcck8sv1alpha1.ReadyConditionType,
			want:     true,
		},
		{
			name: "Ready=False",
			conditions: []kcck8sv1alpha1.Condition{
				{Type: kcck8sv1alpha1.ReadyConditionType, Status: corev1.ConditionFalse},
			},
			condType: kcck8sv1alpha1.ReadyConditionType,
			want:     false,
		},
		{
			name:       "no conditions",
			conditions: nil,
			condType:   kcck8sv1alpha1.ReadyConditionType,
			want:       false,
		},
		{
			name: "Ready condition absent, other condition present",
			conditions: []kcck8sv1alpha1.Condition{
				{Type: "Reconciling", Status: corev1.ConditionTrue},
			},
			condType: kcck8sv1alpha1.ReadyConditionType,
			want:     false,
		},
		{
			name: "multiple conditions, Ready=True among them",
			conditions: []kcck8sv1alpha1.Condition{
				{Type: "Reconciling", Status: corev1.ConditionTrue},
				{Type: kcck8sv1alpha1.ReadyConditionType, Status: corev1.ConditionTrue},
			},
			condType: kcck8sv1alpha1.ReadyConditionType,
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isKCCConditionTrue(tc.conditions, tc.condType)).To(Equal(tc.want))
		})
	}
}

// --- patchSubnetworkCIDRs ---

func TestPatchSubnetworkCIDRs(t *testing.T) {
	tests := []struct {
		name           string
		existingRanges []kcccomputev1beta1.SubnetworkSecondaryIpRange
		clusterNetwork *clusterv1.ClusterNetwork
		wantRanges     []kcccomputev1beta1.SubnetworkSecondaryIpRange
	}{
		{
			name:           "no ClusterNetwork fields set — no ranges added",
			clusterNetwork: &clusterv1.ClusterNetwork{},
			wantRanges:     nil,
		},
		{
			name: "pods CIDR only",
			clusterNetwork: &clusterv1.ClusterNetwork{
				Pods: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
			},
			wantRanges: []kcccomputev1beta1.SubnetworkSecondaryIpRange{
				{RangeName: "pods", IpCidrRange: "10.0.0.0/16"},
			},
		},
		{
			name: "services CIDR only",
			clusterNetwork: &clusterv1.ClusterNetwork{
				Services: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
			},
			wantRanges: []kcccomputev1beta1.SubnetworkSecondaryIpRange{
				{RangeName: "services", IpCidrRange: "10.1.0.0/16"},
			},
		},
		{
			name: "both pods and services CIDRs",
			clusterNetwork: &clusterv1.ClusterNetwork{
				Pods:     clusterv1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
				Services: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
			},
			wantRanges: []kcccomputev1beta1.SubnetworkSecondaryIpRange{
				{RangeName: "pods", IpCidrRange: "10.0.0.0/16"},
				{RangeName: "services", IpCidrRange: "10.1.0.0/16"},
			},
		},
		{
			name: "existing pods range is updated in place",
			existingRanges: []kcccomputev1beta1.SubnetworkSecondaryIpRange{
				{RangeName: "pods", IpCidrRange: "192.168.0.0/16"},
			},
			clusterNetwork: &clusterv1.ClusterNetwork{
				Pods: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
			},
			wantRanges: []kcccomputev1beta1.SubnetworkSecondaryIpRange{
				{RangeName: "pods", IpCidrRange: "10.0.0.0/16"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			subnet := &kcccomputev1beta1.ComputeSubnetwork{}
			subnet.Spec.SecondaryIpRange = tc.existingRanges

			cluster := &clusterv1.Cluster{}
			if tc.clusterNetwork != nil {
				cluster.Spec.ClusterNetwork = *tc.clusterNetwork
			}

			patchSubnetworkCIDRs(subnet, cluster)

			if tc.wantRanges == nil {
				g.Expect(subnet.Spec.SecondaryIpRange).To(BeEmpty())
				return
			}

			g.Expect(subnet.Spec.SecondaryIpRange).To(HaveLen(len(tc.wantRanges)))
			for _, want := range tc.wantRanges {
				found := false
				for _, got := range subnet.Spec.SecondaryIpRange {
					if got.RangeName == want.RangeName {
						g.Expect(got.IpCidrRange).To(Equal(want.IpCidrRange))
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "range %q not found", want.RangeName)
			}
		})
	}
}
