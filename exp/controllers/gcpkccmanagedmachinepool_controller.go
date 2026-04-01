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

	"github.com/go-logr/logr"
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

	infrav1v2 "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta2"
	"sigs.k8s.io/cluster-api-provider-gcp/util/reconciler"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// GCPKCCManagedMachinePoolReconciler reconciles a GCPKCCManagedMachinePool object.
type GCPKCCManagedMachinePoolReconciler struct {
	client.Client
	ReconcileTimeout time.Duration
	WatchFilterValue string
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedmachinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedmachinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedmachinepools/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes,verbs=get;list;watch
//+kubebuilder:rbac:groups=container.cnrm.cloud.google.com,resources=containernodepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinepools;machinepools/status,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GCPKCCManagedMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()

	log := log.FromContext(ctx)

	// 1. Get GCPKCCManagedMachinePool.
	kccMMP := &infrav1v2.GCPKCCManagedMachinePool{}
	if err := r.Get(ctx, req.NamespacedName, kccMMP); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Get owner MachinePool.
	var machinePool *clusterv1.MachinePool
	for _, ref := range kccMMP.OwnerReferences {
		if ref.Kind == "MachinePool" {
			mp := &clusterv1.MachinePool{}
			if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: kccMMP.Namespace}, mp); err != nil {
				return ctrl.Result{}, fmt.Errorf("getting owner MachinePool: %w", err)
			}
			machinePool = mp
			break
		}
	}
	if machinePool == nil {
		log.Info("MachinePool Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	// 3. Get owner Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("Failed to retrieve Cluster from MachinePool")
		return ctrl.Result{}, err
	}
	log = log.WithValues("cluster", cluster.Name)

	// 4. Check pause.
	if annotations.IsPaused(cluster, kccMMP) {
		log.Info("Reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// 5. Defer patch — snapshots the object now and patches spec+status together
	// at the end of reconciliation, matching the scope-based pattern used by
	// existing CAPG controllers. This avoids issues with separate spec/status
	// patches overwriting each other's in-memory changes.
	patchHelper, err := patch.NewHelper(kccMMP, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, kccMMP); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// 6. Branch on deletion.
	if !kccMMP.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, kccMMP)
	}
	return r.reconcileNormal(ctx, kccMMP, machinePool, cluster)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCManagedMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	if err := checkKCCCRDsPresent(ctx, mgr.GetClient(), infrav1v2.ContainerNodePoolGVK); err != nil {
		return err
	}

	containerNodePoolObj := &unstructured.Unstructured{}
	containerNodePoolObj.SetGroupVersionKind(infrav1v2.ContainerNodePoolGVK)

	kccMMPGVK := schema.GroupVersionKind{
		Group:   infrav1v2.GroupVersion.Group,
		Version: infrav1v2.GroupVersion.Version,
		Kind:    "GCPKCCManagedMachinePool",
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1v2.GCPKCCManagedMachinePool{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Owns(containerNodePoolObj).
		Watches(
			&clusterv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(machinePoolToKCCInfrastructureMapFunc(kccMMPGVK)),
		).
		Watches(
			&infrav1v2.GCPKCCManagedControlPlane{},
			handler.EnqueueRequestsFromMapFunc(controlPlaneToKCCMachinePoolMapFunc(r.Client, kccMMPGVK, log)),
		).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}

	clusterToObjectFunc, err := util.ClusterToTypedObjectsMapper(r.Client, &infrav1v2.GCPKCCManagedMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return fmt.Errorf("failed to create mapper for Cluster to GCPKCCManagedMachinePools: %w", err)
	}

	if err := c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToObjectFunc),
			predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), log),
		)); err != nil {
		return fmt.Errorf("failed adding a watch for ready clusters: %w", err)
	}

	return nil
}

