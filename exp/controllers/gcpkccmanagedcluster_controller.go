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

	kcccomputev1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/compute/v1beta1"
	kcck8sv1alpha1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/feature"
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
	subnet, err := r.reconcileSubnetwork(ctx, cluster, kccCluster, network.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ComputeSubnetwork: %w", err)
	}

	// Update status names.
	kccCluster.Status.NetworkName = network.Name
	kccCluster.Status.SubnetworkName = subnet.Name

	// Check readiness.
	networkReady := isKCCConditionTrue(network.Status.Conditions, kcck8sv1alpha1.ReadyConditionType)
	subnetReady := isKCCConditionTrue(subnet.Status.Conditions, kcck8sv1alpha1.ReadyConditionType)
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
	subnetDeleted, err := r.deleteSubnetwork(ctx, kccCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting ComputeSubnetwork: %w", err)
	}

	// Delete ComputeNetwork.
	networkDeleted, err := r.deleteNetwork(ctx, kccCluster)
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
func (r *GCPKCCManagedClusterReconciler) reconcileNetwork(ctx context.Context, _ *clusterv1.Cluster, kccCluster *infrav1exp.GCPKCCManagedCluster) (*kcccomputev1beta1.ComputeNetwork, error) {
	desired := kccCluster.Spec.Network.DeepCopy()
	applyNetworkDefaults(desired, kccCluster.Name)
	desired.Namespace = kccCluster.Namespace
	setOwnerRef(&desired.ObjectMeta, kccCluster, "GCPKCCManagedCluster")

	existing := &kcccomputev1beta1.ComputeNetwork{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, desired); createErr != nil {
			return nil, fmt.Errorf("creating ComputeNetwork %s: %w", desired.Name, createErr)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting ComputeNetwork %s: %w", desired.Name, err)
	}
	return existing, nil
}

// reconcileSubnetwork ensures the ComputeSubnetwork KCC resource exists,
// patching in secondary IP ranges from Cluster.Spec.ClusterNetwork.
func (r *GCPKCCManagedClusterReconciler) reconcileSubnetwork(ctx context.Context, cluster *clusterv1.Cluster, kccCluster *infrav1exp.GCPKCCManagedCluster, networkName string) (*kcccomputev1beta1.ComputeSubnetwork, error) {
	desired := kccCluster.Spec.Subnetwork.DeepCopy()
	applySubnetworkDefaults(desired, kccCluster.Name, networkName)
	desired.Namespace = kccCluster.Namespace
	setOwnerRef(&desired.ObjectMeta, kccCluster, "GCPKCCManagedCluster")

	// Patch secondary IP ranges from Cluster.Spec.ClusterNetwork.
	patchSubnetworkCIDRs(desired, cluster)

	existing := &kcccomputev1beta1.ComputeSubnetwork{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, desired); createErr != nil {
			return nil, fmt.Errorf("creating ComputeSubnetwork %s: %w", desired.Name, createErr)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting ComputeSubnetwork %s: %w", desired.Name, err)
	}
	return existing, nil
}

// patchSubnetworkCIDRs sets secondary IP ranges on the subnetwork
// from Cluster.Spec.ClusterNetwork.Pods/Services CIDRBlocks.
func patchSubnetworkCIDRs(subnet *kcccomputev1beta1.ComputeSubnetwork, cluster *clusterv1.Cluster) {
	// Build a map of existing range names so we can update in place.
	rangeByName := map[string]int{}
	for i, entry := range subnet.Spec.SecondaryIpRange {
		rangeByName[entry.RangeName] = i
	}

	patchRange := func(name, cidr string) {
		if cidr == "" {
			return
		}
		if idx, found := rangeByName[name]; found {
			subnet.Spec.SecondaryIpRange[idx].IpCidrRange = cidr
		} else {
			subnet.Spec.SecondaryIpRange = append(subnet.Spec.SecondaryIpRange, kcccomputev1beta1.SubnetworkSecondaryIpRange{
				RangeName:   name,
				IpCidrRange: cidr,
			})
		}
	}

	if len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
		patchRange("pods", cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0])
	}
	if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
		patchRange("services", cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0])
	}
}

// deleteNetwork deletes the ComputeNetwork and returns true when it is gone.
func (r *GCPKCCManagedClusterReconciler) deleteNetwork(ctx context.Context, kccCluster *infrav1exp.GCPKCCManagedCluster) (bool, error) {
	networkName := kccCluster.Spec.Network.Name
	if networkName == "" {
		networkName = kccCluster.Name
	}
	existing := &kcccomputev1beta1.ComputeNetwork{}
	err := r.Get(ctx, types.NamespacedName{Name: networkName, Namespace: kccCluster.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if existing.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("deleting ComputeNetwork %s: %w", existing.Name, err)
		}
	}
	return false, nil
}

// deleteSubnetwork deletes the ComputeSubnetwork and returns true when it is gone.
func (r *GCPKCCManagedClusterReconciler) deleteSubnetwork(ctx context.Context, kccCluster *infrav1exp.GCPKCCManagedCluster) (bool, error) {
	subnetName := kccCluster.Spec.Subnetwork.Name
	if subnetName == "" {
		subnetName = kccCluster.Name
	}
	existing := &kcccomputev1beta1.ComputeSubnetwork{}
	err := r.Get(ctx, types.NamespacedName{Name: subnetName, Namespace: kccCluster.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if existing.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("deleting ComputeSubnetwork %s: %w", existing.Name, err)
		}
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCManagedClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	// Verify that KCC CRDs are present before registering the controller.
	if err := verifyKCCCRDs(ctx, mgr.GetClient(), kcccomputev1beta1.ComputeNetworkGVK, kcccomputev1beta1.ComputeSubnetworkGVK); err != nil {
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

// setOwnerRef sets the owner reference on a KCC resource's ObjectMeta.
func setOwnerRef(objMeta *metav1.ObjectMeta, owner metav1.Object, kind string) {
	for _, ref := range objMeta.OwnerReferences {
		if ref.UID == owner.GetUID() {
			return // already set
		}
	}
	objMeta.OwnerReferences = append(objMeta.OwnerReferences, metav1.OwnerReference{
		APIVersion: infrav1exp.GroupVersion.String(),
		Kind:       kind,
		Name:       owner.GetName(),
		UID:        owner.GetUID(),
	})
}

// isKCCConditionTrue returns true if the KCC resource has the specified condition set to True.
func isKCCConditionTrue(conditions []kcck8sv1alpha1.Condition, condType string) bool {
	for _, c := range conditions {
		if c.Type == condType && c.Status == corev1.ConditionTrue {
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
