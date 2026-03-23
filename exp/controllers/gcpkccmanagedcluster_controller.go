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
	"encoding/json"
	"fmt"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api-provider-gcp/feature"
	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/util/reconciler"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	computeNetworkGVK    = schema.GroupVersionKind{Group: "compute.cnrm.cloud.google.com", Version: "v1beta1", Kind: "ComputeNetwork"}
	computeSubnetworkGVK = schema.GroupVersionKind{Group: "compute.cnrm.cloud.google.com", Version: "v1beta1", Kind: "ComputeSubnetwork"}
)

// GCPKCCManagedClusterReconciler reconciles a GCPKCCManagedCluster object.
type GCPKCCManagedClusterReconciler struct {
	client.Client
	WatchFilterValue string
	ReconcileTimeout time.Duration
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=compute.cnrm.cloud.google.com,resources=computenetworks;computesubnetworks,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles a GCPKCCManagedCluster by managing Config Connector
// ComputeNetwork and ComputeSubnetwork resources.
func (r *GCPKCCManagedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()

	log := log.FromContext(ctx)

	// Step 1: Check feature gate.
	if !feature.Gates.Enabled(feature.ConfigConnector) {
		log.V(4).Info("ConfigConnector feature gate is disabled, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Step 2: Fetch the GCPKCCManagedCluster.
	kccCluster := &infrav1exp.GCPKCCManagedCluster{}
	if err := r.Get(ctx, req.NamespacedName, kccCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 3: Fetch the owner Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kccCluster.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef, requeuing")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// Step 4: Skip if externally managed.
	if annotations.IsExternallyManaged(cluster) {
		log.Info("Cluster is externally managed, skipping")
		return ctrl.Result{}, nil
	}

	// Step 5: Set up a patcher so we persist status changes on exit.
	patchHelper, err := patch.NewHelper(kccCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, kccCluster); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Step 6: Handle pause.
	if annotations.IsPaused(cluster, kccCluster) {
		log.Info("GCPKCCManagedCluster or linked Cluster is paused")
		apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
			Type:               clusterv1.PausedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             clusterv1.PausedReason,
			Message:            "Reconciliation is paused",
			ObservedGeneration: kccCluster.Generation,
		})
		return ctrl.Result{}, nil
	}

	// Clear Paused condition if not paused.
	apimeta.RemoveStatusCondition(&kccCluster.Status.Conditions, clusterv1.PausedCondition)

	// Step 7: Handle deletion.
	if !kccCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, kccCluster)
	}

	return r.reconcileNormal(ctx, cluster, kccCluster)
}

func (r *GCPKCCManagedClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, kccCluster *infrav1exp.GCPKCCManagedCluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcluster")
	log.Info("Reconciling GCPKCCManagedCluster")

	// Add finalizer.
	if !controllerutil.ContainsFinalizer(kccCluster, infrav1exp.KCCClusterFinalizer) {
		controllerutil.AddFinalizer(kccCluster, infrav1exp.KCCClusterFinalizer)
	}

	// Reconcile ComputeNetwork.
	network, err := r.reconcileNetwork(ctx, cluster, kccCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ComputeNetwork: %w", err)
	}

	// Reconcile ComputeSubnetwork.
	subnet, err := r.reconcileSubnetwork(ctx, cluster, kccCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ComputeSubnetwork: %w", err)
	}

	// Update status names.
	kccCluster.Status.NetworkName = network.GetName()
	kccCluster.Status.SubnetworkName = subnet.GetName()

	// Check readiness.
	networkReady := isKCCResourceReady(network)
	subnetReady := isKCCResourceReady(subnet)
	provisioned := networkReady && subnetReady

	kccCluster.Status.Ready = provisioned
	if kccCluster.Status.Initialization == nil {
		kccCluster.Status.Initialization = &infrav1exp.GCPKCCManagedClusterInitializationStatus{}
	}
	kccCluster.Status.Initialization.Provisioned = &provisioned

	if !provisioned {
		log.Info("KCC network resources not yet ready, requeuing", "networkReady", networkReady, "subnetReady", subnetReady)
		apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "WaitingForKCCResources",
			Message:            "Waiting for ComputeNetwork and ComputeSubnetwork to be ready",
			ObservedGeneration: kccCluster.Generation,
		})
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Provisioned",
		Message:            "Network infrastructure is provisioned",
		ObservedGeneration: kccCluster.Generation,
	})

	log.Info("GCPKCCManagedCluster is ready")
	return ctrl.Result{}, nil
}