// machinePoolToKCCInfrastructureMapFunc maps a MachinePool to its infrastructure
// ref if the GVK matches GCPKCCManagedMachinePool.
func machinePoolToKCCInfrastructureMapFunc(gvk schema.GroupVersionKind) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		m, ok := o.(*clusterv1.MachinePool)
		if !ok {
			panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
		}

		gk := gvk.GroupKind()
		infraGK := m.Spec.Template.Spec.InfrastructureRef.GroupKind()
		if gk != infraGK {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: m.Namespace,
					Name:      m.Spec.Template.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// controlPlaneToKCCMachinePoolMapFunc maps ControlPlane changes to all machine
// pools in the cluster that reference GCPKCCManagedMachinePool.
func controlPlaneToKCCMachinePoolMapFunc(c client.Client, gvk schema.GroupVersionKind, log logr.Logger) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		kccCP, ok := o.(*infrav1v2.GCPKCCManagedControlPlane)
		if !ok {
			panic(fmt.Sprintf("Expected a GCPKCCManagedControlPlane but got a %T", o))
		}

		if !kccCP.DeletionTimestamp.IsZero() {
			return nil
		}

		clusterKey, err := GetOwnerClusterKey(kccCP.ObjectMeta)
		if err != nil {
			log.Error(err, "couldn't get GCPKCCManagedControlPlane owner ObjectKey")
			return nil
		}
		if clusterKey == nil {
			return nil
		}

		managedPoolForClusterList := clusterv1.MachinePoolList{}
		if err := c.List(
			ctx, &managedPoolForClusterList, client.InNamespace(clusterKey.Namespace), client.MatchingLabels{clusterv1.ClusterNameLabel: clusterKey.Name},
		); err != nil {
			log.Error(err, "couldn't list pools for cluster")
			return nil
		}

		mapFunc := machinePoolToKCCInfrastructureMapFunc(gvk)

		var results []ctrl.Request
		for i := range managedPoolForClusterList.Items {
			managedPool := mapFunc(ctx, &managedPoolForClusterList.Items[i])
			results = append(results, managedPool...)
		}

		return results
	}
}

