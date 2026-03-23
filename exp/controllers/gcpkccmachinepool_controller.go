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

	kcccontainerv1beta1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/container/v1beta1"
	kcck8sv1alpha1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/feature"
	"sigs.k8s.io/cluster-api-provider-gcp/util/reconciler"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GCPKCCMachinePoolReconciler reconciles a GCPKCCMachinePool object.
type GCPKCCMachinePoolReconciler struct {
	client.Client
	WatchFilterValue string
	ReconcileTimeout time.Duration
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmachinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmachinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmachinepools/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes,verbs=get;list;watch
//+kubebuilder:rbac:groups=container.cnrm.cloud.google.com,resources=containernodepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile reconciles a GCPKCCMachinePool by managing a Config Connector
// ContainerNodePool resource and populating spec.providerIDList.
func (r *GCPKCCMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()

	log := log.FromContext(ctx)

	// Step 1: Check feature gate.
	if !feature.Gates.Enabled(feature.ConfigConnector) {
		log.V(4).Info("ConfigConnector feature gate is disabled, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Step 2: Fetch the GCPKCCMachinePool.
	kccMP := &infrav1exp.GCPKCCMachinePool{}
	if err := r.Get(ctx, req.NamespacedName, kccMP); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 3: Fetch the owner Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kccMP.ObjectMeta)
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

	// Step 5: Set up a patcher.
	patchHelper, err := patch.NewHelper(kccMP, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, kccMP); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Step 6: Handle pause.
	if annotations.IsPaused(cluster, kccMP) {
		log.Info("GCPKCCMachinePool or linked Cluster is paused")
		apimeta.SetStatusCondition(&kccMP.Status.Conditions, metav1.Condition{
			Type:               clusterv1.PausedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             clusterv1.PausedReason,
			Message:            "Reconciliation is paused",
			ObservedGeneration: kccMP.Generation,
		})
		return ctrl.Result{}, nil
	}
	apimeta.RemoveStatusCondition(&kccMP.Status.Conditions, clusterv1.PausedCondition)

	// Step 7: Handle deletion.
	if !kccMP.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, kccMP)
	}

	return r.reconcileNormal(ctx, cluster, kccMP)
}

func (r *GCPKCCMachinePoolReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, kccMP *infrav1exp.GCPKCCMachinePool) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmachinepool")
	log.Info("Reconciling GCPKCCMachinePool")

	// Add finalizer.
	if !controllerutil.ContainsFinalizer(kccMP, infrav1exp.KCCMachinePoolFinalizer) {
		controllerutil.AddFinalizer(kccMP, infrav1exp.KCCMachinePoolFinalizer)
	}

	// Gate on GCPKCCManagedControlPlane being initialized.
	cpInitialized, err := r.isControlPlaneInitialized(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !cpInitialized {
		log.Info("Waiting for GCPKCCManagedControlPlane to be initialized")
		apimeta.SetStatusCondition(&kccMP.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "WaitingForControlPlane",
			Message:            "Waiting for GCPKCCManagedControlPlane to be initialized",
			ObservedGeneration: kccMP.Generation,
		})
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// Reconcile ContainerNodePool.
	nodePool, err := r.reconcileNodePool(ctx, cluster, kccMP)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ContainerNodePool: %w", err)
	}

	// Check if ContainerNodePool is ready.
	if !isKCCConditionTrue(nodePool.Status.Conditions, kcck8sv1alpha1.ReadyConditionType) {
		log.Info("ContainerNodePool not yet ready, requeuing")
		apimeta.SetStatusCondition(&kccMP.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "WaitingForNodePool",
			Message:            "Waiting for ContainerNodePool to be ready",
			ObservedGeneration: kccMP.Generation,
		})
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Populate spec.providerIDList from workload cluster Node objects.
	if err := r.reconcileProviderIDList(ctx, cluster, kccMP); err != nil {
		// Non-fatal: kubeconfig may not be available yet.
		log.Info("Could not populate providerIDList yet, will retry", "err", err)
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// Update replicas from providerIDList.
	kccMP.Status.Replicas = int32(len(kccMP.Spec.ProviderIDList))
	kccMP.Status.ReadyReplicas = kccMP.Status.Replicas

	provisioned := true
	kccMP.Status.Ready = true
	if kccMP.Status.Initialization == nil {
		kccMP.Status.Initialization = &infrav1exp.GCPKCCMachinePoolInitializationStatus{}
	}
	kccMP.Status.Initialization.Provisioned = &provisioned

	apimeta.SetStatusCondition(&kccMP.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Provisioned",
		Message:            "Node pool is provisioned",
		ObservedGeneration: kccMP.Generation,
	})

	log.Info("GCPKCCMachinePool is ready")
	return ctrl.Result{}, nil
}

