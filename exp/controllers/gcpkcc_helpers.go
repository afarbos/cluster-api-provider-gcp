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
	"sort"
	"strings"

	"golang.org/x/mod/semver"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// getKCCReadiness checks whether a KCC unstructured resource has a Ready
// condition with status "True" and returns (ready, message).
func getKCCReadiness(obj *unstructured.Unstructured) (bool, string) {
	if obj == nil {
		return false, ""
	}
	status, ok := obj.Object["status"].(map[string]interface{})
	if !ok {
		return false, ""
	}
	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return false, ""
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if condType, _ := cond["type"].(string); condType == "Ready" {
			msg, _ := cond["message"].(string)
			if condStatus, _ := cond["status"].(string); condStatus == "True" {
				return true, msg
			}
			return false, msg
		}
	}
	return false, ""
}

// getStatusFieldFromUnstructured extracts a nested string field from the status of a KCC
// unstructured resource. The fields parameter specifies the path relative to
// status (e.g., "observedState", "endpoint").
func getStatusFieldFromUnstructured(obj *unstructured.Unstructured, fields ...string) (string, bool) {
	val, found, err := unstructured.NestedString(obj.Object, append([]string{"status"}, fields...)...)
	if err != nil || !found {
		return "", false
	}
	return val, true
}

// applyKCCResource applies the desired KCC resource using server-side apply.
// This only sends the fields we manage — KCC-managed fields (like spec.resourceID)
// are left untouched. Creates the resource if it doesn't exist, updates our
// managed fields if it does.
func applyKCCResource(ctx context.Context, c client.Client, desired *unstructured.Unstructured) error {
	return c.Patch(ctx, desired, client.Apply, client.FieldOwner("capg-kcc-controller"), client.ForceOwnership)
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

// deleteResource deletes a KCC resource by GVK/name/namespace. Returns true if
// the resource no longer exists (deleted or already gone), false if deletion was
// issued and the caller should requeue.
func deleteResource(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, name, namespace string) (bool, error) {
	if name == "" {
		return true, nil
	}
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	if err := c.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("deleting resource %s/%s: %w", namespace, name, err)
	}
	return false, nil
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

// nodePoolInfo contains the provider ID list and replica count for a node pool.
type nodePoolInfo struct {
	ProviderIDList []string
	Replicas       int32
}

// getNodePoolInfoFromWorkloadCluster connects to the workload cluster and lists
// nodes belonging to the given node pool. Returns provider IDs and ready count.
func getNodePoolInfoFromWorkloadCluster(ctx context.Context, mgmtClient client.Client, clusterName, namespace, nodePoolName string) (*nodePoolInfo, error) {
	// Get the kubeconfig secret.
	secretName := fmt.Sprintf("%s-kubeconfig", clusterName)
	secret := &corev1.Secret{}
	if err := mgmtClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		return nil, fmt.Errorf("getting kubeconfig secret %s: %w", secretName, err)
	}

	kubeconfigData, ok := secret.Data["value"]
	if !ok || len(kubeconfigData) == 0 {
		return nil, fmt.Errorf("kubeconfig secret %s has no 'value' key", secretName)
	}

	// Build REST config from kubeconfig.
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}

	// Create a Kubernetes clientset for the workload cluster.
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating workload cluster client: %w", err)
	}

	// List nodes belonging to this node pool.
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("cloud.google.com/gke-nodepool=%s", nodePoolName),
	})
	if err != nil {
		return nil, fmt.Errorf("listing workload cluster nodes: %w", err)
	}

	info := &nodePoolInfo{}
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		if node.Spec.ProviderID != "" {
			info.ProviderIDList = append(info.ProviderIDList, node.Spec.ProviderID)
		}
		info.Replicas++
	}

	// Sort for deterministic output.
	sort.Strings(info.ProviderIDList)
	return info, nil
}

// isVersionUpToDate returns true if the observed version's major.minor is >= the
// desired version's major.minor. GKE auto-upgrades can result in observed > desired.
// Returns true if desired is empty (no version constraint).
// Uses golang.org/x/mod/semver (already a CAPG dependency) for comparison.
func isVersionUpToDate(desired, observed string) bool {
	if desired == "" {
		return true
	}
	// semver requires "v" prefix; MajorMinor strips patch/pre-release (e.g., "-gke.1026000").
	// Handle versions that may already have a "v" prefix (e.g., from topology).
	d := semver.MajorMinor(ensureVPrefix(desired))
	o := semver.MajorMinor(ensureVPrefix(observed))
	if d == "" || o == "" {
		return false
	}
	return semver.Compare(o, d) >= 0
}

// boolToReplicaCount returns ptr.To(int32(1)) if true, ptr.To(int32(0)) if false.
func boolToReplicaCount(b bool) *int32 {
	if b {
		return ptr.To(int32(1))
	}
	return ptr.To(int32(0))
}

// toGKEVersion converts a CAPI semver version to a GKE-compatible version string.
// Generic versions (e.g., "v1.32.0") are stripped to major.minor ("1.32") so GKE
// picks the latest patch. Specific GKE versions with a prerelease tag
// (e.g., "v1.32.0-gke.1026000") are kept intact with only the "v" prefix removed.
func toGKEVersion(version string) string {
	v := ensureVPrefix(version)
	if semver.Prerelease(v) != "" {
		return strings.TrimPrefix(v, "v")
	}
	return strings.TrimPrefix(semver.MajorMinor(v), "v")
}

func ensureVPrefix(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

