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

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/util/reconciler"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// GCPKCCManagedClusterReconciler reconciles a GCPKCCManagedCluster object.
type GCPKCCManagedClusterReconciler struct {
	client.Client
	ReconcileTimeout time.Duration
	WatchFilterValue string
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes,verbs=get;list;watch
//+kubebuilder:rbac:groups=compute.cnrm.cloud.google.com,resources=computenetworks;computesubnetworks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GCPKCCManagedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()

	log := log.FromContext(ctx)

	// 1. Get GCPKCCManagedCluster.
	kccCluster := &infrav1exp.GCPKCCManagedCluster{}
	if err := r.Get(ctx, req.NamespacedName, kccCluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Get owner Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kccCluster.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	log = log.WithValues("cluster", cluster.Name)

	// 3. Check pause.
	if annotations.IsPaused(cluster, kccCluster) {
		log.Info("Reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// 4. Defer status patch.
	patchBase := client.MergeFrom(kccCluster.DeepCopy())
	defer func() {
		if err := r.Status().Patch(ctx, kccCluster, patchBase); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// 5. Branch on deletion.
	if !kccCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, kccCluster, cluster)
	}
	return r.reconcileNormal(ctx, kccCluster, cluster)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCManagedClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	if err := checkKCCCRDsPresent(ctx, mgr.GetClient(), infrav1exp.ComputeNetworkGVK, infrav1exp.ComputeSubnetworkGVK); err != nil {
		return err
	}

	networkObj := &unstructured.Unstructured{}
	networkObj.SetGroupVersionKind(infrav1exp.ComputeNetworkGVK)

	subnetworkObj := &unstructured.Unstructured{}
	subnetworkObj.SetGroupVersionKind(infrav1exp.ComputeSubnetworkGVK)

	c, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1exp.GCPKCCManagedCluster{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Owns(networkObj).
		Owns(subnetworkObj).
		Watches(
			&infrav1exp.GCPKCCManagedControlPlane{},
			handler.EnqueueRequestsFromMapFunc(r.managedControlPlaneMapper(ctx)),
		).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}

	if err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1exp.GroupVersion.WithKind("GCPKCCManagedCluster"), mgr.GetClient(), &infrav1exp.GCPKCCManagedCluster{})),
			predicates.ClusterUnpaused(mgr.GetScheme(), log),
		)); err != nil {
		return fmt.Errorf("adding watch for ready clusters: %w", err)
	}

	return nil
}

func (r *GCPKCCManagedClusterReconciler) managedControlPlaneMapper(ctx context.Context) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []ctrl.Request {
		log := ctrl.LoggerFrom(ctx)
		gcpManagedControlPlane, ok := o.(*infrav1exp.GCPKCCManagedControlPlane)
		if !ok {
			log.Error(fmt.Errorf("expected a GCPKCCManagedControlPlane, got %T instead", o), "failed to map GCPKCCManagedControlPlane")
			return nil
		}

		log = log.WithValues("objectMapper", "cpTomc", "gcpkccmanagedcontrolplane", fmt.Sprintf("%s/%s", gcpManagedControlPlane.Namespace, gcpManagedControlPlane.Name))

		if !gcpManagedControlPlane.DeletionTimestamp.IsZero() {
			log.Info("GCPKCCManagedControlPlane has a deletion timestamp, skipping mapping")
			return nil
		}

		cluster, err := util.GetOwnerCluster(ctx, r.Client, gcpManagedControlPlane.ObjectMeta)
		if err != nil {
			log.Error(err, "failed to get owning cluster")
			return nil
		}
		if cluster == nil {
			log.Info("no owning cluster, skipping mapping")
			return nil
		}

		managedClusterRef := cluster.Spec.InfrastructureRef
		if !managedClusterRef.IsDefined() || managedClusterRef.Kind != "GCPKCCManagedCluster" {
			log.Info("InfrastructureRef is nil or not GCPKCCManagedCluster, skipping mapping")
			return nil
		}

		return []ctrl.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      managedClusterRef.Name,
					Namespace: cluster.Namespace,
				},
			},
		}
	}
}