func (r *GCPKCCMachinePoolReconciler) reconcileDelete(ctx context.Context, _ *clusterv1.Cluster, kccMP *infrav1exp.GCPKCCMachinePool) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmachinepool", "action", "delete")
	log.Info("Reconciling delete GCPKCCMachinePool")

	if !controllerutil.ContainsFinalizer(kccMP, infrav1exp.KCCMachinePoolFinalizer) {
		return ctrl.Result{}, nil
	}

	nodePoolDeleted, err := r.deleteNodePool(ctx, kccMP)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !nodePoolDeleted {
		log.Info("Waiting for ContainerNodePool to be deleted")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	controllerutil.RemoveFinalizer(kccMP, infrav1exp.KCCMachinePoolFinalizer)
	return ctrl.Result{}, nil
}

// reconcileNodePool creates or retrieves the ContainerNodePool KCC resource.
func (r *GCPKCCMachinePoolReconciler) reconcileNodePool(ctx context.Context, _ *clusterv1.Cluster, kccMP *infrav1exp.GCPKCCMachinePool) (*kcccontainerv1beta1.ContainerNodePool, error) {
	desired := kccMP.Spec.NodePool.DeepCopy()
	desired.Namespace = kccMP.Namespace
	setOwnerRef(&desired.ObjectMeta, kccMP, "GCPKCCMachinePool")

	existing := &kcccontainerv1beta1.ContainerNodePool{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, desired); createErr != nil {
			return nil, fmt.Errorf("creating ContainerNodePool %s: %w", desired.Name, createErr)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting ContainerNodePool %s: %w", desired.Name, err)
	}
	return existing, nil
}

// reconcileProviderIDList populates spec.providerIDList by listing Node objects
// in the workload cluster and reading node.Spec.ProviderID.
func (r *GCPKCCMachinePoolReconciler) reconcileProviderIDList(ctx context.Context, cluster *clusterv1.Cluster, kccMP *infrav1exp.GCPKCCMachinePool) error {
	// Fetch the kubeconfig secret for the workload cluster.
	kubeconfigSecret := &corev1.Secret{}
	secretName := secret.Name(cluster.Name, secret.Kubeconfig)
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: kccMP.Namespace}, kubeconfigSecret); err != nil {
		return fmt.Errorf("kubeconfig secret %s not yet available: %w", secretName, err)
	}

	kubeconfigData, ok := kubeconfigSecret.Data[secret.KubeconfigDataName]
	if !ok || len(kubeconfigData) == 0 {
		return fmt.Errorf("kubeconfig secret %s has no data", secretName)
	}

	// Build a client for the workload cluster.
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return fmt.Errorf("parsing kubeconfig: %w", err)
	}

	workloadClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return fmt.Errorf("creating workload cluster client: %w", err)
	}

	// List Node objects and collect ProviderIDs.
	nodeList := &corev1.NodeList{}
	if err := workloadClient.List(ctx, nodeList); err != nil {
		return fmt.Errorf("listing nodes in workload cluster: %w", err)
	}

	providerIDs := make([]string, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		if node.Spec.ProviderID != "" {
			providerIDs = append(providerIDs, node.Spec.ProviderID)
		}
	}

	kccMP.Spec.ProviderIDList = providerIDs
	return nil
}

// deleteNodePool deletes the ContainerNodePool and returns true when it is gone.
func (r *GCPKCCMachinePoolReconciler) deleteNodePool(ctx context.Context, kccMP *infrav1exp.GCPKCCMachinePool) (bool, error) {
	existing := &kcccontainerv1beta1.ContainerNodePool{}
	err := r.Get(ctx, types.NamespacedName{Name: kccMP.Spec.NodePool.Name, Namespace: kccMP.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if existing.DeletionTimestamp.IsZero() {
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("deleting ContainerNodePool %s: %w", existing.Name, err)
		}
	}
	return false, nil
}

// isControlPlaneInitialized returns true if the GCPKCCManagedControlPlane is initialized.
func (r *GCPKCCMachinePoolReconciler) isControlPlaneInitialized(ctx context.Context, cluster *clusterv1.Cluster) (bool, error) {
	if !cluster.Spec.ControlPlaneRef.IsDefined() {
		return false, nil
	}
	cpRef := cluster.Spec.ControlPlaneRef
	kccCP := &infrav1exp.GCPKCCManagedControlPlane{}
	if err := r.Get(ctx, types.NamespacedName{Name: cpRef.Name, Namespace: cluster.Namespace}, kccCP); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if kccCP.Status.Initialization == nil || kccCP.Status.Initialization.ControlPlaneInitialized == nil {
		return false, nil
	}
	return *kccCP.Status.Initialization.ControlPlaneInitialized, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	// Verify that KCC CRDs are present.
	if err := verifyKCCCRDs(ctx, mgr.GetClient(), kcccontainerv1beta1.ContainerNodePoolGVK); err != nil {
		return fmt.Errorf("KCC CRDs not found — install Config Connector before enabling the ConfigConnector feature gate: %w", err)
	}

	_, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1exp.GCPKCCMachinePool{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}
	return nil
}
