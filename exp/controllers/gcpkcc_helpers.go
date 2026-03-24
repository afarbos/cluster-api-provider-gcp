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
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// kccReconciliationTimeout is the maximum time to wait for a KCC resource to become ready.
	kccReconciliationTimeout = 30 * time.Minute
)

var (
	// computeNetworkGVK is the GroupVersionKind for KCC ComputeNetwork resources.
	computeNetworkGVK = schema.GroupVersionKind{
		Group:   "compute.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ComputeNetwork",
	}

	// computeSubnetworkGVK is the GroupVersionKind for KCC ComputeSubnetwork resources.
	computeSubnetworkGVK = schema.GroupVersionKind{
		Group:   "compute.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ComputeSubnetwork",
	}

	// containerClusterGVK is the GroupVersionKind for KCC ContainerCluster resources.
	containerClusterGVK = schema.GroupVersionKind{
		Group:   "container.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ContainerCluster",
	}

	// containerNodePoolGVK is the GroupVersionKind for KCC ContainerNodePool resources.
	containerNodePoolGVK = schema.GroupVersionKind{
		Group:   "container.cnrm.cloud.google.com",
		Version: "v1beta1",
		Kind:    "ContainerNodePool",
	}
)

// isKCCResourceReady checks whether a KCC unstructured resource has a Ready
// condition with status "True".
func isKCCResourceReady(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}

	status, ok := obj.Object["status"].(map[string]interface{})
	if !ok {
		return false
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return false
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if condType, _ := cond["type"].(string); condType == "Ready" {
			if condStatus, _ := cond["status"].(string); condStatus == "True" {
				return true
			}
		}
	}

	return false
}

// getKCCConditionMessage returns the message from the Ready condition of a KCC
// unstructured resource. Returns an empty string if the condition is not found.
func getKCCConditionMessage(obj *unstructured.Unstructured) string {
	if obj == nil {
		return ""
	}

	status, ok := obj.Object["status"].(map[string]interface{})
	if !ok {
		return ""
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return ""
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if condType, _ := cond["type"].(string); condType == "Ready" {
			msg, _ := cond["message"].(string)
			return msg
		}
	}

	return ""
}

// getKCCStatusField extracts a nested string field from the status of a KCC
// unstructured resource. The fields parameter specifies the path relative to
// status (e.g., "observedState", "endpoint").
func getKCCStatusField(obj *unstructured.Unstructured, fields ...string) (string, bool) {
	val, found, err := unstructured.NestedString(obj.Object, append([]string{"status"}, fields...)...)
	if err != nil || !found {
		return "", false
	}
	return val, true
}

// createOrPatchKCCResource creates the KCC resource if it does not exist, or
// patches it using a merge patch if it already exists. Existing labels and
// annotations are preserved (merged, not replaced).
func createOrPatchKCCResource(ctx context.Context, c client.Client, desired *unstructured.Unstructured) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())

	err := c.Get(ctx, types.NamespacedName{
		Name:      desired.GetName(),
		Namespace: desired.GetNamespace(),
	}, existing)

	if apierrors.IsNotFound(err) {
		return c.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("failed to get existing KCC resource %s/%s: %w",
			desired.GetNamespace(), desired.GetName(), err)
	}

	// Merge labels: existing labels take lower precedence than desired labels,
	// but we preserve any existing labels not set by desired.
	mergedLabels := mergeStringMaps(existing.GetLabels(), desired.GetLabels())
	desired.SetLabels(mergedLabels)

	// Merge annotations: same strategy as labels.
	mergedAnnotations := mergeStringMaps(existing.GetAnnotations(), desired.GetAnnotations())
	desired.SetAnnotations(mergedAnnotations)

	// Copy ResourceVersion from existing to desired so the patch can succeed.
	desired.SetResourceVersion(existing.GetResourceVersion())

	patch := client.MergeFrom(existing)
	return c.Patch(ctx, desired, patch)
}

// mergeStringMaps merges two string maps. Values from overlay take precedence
// over values from base for the same key.
func mergeStringMaps(base, overlay map[string]string) map[string]string {
	if base == nil && overlay == nil {
		return nil
	}
	merged := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}
	return merged
}

// setKCCOwnerReference sets a controller owner reference on the owned
// unstructured object, pointing back to the owner. The owner reference has
// BlockOwnerDeletion and Controller set to true.
func setKCCOwnerReference(owner client.Object, ownerGVK schema.GroupVersionKind, owned *unstructured.Unstructured) error {
	blockOwnerDeletion := true
	isController := true

	ownerRef := metav1.OwnerReference{
		APIVersion:         ownerGVK.GroupVersion().String(),
		Kind:               ownerGVK.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}

	existingRefs := owned.GetOwnerReferences()

	// Check if a controller reference from this owner already exists and update it.
	for i, ref := range existingRefs {
		if ref.UID == owner.GetUID() {
			existingRefs[i] = ownerRef
			owned.SetOwnerReferences(existingRefs)
			return nil
		}
	}

	// Check that no other controller reference is already set.
	for _, ref := range existingRefs {
		if ref.Controller != nil && *ref.Controller {
			return &controllerutil.AlreadyOwnedError{
				Object: owned,
				Owner:  ref,
			}
		}
	}

	owned.SetOwnerReferences(append(existingRefs, ownerRef))
	return nil
}

// checkKCCCRDsPresent verifies that the CRDs for the given GVKs are installed
// in the cluster by attempting a list with limit=1 for each GVK.
func checkKCCCRDsPresent(ctx context.Context, c client.Client, gvks ...schema.GroupVersionKind) error {
	for _, gvk := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		if err := c.List(ctx, list, client.Limit(1)); err != nil {
			return fmt.Errorf("KCC CRD %s not found in cluster: %w. "+
				"Ensure Config Connector is installed: "+
				"https://cloud.google.com/config-connector/docs/how-to/install-upgrade-uninstall",
				gvk.Kind, err)
		}
	}
	return nil
}
