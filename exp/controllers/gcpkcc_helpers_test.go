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

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// --- rawExtensionToUnstructured ---

func TestRawExtensionToUnstructured(t *testing.T) {
	tests := []struct {
		name    string
		raw     k8sruntime.RawExtension
		want    string // expected name
		wantErr bool
	}{
		{
			name: "valid resource",
			raw: mustMarshalRaw(map[string]interface{}{
				"apiVersion": "compute.cnrm.cloud.google.com/v1beta1",
				"kind":       "ComputeNetwork",
				"metadata":   map[string]interface{}{"name": "my-network"},
			}),
			want: "my-network",
		},
		{
			name:    "empty raw bytes",
			raw:     k8sruntime.RawExtension{},
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			raw:     k8sruntime.RawExtension{Raw: []byte(`{not valid json}`)},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			obj, err := rawExtensionToUnstructured(tc.raw)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(obj.GetName()).To(Equal(tc.want))
		})
	}
}

// --- getResourceName ---

func TestGetResourceName(t *testing.T) {
	tests := []struct {
		name    string
		raw     k8sruntime.RawExtension
		want    string
		wantErr bool
	}{
		{
			name: "name present",
			raw: mustMarshalRaw(map[string]interface{}{
				"metadata": map[string]interface{}{"name": "my-net"},
			}),
			want: "my-net",
		},
		{
			name:    "empty raw",
			raw:     k8sruntime.RawExtension{},
			wantErr: true,
		},
		{
			name: "missing name",
			raw: mustMarshalRaw(map[string]interface{}{
				"metadata": map[string]interface{}{},
			}),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			name, err := getResourceName(tc.raw)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(name).To(Equal(tc.want))
		})
	}
}

// --- isKCCResourceReady ---

func TestIsKCCResourceReady(t *testing.T) {
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want bool
	}{
		{
			name: "Ready=True",
			obj:  unstructuredWithConditions([]interface{}{kccCondition("Ready", "True")}),
			want: true,
		},
		{
			name: "Ready=False",
			obj:  unstructuredWithConditions([]interface{}{kccCondition("Ready", "False")}),
			want: false,
		},
		{
			name: "no conditions",
			obj:  &unstructured.Unstructured{Object: map[string]interface{}{}},
			want: false,
		},
		{
			name: "Ready condition absent, other condition present",
			obj:  unstructuredWithConditions([]interface{}{kccCondition("Reconciling", "True")}),
			want: false,
		},
		{
			name: "multiple conditions, Ready=True among them",
			obj: unstructuredWithConditions([]interface{}{
				kccCondition("Reconciling", "True"),
				kccCondition("Ready", "True"),
			}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isKCCResourceReady(tc.obj)).To(Equal(tc.want))
		})
	}
}

// --- patchSubnetworkCIDRs ---

func TestPatchSubnetworkCIDRs(t *testing.T) {
	tests := []struct {
		name           string
		existingRanges []interface{}
		clusterNetwork *clusterv1.ClusterNetwork
		wantRanges     []map[string]string // rangeName → ipCidrRange
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
			wantRanges: []map[string]string{{"pods": "10.0.0.0/16"}},
		},
		{
			name: "services CIDR only",
			clusterNetwork: &clusterv1.ClusterNetwork{
				Services: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
			},
			wantRanges: []map[string]string{{"services": "10.1.0.0/16"}},
		},
		{
			name: "both pods and services CIDRs",
			clusterNetwork: &clusterv1.ClusterNetwork{
				Pods:     clusterv1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
				Services: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.1.0.0/16"}},
			},
			wantRanges: []map[string]string{
				{"pods": "10.0.0.0/16"},
				{"services": "10.1.0.0/16"},
			},
		},
		{
			name: "existing pods range is updated in place",
			existingRanges: []interface{}{
				map[string]interface{}{"rangeName": "pods", "ipCidrRange": "192.168.0.0/16"},
			},
			clusterNetwork: &clusterv1.ClusterNetwork{
				Pods: clusterv1.NetworkRanges{CIDRBlocks: []string{"10.0.0.0/16"}},
			},
			wantRanges: []map[string]string{{"pods": "10.0.0.0/16"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			subnet := &unstructured.Unstructured{Object: map[string]interface{}{}}
			if len(tc.existingRanges) > 0 {
				g.Expect(unstructured.SetNestedSlice(subnet.Object, tc.existingRanges, "spec", "secondaryIpRange")).To(Succeed())
			}

			cluster := &clusterv1.Cluster{}
			if tc.clusterNetwork != nil {
				cluster.Spec.ClusterNetwork = *tc.clusterNetwork
			}

			err := patchSubnetworkCIDRs(subnet, cluster)
			g.Expect(err).NotTo(HaveOccurred())

			if tc.wantRanges == nil {
				ranges, found, _ := unstructured.NestedSlice(subnet.Object, "spec", "secondaryIpRange")
				g.Expect(!found || len(ranges) == 0).To(BeTrue())
				return
			}

			ranges, found, err := unstructured.NestedSlice(subnet.Object, "spec", "secondaryIpRange")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(found).To(BeTrue())
			g.Expect(ranges).To(HaveLen(len(tc.wantRanges)))

			for _, want := range tc.wantRanges {
				for wantName, wantCIDR := range want {
					found := false
					for _, r := range ranges {
						rm := r.(map[string]interface{})
						if rm["rangeName"] == wantName {
							g.Expect(rm["ipCidrRange"]).To(Equal(wantCIDR))
							found = true
							break
						}
					}
					g.Expect(found).To(BeTrue(), "range %q not found", wantName)
				}
			}
		})
	}
}

// --- helpers ---

func mustMarshalRaw(v interface{}) k8sruntime.RawExtension {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return k8sruntime.RawExtension{Raw: b}
}

func kccCondition(condType, status string) map[string]interface{} {
	return map[string]interface{}{
		"type":   condType,
		"status": status,
	}
}

func unstructuredWithConditions(conditions []interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	_ = unstructured.SetNestedSlice(obj.Object, conditions, "status", "conditions")
	return obj
}
