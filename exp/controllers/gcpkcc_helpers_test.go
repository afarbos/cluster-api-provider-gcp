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

func TestGetKCCReadiness(t *testing.T) {
	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		ready   bool
		message string
	}{
		{
			name: "ready with message",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "True", "message": "All good"},
			}),
			ready:   true,
			message: "All good",
		},
		{
			name: "ready without message",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "True"},
			}),
			ready:   true,
			message: "",
		},
		{
			name: "not ready with message",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "False", "message": "Provisioning"},
			}),
			ready:   false,
			message: "Provisioning",
		},
		{
			name: "not ready without message",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Ready", "status": "False"},
			}),
			ready:   false,
			message: "",
		},
		{
			name:    "no conditions",
			obj:     makeKCCResource([]map[string]interface{}{}),
			ready:   false,
			message: "",
		},
		{
			name: "no status",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			ready:   false,
			message: "",
		},
		{
			name:    "nil object",
			obj:     nil,
			ready:   false,
			message: "",
		},
		{
			name: "wrong condition type",
			obj: makeKCCResource([]map[string]interface{}{
				{"type": "Available", "status": "True"},
			}),
			ready:   false,
			message: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReady, gotMsg := getKCCReadiness(tt.obj)
			if gotReady != tt.ready {
				t.Errorf("getKCCReadiness() ready = %v, want %v", gotReady, tt.ready)
			}
			if gotMsg != tt.message {
				t.Errorf("getKCCReadiness() message = %q, want %q", gotMsg, tt.message)
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
			gotVal, gotFound := getStatusFieldFromUnstructured(tt.obj, tt.fields...)
			if gotVal != tt.wantVal {
				t.Errorf("getStatusFieldFromUnstructured() value = %q, want %q", gotVal, tt.wantVal)
			}
			if gotFound != tt.wantFound {
				t.Errorf("getStatusFieldFromUnstructured() found = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}
