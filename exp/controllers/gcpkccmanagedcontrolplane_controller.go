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
	"encoding/base64"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// GCPKCCManagedControlPlaneReconciler reconciles a GCPKCCManagedControlPlane object.
type GCPKCCManagedControlPlaneReconciler struct {
	client.Client
	ReconcileTimeout time.Duration
	WatchFilterValue string
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=container.cnrm.cloud.google.com,resources=containerclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GCPKCCManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()

	log := log.FromContext(ctx)

	// 1. Get GCPKCCManagedControlPlane.
	kccCP := &infrav1exp.GCPKCCManagedControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, kccCP); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Get owner Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kccCP.ObjectMeta)
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
	if annotations.IsPaused(cluster, kccCP) {
		log.Info("Reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// 4. Defer patch — snapshots the object now and patches spec+status together
	// at the end of reconciliation, matching the scope-based pattern used by
	// existing CAPG controllers.
	patchHelper, err := patch.NewHelper(kccCP, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}
	defer func() {
		if err := patchHelper.Patch(ctx, kccCP); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// 5. Branch on deletion.
	if !kccCP.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, kccCP, cluster)
	}
	return r.reconcileNormal(ctx, kccCP, cluster)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	if err := checkKCCCRDsPresent(ctx, mgr.GetClient(), infrav1exp.ContainerClusterGVK); err != nil {
		return err
	}

	containerClusterObj := &unstructured.Unstructured{}
	containerClusterObj.SetGroupVersionKind(infrav1exp.ContainerClusterGVK)

	c, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1exp.GCPKCCManagedControlPlane{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Owns(containerClusterObj).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}

	if err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1exp.GroupVersion.WithKind("GCPKCCManagedControlPlane"), mgr.GetClient(), &infrav1exp.GCPKCCManagedControlPlane{})),
			predicates.ClusterUnpaused(mgr.GetScheme(), log),
		)); err != nil {
		return fmt.Errorf("adding watch for ready clusters: %w", err)
	}

	return nil
}

func (r *GCPKCCManagedControlPlaneReconciler) reconcileNormal(ctx context.Context, kccCP *infrav1exp.GCPKCCManagedControlPlane, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcontrolplane")
	log.Info("Reconciling GCPKCCManagedControlPlane")

	// 1. Add finalizer.
	if controllerutil.AddFinalizer(kccCP, infrav1exp.KCCManagedControlPlaneFinalizer) {
		patchBase := client.MergeFrom(kccCP.DeepCopy())
		if err := r.Patch(ctx, kccCP, patchBase); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	// 2. Get GCPKCCManagedCluster from cluster.Spec.InfrastructureRef.
	kccInfraCluster := &infrav1exp.GCPKCCManagedCluster{}
	infraClusterRef := types.NamespacedName{
		Name:      cluster.Spec.InfrastructureRef.Name,
		Namespace: cluster.Namespace,
	}
	if err := r.Get(ctx, infraClusterRef, kccInfraCluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get GCPKCCManagedCluster: %w", err)
	}

	// 3. Gate on infra cluster provisioned.
	if kccInfraCluster.Status.Initialization == nil || kccInfraCluster.Status.Initialization.Provisioned == nil || !*kccInfraCluster.Status.Initialization.Provisioned {
		log.Info("Waiting for infrastructure cluster to be provisioned")
		apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
			Type:    infrav1exp.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.WaitingForClusterInfrastructureReadyReason,
			Message: "Waiting for infrastructure cluster to be provisioned",
		})
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// 4. Apply defaults and CAPI overrides.
	if err := applyControlPlaneDefaults(kccCP, cluster, kccInfraCluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("applying defaults: %w", err)
	}

	// 5. Convert to unstructured ContainerCluster.
	containerClusterU, err := infrav1exp.ToUnstructured(kccCP.Spec.ContainerCluster, infrav1exp.ContainerClusterGVK)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("converting ContainerCluster to unstructured: %w", err)
	}

	// 6. Set owner ref, create or patch.
	kccCPGVK := schema.GroupVersionKind{
		Group:   infrav1exp.GroupVersion.Group,
		Version: infrav1exp.GroupVersion.Version,
		Kind:    "GCPKCCManagedControlPlane",
	}
	if err := setKCCOwnerReference(kccCP, kccCPGVK, containerClusterU); err != nil {
		return ctrl.Result{}, fmt.Errorf("setting ContainerCluster owner reference: %w", err)
	}

	if err := applyKCCResource(ctx, r.Client, containerClusterU); err != nil {
		return ctrl.Result{}, fmt.Errorf("creating/patching KCC ContainerCluster: %w", err)
	}

	// 7. Check readiness.
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(infrav1exp.ContainerClusterGVK)
	if err := r.Get(ctx, types.NamespacedName{Name: containerClusterU.GetName(), Namespace: kccCP.Namespace}, existing); err != nil {
		return ctrl.Result{}, fmt.Errorf("getting KCC ContainerCluster: %w", err)
	}

	ready, readyMsg := getKCCReadiness(existing)

	// 8. If KCC resource is ready, generate kubeconfig and set status.
	if ready {
		endpoint, _ := getStatusFieldFromUnstructured(existing, "endpoint")
		caCert, _ := getStatusFieldFromUnstructured(existing, "observedState", "masterAuth", "clusterCaCertificate")

		// Don't mark ready until we have both endpoint and CA cert for kubeconfig.
		if endpoint == "" || caCert == "" {
			log.Info("KCC ContainerCluster is ready but endpoint or CA cert not yet available, requeueing")
			apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
				Type:    infrav1exp.ReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.NotReadyReason,
				Message: "Waiting for endpoint and CA certificate",
			})
			return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
		}

		// Generate kubeconfig before marking ready.
		if err := r.reconcileKubeconfig(ctx, kccCP, cluster, endpoint, caCert); err != nil {
			return ctrl.Result{}, fmt.Errorf("reconciling kubeconfig: %w", err)
		}

		kccCP.Spec.ControlPlaneEndpoint = clusterv1beta1.APIEndpoint{
			Host: endpoint,
			Port: 443,
		}
		kccCP.Status.ExternalManagedControlPlane = ptr.To(true)
		kccCP.Status.Ready = true
		kccCP.Status.Initialized = true
		kccCP.Status.Initialization = &infrav1exp.GCPKCCManagedControlPlaneInitializationStatus{
			ControlPlaneInitialized: ptr.To(true),
		}
		kccCP.Status.ClusterName = containerClusterU.GetName()

		masterVersion, _ := getStatusFieldFromUnstructured(existing, "masterVersion")
		if masterVersion != "" {
			kccCP.Status.Version = &masterVersion
		}

		apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
			Type:    infrav1exp.ReadyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.ReadyReason,
			Message: "KCC ContainerCluster is ready",
		})

		log.Info("GCPKCCManagedControlPlane is ready")
		return ctrl.Result{}, nil
	}

	// 9. Not ready: requeue.
	msg := readyMsg
	if msg == "" {
		msg = "KCC ContainerCluster is not yet ready"
	}
	apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
		Type:    infrav1exp.ReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.NotReadyReason,
		Message: msg,
	})

	log.Info("KCC ContainerCluster not yet ready, requeueing")
	return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
}