func (r *GCPKCCManagedMachinePoolReconciler) reconcileNormal(ctx context.Context, kccMMP *infrav1v2.GCPKCCManagedMachinePool, machinePool *clusterv1.MachinePool, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedmachinepool")
	log.Info("Reconciling GCPKCCManagedMachinePool")

	// 1. Add finalizer.
	if controllerutil.AddFinalizer(kccMMP, infrav1v2.KCCManagedMachinePoolFinalizer) {
		patchBase := client.MergeFrom(kccMMP.DeepCopy())
		if err := r.Patch(ctx, kccMMP, patchBase); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// 2. Get GCPKCCManagedControlPlane, gate on controlPlaneInitialized.
	kccCP := &infrav1v2.GCPKCCManagedControlPlane{}
	controlPlaneRef := types.NamespacedName{
		Name:      cluster.Spec.ControlPlaneRef.Name,
		Namespace: cluster.Namespace,
	}
	if err := r.Get(ctx, controlPlaneRef, kccCP); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get GCPKCCManagedControlPlane: %w", err)
	}

	if kccCP.Status.Initialization == nil || kccCP.Status.Initialization.ControlPlaneInitialized == nil || !*kccCP.Status.Initialization.ControlPlaneInitialized {
		log.Info("Waiting for control plane to be initialized")
		apimeta.SetStatusCondition(&kccMMP.Status.Conditions, metav1.Condition{
			Type:    infrav1v2.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.WaitingForControlPlaneInitializedReason,
			Message: "Waiting for control plane to be initialized",
		})
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// 3. Get GCPKCCManagedCluster for location defaulting.
	kccInfraCluster := &infrav1v2.GCPKCCManagedCluster{}
	infraClusterRef := types.NamespacedName{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Namespace,
	}
	if err := r.Get(ctx, infraClusterRef, kccInfraCluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get GCPKCCManagedCluster: %w", err)
	}

	// 4. Read the live KCC ContainerCluster for location (populated via state-into-spec: merge).
	existingCluster := &unstructured.Unstructured{}
	existingCluster.SetGroupVersionKind(infrav1v2.ContainerClusterGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: kccCP.Status.ClusterName, Namespace: kccMMP.Namespace}, existingCluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting KCC ContainerCluster: %w", err)
	}

	// 5. Apply defaults and CAPI overrides.
	if err := applyMachinePoolDefaults(kccMMP, machinePool, kccCP, existingCluster, kccInfraCluster); err != nil {
		apimeta.SetStatusCondition(&kccMMP.Status.Conditions, metav1.Condition{
			Type:    infrav1v2.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1v2.ConfigurationErrorReason,
			Message: err.Error(),
		})
		return ctrl.Result{}, fmt.Errorf("applying defaults: %w", err)
	}

	// 6. Convert to unstructured ContainerNodePool.
	containerNodePoolU, err := infrav1v2.ToUnstructured(kccMMP.Spec.NodePool, infrav1v2.ContainerNodePoolGVK)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("converting ContainerNodePool to unstructured: %w", err)
	}

	// 5. Set owner ref, create or patch.
	kccMMPGVK := schema.GroupVersionKind{
		Group:   infrav1v2.GroupVersion.Group,
		Version: infrav1v2.GroupVersion.Version,
		Kind:    "GCPKCCManagedMachinePool",
	}
	if err := setKCCOwnerReference(kccMMP, kccMMPGVK, containerNodePoolU); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting ContainerNodePool owner reference: %w", err)
	}

	if err := applyKCCResource(ctx, r.Client, containerNodePoolU); err != nil {
		return ctrl.Result{}, fmt.Errorf("creating/patching KCC ContainerNodePool: %w", err)
	}

	// 6. Check readiness.
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(infrav1v2.ContainerNodePoolGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: containerNodePoolU.GetName(), Namespace: kccMMP.Namespace}, existing); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting KCC ContainerNodePool: %w", err)
	}

	ready, readyMsg := getKCCReadiness(existing)

	// 7. If ready, set status fields and populate providerIDList from workload cluster.
	if ready {
		kccMMP.Status.Ready = true
		kccMMP.Status.Initialization = &infrav1v2.GCPKCCManagedMachinePoolInitializationStatus{
			Provisioned: ptr.To(true),
		}
		kccMMP.Status.NodePoolName = containerNodePoolU.GetName()

		// Read per-zone node count from KCC resource (populated via
		// state-into-spec merge) as fallback. Multiply by zone count for total.
		nodeCount, nodeCountFound, _ := unstructured.NestedInt64(existing.Object, "spec", "nodeCount")
		if nodeCountFound {
			total := int32(nodeCount)
			nodeLocs, found, _ := unstructured.NestedStringSlice(existing.Object, "spec", "nodeLocations")
			if found && len(nodeLocs) > 0 {
				total *= int32(len(nodeLocs))
			}
			kccMMP.Status.Replicas = total
		}

		// Read observed version from the KCC ContainerNodePool.
		if version, _ := getStatusFieldFromUnstructured(existing, "observedState", "version"); version != "" {
			kccMMP.Status.Version = &version
		}

		// Populate providerIDList and readyReplicas from workload cluster nodes.
		// Use resourceID as the GKE node pool name (nodes are labeled with it),
		// falling back to the Kubernetes resource name if resourceID is not set.
		gkePoolName, _, _ := unstructured.NestedString(existing.Object, "spec", "resourceID")
		if gkePoolName == "" {
			gkePoolName = containerNodePoolU.GetName()
		}
		npInfo, err := getNodePoolInfoFromWorkloadCluster(ctx, r.Client, cluster.Name, kccMMP.Namespace, gkePoolName)
		if err != nil {
			log.Error(err, "Failed to get node pool info from workload cluster, will retry")
		} else if len(npInfo.ProviderIDList) > 0 {
			kccMMP.Spec.ProviderIDList = npInfo.ProviderIDList
			kccMMP.Status.Replicas = npInfo.Replicas
		}

		apimeta.SetStatusCondition(&kccMMP.Status.Conditions, metav1.Condition{
			Type:    infrav1v2.ReadyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.ReadyReason,
			Message: "KCC ContainerNodePool is ready",
		})

		log.Info("GCPKCCManagedMachinePool is ready")
		return ctrl.Result{}, nil
	}

	// 8. Not ready: requeue.
	msg := readyMsg
	if msg == "" {
		msg = "KCC ContainerNodePool is not yet ready"
	}
	apimeta.SetStatusCondition(&kccMMP.Status.Conditions, metav1.Condition{
		Type:    infrav1v2.ReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.NotReadyReason,
		Message: msg,
	})

	log.Info("KCC ContainerNodePool not yet ready, requeueing")
	return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
}

func (r *GCPKCCManagedMachinePoolReconciler) reconcileDelete(ctx context.Context, kccMMP *infrav1v2.GCPKCCManagedMachinePool) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedmachinepool", "action", "delete")
	log.Info("Reconciling Delete GCPKCCManagedMachinePool")

	// 1. Delete the ContainerNodePool KCC resource.
	gone, err := deleteResource(ctx, r.Client, infrav1v2.ContainerNodePoolGVK, getRawName(kccMMP.Spec.NodePool), kccMMP.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting KCC ContainerNodePool: %w", err)
	}

	// 2. Wait for it to be gone.
	if !gone {
		log.Info("KCC ContainerNodePool still being deleted, requeueing")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// 3. Remove finalizer.
	controllerutil.RemoveFinalizer(kccMMP, infrav1v2.KCCManagedMachinePoolFinalizer)
	if err := r.Patch(ctx, kccMMP, client.MergeFrom(kccMMP.DeepCopy())); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	log.Info("GCPKCCManagedMachinePool deletion complete")
	return ctrl.Result{}, nil
}

// Ensure GCPKCCManagedMachinePoolReconciler implements reconcile.Reconciler.
var _ reconcile.Reconciler = &GCPKCCManagedMachinePoolReconciler{}