func (r *GCPKCCManagedClusterReconciler) reconcileDelete(ctx context.Context, _ *clusterv1.Cluster, kccCluster *infrav1exp.GCPKCCManagedCluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcluster", "action", "delete")
	log.Info("Reconciling delete GCPKCCManagedCluster")

	if !controllerutil.ContainsFinalizer(kccCluster, infrav1exp.KCCClusterFinalizer) {
		return ctrl.Result{}, nil
	}

	// Delete ComputeSubnetwork first (depends on network).
	subnetDeleted, err := r.deleteKCCResource(ctx, kccCluster, computeSubnetworkGVK, kccCluster.Spec.Subnetwork)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting ComputeSubnetwork: %w", err)
	}

	// Delete ComputeNetwork.
	networkDeleted, err := r.deleteKCCResource(ctx, kccCluster, computeNetworkGVK, kccCluster.Spec.Network)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting ComputeNetwork: %w", err)
	}

	if !subnetDeleted || !networkDeleted {
		log.Info("Waiting for KCC resources to be deleted")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// All KCC resources gone — remove finalizer.
	controllerutil.RemoveFinalizer(kccCluster, infrav1exp.KCCClusterFinalizer)
	return ctrl.Result{}, nil
}

// reconcileNetwork ensures the ComputeNetwork KCC resource exists.
func (r *GCPKCCManagedClusterReconciler) reconcileNetwork(ctx context.Context, _ *clusterv1.Cluster, kccCluster *infrav1exp.GCPKCCManagedCluster) (*unstructured.Unstructured, error) {
	desired, err := rawExtensionToUnstructured(kccCluster.Spec.Network)
	if err != nil {
		return nil, fmt.Errorf("parsing network spec: %w", err)
	}
	desired.SetGroupVersionKind(computeNetworkGVK)
	desired.SetNamespace(kccCluster.Namespace)
	setKCCOwnerReference(desired, kccCluster, "GCPKCCManagedCluster")

	return r.applyKCCResource(ctx, desired)
}

// reconcileSubnetwork ensures the ComputeSubnetwork KCC resource exists,
// patching in secondary IP ranges from Cluster.Spec.ClusterNetwork.
func (r *GCPKCCManagedClusterReconciler) reconcileSubnetwork(ctx context.Context, cluster *clusterv1.Cluster, kccCluster *infrav1exp.GCPKCCManagedCluster) (*unstructured.Unstructured, error) {
	desired, err := rawExtensionToUnstructured(kccCluster.Spec.Subnetwork)
	if err != nil {
		return nil, fmt.Errorf("parsing subnetwork spec: %w", err)
	}
	desired.SetGroupVersionKind(computeSubnetworkGVK)
	desired.SetNamespace(kccCluster.Namespace)
	setKCCOwnerReference(desired, kccCluster, "GCPKCCManagedCluster")

	// Patch secondary IP ranges from Cluster.Spec.ClusterNetwork.
	if err := patchSubnetworkCIDRs(desired, cluster); err != nil {
		return nil, err
	}

	return r.applyKCCResource(ctx, desired)
}

// patchSubnetworkCIDRs sets secondary IP ranges on the subnetwork unstructured object
// from Cluster.Spec.ClusterNetwork.Pods/Services CIDRBlocks.
func patchSubnetworkCIDRs(subnet *unstructured.Unstructured, cluster *clusterv1.Cluster) error {
	secondaryRanges, _, _ := unstructured.NestedSlice(subnet.Object, "spec", "secondaryIpRange")
	if secondaryRanges == nil {
		secondaryRanges = []interface{}{}
	}

	// Build a map of existing range names so we can update in place.
	rangeByName := map[string]int{}
	for i, entry := range secondaryRanges {
		if rm, ok := entry.(map[string]interface{}); ok {
			if name, ok := rm["rangeName"].(string); ok {
				rangeByName[name] = i
			}
		}
	}

	patchRange := func(name, cidr string) {
		if cidr == "" {
			return
		}
		if idx, found := rangeByName[name]; found {
			if rm, ok := secondaryRanges[idx].(map[string]interface{}); ok {
				rm["ipCidrRange"] = cidr
			}
		} else {
			secondaryRanges = append(secondaryRanges, map[string]interface{}{
				"rangeName":   name,
				"ipCidrRange": cidr,
			})
		}
	}

	if len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
		patchRange("pods", cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0])
	}
	if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
		patchRange("services", cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0])
	}

	if len(secondaryRanges) > 0 {
		if err := unstructured.SetNestedSlice(subnet.Object, secondaryRanges, "spec", "secondaryIpRange"); err != nil {
			return fmt.Errorf("setting secondaryIpRange: %w", err)
		}
	}
	return nil
}