func (r *GCPKCCManagedControlPlaneReconciler) reconcileDelete(ctx context.Context, kccCP *infrav1exp.GCPKCCManagedControlPlane, _ *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcontrolplane", "action", "delete")
	log.Info("Reconciling Delete GCPKCCManagedControlPlane")

	// 1. Delete the ContainerCluster KCC resource.
	gone, err := deleteResource(ctx, r.Client, infrav1exp.ContainerClusterGVK, getRawName(kccCP.Spec.ContainerCluster), kccCP.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("deleting KCC ContainerCluster: %w", err)
	}

	// 2. Wait for it to be gone.
	if !gone {
		log.Info("KCC ContainerCluster still being deleted, requeueing")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// 3. Remove finalizer.
	controllerutil.RemoveFinalizer(kccCP, infrav1exp.KCCManagedControlPlaneFinalizer)
	if err := r.Patch(ctx, kccCP, client.MergeFrom(kccCP.DeepCopy())); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
	}

	log.Info("GCPKCCManagedControlPlane deletion complete")
	return ctrl.Result{}, nil
}

// reconcileKubeconfig creates or updates the kubeconfig Secret for the cluster.
func (r *GCPKCCManagedControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, kccCP *infrav1exp.GCPKCCManagedControlPlane, cluster *clusterv1.Cluster, endpoint, caCert string) error {
	// Check if secret already exists.
	secretName := fmt.Sprintf("%s-kubeconfig", cluster.Name)
	secretKey := types.NamespacedName{Name: secretName, Namespace: cluster.Namespace}

	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, secretKey, existingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Decode CA cert.
	caData, err2 := base64.StdEncoding.DecodeString(caCert)
	if err2 != nil {
		return fmt.Errorf("decoding CA certificate: %w", err2)
	}

	// Generate a bearer token from Application Default Credentials.
	token, err4 := generateGCPAccessToken(ctx)
	if err4 != nil {
		return fmt.Errorf("generating access token for kubeconfig: %w", err4)
	}

	// Build kubeconfig with bearer token.
	contextName := fmt.Sprintf("gke_%s", cluster.Name)
	kubeconfigData := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			contextName: {
				Server:                   fmt.Sprintf("https://%s", endpoint),
				CertificateAuthorityData: caData,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  contextName,
				AuthInfo: contextName,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			contextName: {
				Token: token,
			},
		},
		CurrentContext: contextName,
	}

	out, err3 := clientcmd.Write(kubeconfigData)
	if err3 != nil {
		return fmt.Errorf("writing kubeconfig: %w", err3)
	}

	if apierrors.IsNotFound(err) {
		// Create new secret.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterNameLabel: cluster.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(kccCP, infrav1exp.GroupVersion.WithKind("GCPKCCManagedControlPlane")),
				},
			},
			Type: clusterv1.ClusterSecretType,
			Data: map[string][]byte{
				"value": out,
			},
		}
		return r.Create(ctx, secret)
	}

	// Update existing secret.
	existingSecret.Data = map[string][]byte{"value": out}
	return r.Update(ctx, existingSecret)
}

// Ensure GCPKCCManagedControlPlaneReconciler implements reconcile.Reconciler.
var _ reconcile.Reconciler = &GCPKCCManagedControlPlaneReconciler{}
