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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func toInterfaceSlice(items []map[string]interface{}) []interface{} {
	result := make([]interface{}, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

func makeKCCResource(conditions []map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.Object = map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": toInterfaceSlice(conditions),
		},
	}
	return obj
}

func TestIsKCCResourceReady(t *testing.T) {
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want bool
	}{
		{
			name: "ready",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "True"},
			}),
			want: true,
		},
		{
			name: "not ready",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "False"},
			}),
			want: false,
		},
		{
			name: "no conditions",
			obj:  makeKCCResource([]map[string]interface{}{}),
			want: false,
		},
		{
			name: "no status",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			want: false,
		},
		{
			name: "wrong condition type",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Available", "status": "True"},
			}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKCCResourceReady(tt.obj)
			if got != tt.want {
				t.Errorf("isKCCResourceReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetKCCConditionMessage(t *testing.T) {
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want string
	}{
		{
			name: "has message",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "True", "message": "Ready"},
			}),
			want: "Ready",
		},
		{
			name: "no message",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "False"},
			}),
			want: "",
		},
		{
			name: "no ready condition",
			obj:  makeKCCResource([]map[string]interface{}{}),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getKCCConditionMessage(tt.obj)
			if got != tt.want {
				t.Errorf("getKCCConditionMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetKCCStatusField(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		fields    []string
		wantVal   string
		wantFound bool
	}{
		{
			name: "simple field",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"endpoint": "1.2.3.4",
					},
				},
			},
			fields:    []string{"endpoint"},
			wantVal:   "1.2.3.4",
			wantFound: true,
		},
		{
			name: "nested field",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"masterAuth": map[string]interface{}{
							"clusterCaCertificate": "abc",
						},
					},
				},
			},
			fields:    []string{"masterAuth", "clusterCaCertificate"},
			wantVal:   "abc",
			wantFound: true,
		},
		{
			name: "missing field",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"status": map[string]interface{}{},
				},
			},
			fields:    []string{"nonexistent"},
			wantVal:   "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotFound := getKCCStatusField(tt.obj, tt.fields...)
			if gotVal != tt.wantVal {
				t.Errorf("getKCCStatusField() value = %q, want %q", gotVal, tt.wantVal)
			}
			if gotFound != tt.wantFound {
				t.Errorf("getKCCStatusField() found = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}