func (r *GCPKCCManagedClusterReconciler) reconcileNormal(ctx context.Context, kccCluster *infrav1exp.GCPKCCManagedCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcluster")
	log.Info("Reconciling GCPKCCManagedCluster")

	// 1. Add finalizer.
	if controllerutil.AddFinalizer(kccCluster, infrav1exp.KCCClusterFinalizer) {
		patchBase := client.MergeFrom(kccCluster.DeepCopy())
		if err := r.Patch(ctx, kccCluster, patchBase); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// 2. Get GCPKCCManagedControlPlane for endpoint propagation (optional, may not exist yet).
	var controlPlane *infrav1exp.GCPKCCManagedControlPlane
	if cluster.Spec.ControlPlaneRef.IsDefined() {
		controlPlane = &infrav1exp.GCPKCCManagedControlPlane{}
		controlPlaneRef := types.NamespacedName{
			Name:      cluster.Spec.ControlPlaneRef.Name,
			Namespace: cluster.Namespace,
		}
		if err := r.Get(ctx, controlPlaneRef, controlPlane); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, fmt.Errorf("failed to get control plane ref: %w", err)
			}
			controlPlane = nil
		}
	}

	// 3. Apply defaults.
	if err := applyNetworkDefaults(&kccCluster.Spec.Network, cluster.Name, kccCluster.Namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying network defaults: %w", err)
	}
	if err := applySubnetworkDefaults(&kccCluster.Spec.Subnetwork, cluster.Name, kccCluster.Spec.Network.Metadata.Name, kccCluster.Namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying subnetwork defaults: %w", err)
	}

	// 4. Apply CIDR overrides from Cluster.Spec.ClusterNetwork.
	var podCIDR, serviceCIDR string
	if len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
		podCIDR = cluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]
	}
	if len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
		serviceCIDR = cluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]
	}
	if err := applySubnetworkCIDROverrides(&kccCluster.Spec.Subnetwork, podCIDR, serviceCIDR); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying CIDR overrides: %w", err)
	}

	// 5. Convert to unstructured KCC resources.
	kccClusterGVK := schema.GroupVersionKind{
		Group:   infrav1exp.GroupVersion.Group,
		Version: infrav1exp.GroupVersion.Version,
		Kind:    "GCPKCCManagedCluster",
	}

	networkU, err := infrav1exp.ToUnstructuredComputeNetwork(kccCluster.Spec.Network, kccCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("converting network to unstructured: %w", err)
	}

	subnetworkU, err := infrav1exp.ToUnstructuredComputeSubnetwork(kccCluster.Spec.Subnetwork, kccCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("converting subnetwork to unstructured: %w", err)
	}

	// 6. Set owner references.
	if err := setKCCOwnerReference(kccCluster, kccClusterGVK, networkU); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting network owner reference: %w", err)
	}
	if err := setKCCOwnerReference(kccCluster, kccClusterGVK, subnetworkU); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting subnetwork owner reference: %w", err)
	}

	// 7. Create or patch KCC resources.
	if err := createOrPatchKCCResource(ctx, r.Client, networkU); err != nil {
		return ctrl.Result{}, fmt.Errorf("creating/patching KCC ComputeNetwork: %w", err)
	}
	if err := createOrPatchKCCResource(ctx, r.Client, subnetworkU); err != nil {
		return ctrl.Result{}, fmt.Errorf("creating/patching KCC ComputeSubnetwork: %w", err)
	}

	// 8. Check readiness of KCC resources.
	existingNetwork := &unstructured.Unstructured{}
	existingNetwork.SetGroupVersionKind(infrav1exp.ComputeNetworkGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: kccCluster.Spec.Network.Metadata.Name, Namespace: kccCluster.Namespace}, existingNetwork); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting KCC ComputeNetwork: %w", err)
	}
	networkReady := isKCCResourceReady(existingNetwork)

	existingSubnetwork := &unstructured.Unstructured{}
	existingSubnetwork.SetGroupVersionKind(infrav1exp.ComputeSubnetworkGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: kccCluster.Spec.Subnetwork.Metadata.Name, Namespace: kccCluster.Namespace}, existingSubnetwork); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting KCC ComputeSubnetwork: %w", err)
	}
	subnetworkReady := isKCCResourceReady(existingSubnetwork)

	// 9. Update status based on readiness.
	if networkReady && subnetworkReady {
		kccCluster.Status.Ready = true
		kccCluster.Status.Initialization = &infrav1exp.GCPKCCManagedClusterInitializationStatus{
			Provisioned: ptr.To(true),
		}
		kccCluster.Status.NetworkName = kccCluster.Spec.Network.Metadata.Name
		kccCluster.Status.SubnetworkName = kccCluster.Spec.Subnetwork.Metadata.Name

		apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
			Type:               infrav1exp.KCCNetworkReadyCondition,
			Status:             metav1.ConditionTrue,
			Reason:             clusterv1.ReadyReason,
			LastTransitionTime: metav1.Now(),
		})
		apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
			Type:               infrav1exp.KCCSubnetworkReadyCondition,
			Status:             metav1.ConditionTrue,
			Reason:             clusterv1.ReadyReason,
			LastTransitionTime: metav1.Now(),
		})

		// Propagate endpoint from control plane if available.
		if controlPlane != nil && !controlPlane.Spec.ControlPlaneEndpoint.IsZero() {
			kccCluster.Spec.ControlPlaneEndpoint = controlPlane.Spec.ControlPlaneEndpoint
		}

		log.Info("GCPKCCManagedCluster is ready")
		return ctrl.Result{}, nil
	}

	// 10. Not ready: set conditions with KCC messages and requeue.
	if !networkReady {
		msg := getKCCConditionMessage(existingNetwork)
		if msg == "" {
			msg = "KCC ComputeNetwork is not yet ready"
		}
		apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
			Type:               infrav1exp.KCCNetworkReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             clusterv1.NotReadyReason,
			Message:            msg,
			LastTransitionTime: metav1.Now(),
		})
	}
	if !subnetworkReady {
		msg := getKCCConditionMessage(existingSubnetwork)
		if msg == "" {
			msg = "KCC ComputeSubnetwork is not yet ready"
		}
		apimeta.SetStatusCondition(&kccCluster.Status.Conditions, metav1.Condition{
			Type:               infrav1exp.KCCSubnetworkReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             clusterv1.NotReadyReason,
			Message:            msg,
			LastTransitionTime: metav1.Now(),
		})
	}

	log.Info("KCC resources not yet ready, requeueing", "networkReady", networkReady, "subnetworkReady", subnetworkReady)
	return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
}