// applyKCCResource creates or retrieves the KCC resource. Returns the live object.
func (r *GCPKCCManagedClusterReconciler) applyKCCResource(ctx context.Context, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, desired); createErr != nil {
			return nil, fmt.Errorf("creating %s %s: %w", desired.GetKind(), desired.GetName(), createErr)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting %s %s: %w", desired.GetKind(), desired.GetName(), err)
	}
	return existing, nil
}

// deleteKCCResource deletes the KCC resource identified by the raw extension.
// Returns true when the resource is confirmed gone.
func (r *GCPKCCManagedClusterReconciler) deleteKCCResource(ctx context.Context, owner metav1.Object, gvk schema.GroupVersionKind, raw runtime.RawExtension) (bool, error) {
	obj, err := rawExtensionToUnstructured(raw)
	if err != nil {
		return false, err
	}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(owner.GetNamespace())

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err = r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	// Issue delete if not already being deleted.
	if existing.GetDeletionTimestamp().IsZero() {
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("deleting %s %s: %w", gvk.Kind, existing.GetName(), err)
		}
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCManagedClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	// Verify that KCC CRDs are present before registering the controller.
	if err := verifyKCCCRDs(ctx, mgr.GetClient(), computeNetworkGVK, computeSubnetworkGVK); err != nil {
		return fmt.Errorf("KCC CRDs not found — install Config Connector before enabling the ConfigConnector feature gate: %w", err)
	}

	_, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1exp.GCPKCCManagedCluster{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}
	return nil
}

// --- shared helpers ---

// rawExtensionToUnstructured parses a runtime.RawExtension into an Unstructured object.
func rawExtensionToUnstructured(raw runtime.RawExtension) (*unstructured.Unstructured, error) {
	if len(raw.Raw) == 0 {
		return nil, fmt.Errorf("raw extension is empty")
	}
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(raw.Raw, &obj.Object); err != nil {
		return nil, fmt.Errorf("unmarshalling raw extension: %w", err)
	}
	return obj, nil
}

// getResourceName returns the name embedded in a raw extension JSON blob.
func getResourceName(raw runtime.RawExtension) (string, error) {
	obj, err := rawExtensionToUnstructured(raw)
	if err != nil {
		return "", err
	}
	name := obj.GetName()
	if name == "" {
		return "", fmt.Errorf("resource in raw extension has no metadata.name")
	}
	return name, nil
}

// setKCCOwnerReference sets the owner reference on a KCC resource.
func setKCCOwnerReference(obj *unstructured.Unstructured, owner metav1.Object, kind string) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == owner.GetUID() {
			return // already set
		}
	}
	obj.SetOwnerReferences(append(obj.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: infrav1exp.GroupVersion.String(),
		Kind:       kind,
		Name:       owner.GetName(),
		UID:        owner.GetUID(),
	}))
}

// isKCCResourceReady returns true if the KCC resource has a Ready=True condition.
func isKCCResourceReady(obj *unstructured.Unstructured) bool {
	conditionsRaw, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, c := range conditionsRaw {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cm["type"] == "Ready" && cm["status"] == "True" {
			return true
		}
	}
	return false
}

// verifyKCCCRDs checks that the given GVKs are discoverable in the API server.
func verifyKCCCRDs(ctx context.Context, c client.Client, gvks ...schema.GroupVersionKind) error {
	for _, gvk := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})
		if err := c.List(ctx, list, client.Limit(1)); err != nil {
			return fmt.Errorf("CRD for %s not available (is Config Connector installed?): %w", gvk.Kind, err)
		}
	}
	return nil
}