func (r *GCPKCCManagedClusterReconciler) reconcileDelete(ctx context.Context, kccCluster *infrav1exp.GCPKCCManagedCluster, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcluster", "action", "delete")
	log.Info("Reconciling Delete GCPKCCManagedCluster")

	// Delete KCC ComputeSubnetwork (delete subnet before network).
	subnetworkGone, err := deleteResource(ctx, r.Client, infrav1exp.ComputeSubnetworkGVK, kccCluster.Spec.Subnetwork.Metadata.Name, kccCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting KCC ComputeSubnetwork: %w", err)
	}

	// Delete KCC ComputeNetwork.
	networkGone, err := deleteResource(ctx, r.Client, infrav1exp.ComputeNetworkGVK, kccCluster.Spec.Network.Metadata.Name, kccCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting KCC ComputeNetwork: %w", err)
	}

	// If both resources are gone, remove the finalizer.
	if networkGone && subnetworkGone {
		controllerutil.RemoveFinalizer(kccCluster, infrav1exp.KCCClusterFinalizer)
		if err := r.Patch(ctx, kccCluster, client.MergeFrom(kccCluster.DeepCopy())); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
		log.Info("GCPKCCManagedCluster deletion complete")
		return ctrl.Result{}, nil
	}

	log.Info("KCC resources still being deleted, requeueing", "networkGone", networkGone, "subnetworkGone", subnetworkGone)
	return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
}

// Ensure GCPKCCManagedClusterReconciler implements reconcile.Reconciler.
var _ reconcile.Reconciler = &GCPKCCManagedClusterReconciler{}
