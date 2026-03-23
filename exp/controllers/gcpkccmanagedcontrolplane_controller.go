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

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	"sigs.k8s.io/cluster-api-provider-gcp/feature"
	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/util/reconciler"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
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

var (
	containerClusterGVK = schema.GroupVersionKind{Group: "container.cnrm.cloud.google.com", Version: "v1beta1", Kind: "ContainerCluster"}
)

// GCPKCCManagedControlPlaneReconciler reconciles a GCPKCCManagedControlPlane object.
type GCPKCCManagedControlPlaneReconciler struct {
	client.Client
	WatchFilterValue string
	ReconcileTimeout time.Duration
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedcontrolplanes/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=gcpkccmanagedclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=container.cnrm.cloud.google.com,resources=containerclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

// Reconcile reconciles a GCPKCCManagedControlPlane by managing a Config Connector
// ContainerCluster resource and generating the kubeconfig secret.
func (r *GCPKCCManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(ctx, reconciler.DefaultedLoopTimeout(r.ReconcileTimeout))
	defer cancel()

	log := log.FromContext(ctx)

	// Step 1: Check feature gate.
	if !feature.Gates.Enabled(feature.ConfigConnector) {
		log.V(4).Info("ConfigConnector feature gate is disabled, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Step 2: Fetch the GCPKCCManagedControlPlane.
	kccCP := &infrav1exp.GCPKCCManagedControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, kccCP); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 3: Fetch the owner Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kccCP.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef, requeuing")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// Step 4: Check InfrastructureRef kind — only handle GCPKCCManagedCluster.
	if cluster.Spec.InfrastructureRef.IsDefined() && cluster.Spec.InfrastructureRef.Kind != "GCPKCCManagedCluster" {
		log.Info("Cluster InfrastructureRef is not a GCPKCCManagedCluster, skipping")
		return ctrl.Result{}, nil
	}

	// Step 5: Skip if externally managed.
	if annotations.IsExternallyManaged(cluster) {
		log.Info("Cluster is externally managed, skipping")
		return ctrl.Result{}, nil
	}

	// Step 6: Set up a patcher.
	patchHelper, err := patch.NewHelper(kccCP, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, kccCP); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Always set ExternalManagedControlPlane = true (GKE manages the control plane).
	t := true
	kccCP.Status.ExternalManagedControlPlane = &t

	// Step 7: Handle pause.
	if annotations.IsPaused(cluster, kccCP) {
		log.Info("GCPKCCManagedControlPlane or linked Cluster is paused")
		apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
			Type:               clusterv1.PausedCondition,
			Status:             metav1.ConditionTrue,
			Reason:             clusterv1.PausedReason,
			Message:            "Reconciliation is paused",
			ObservedGeneration: kccCP.Generation,
		})
		return ctrl.Result{}, nil
	}
	apimeta.RemoveStatusCondition(&kccCP.Status.Conditions, clusterv1.PausedCondition)

	// Step 8: Handle deletion.
	if !kccCP.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, kccCP)
	}

	return r.reconcileNormal(ctx, cluster, kccCP)
}

func (r *GCPKCCManagedControlPlaneReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, kccCP *infrav1exp.GCPKCCManagedControlPlane) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcontrolplane")
	log.Info("Reconciling GCPKCCManagedControlPlane")

	// Add finalizer.
	if !controllerutil.ContainsFinalizer(kccCP, infrav1exp.KCCControlPlaneFinalizer) {
		controllerutil.AddFinalizer(kccCP, infrav1exp.KCCControlPlaneFinalizer)
	}

	// Gate on GCPKCCManagedCluster being provisioned.
	infraClusterReady, err := r.isInfraClusterProvisioned(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !infraClusterReady {
		log.Info("Waiting for GCPKCCManagedCluster to be provisioned")
		apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "WaitingForInfrastructure",
			Message:            "Waiting for GCPKCCManagedCluster to be provisioned",
			ObservedGeneration: kccCP.Generation,
		})
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// Reconcile ContainerCluster.
	containerCluster, err := r.reconcileContainerCluster(ctx, cluster, kccCP)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling ContainerCluster: %w", err)
	}

	// Check if ContainerCluster is ready.
	if !isKCCResourceReady(containerCluster) {
		log.Info("ContainerCluster not yet ready, requeuing")
		apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
			Type:               "Available",
			Status:             metav1.ConditionFalse,
			Reason:             "WaitingForContainerCluster",
			Message:            "Waiting for ContainerCluster to be ready",
			ObservedGeneration: kccCP.Generation,
		})
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Extract endpoint from ContainerCluster status.
	endpoint, err := extractClusterEndpoint(containerCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("extracting cluster endpoint: %w", err)
	}

	// Update control plane endpoint.
	kccCP.Spec.ControlPlaneEndpoint = clusterv1beta1.APIEndpoint{
		Host: endpoint,
		Port: 443,
	}

	// Generate kubeconfig secret.
	if err := r.reconcileKubeconfig(ctx, cluster, kccCP, containerCluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling kubeconfig: %w", err)
	}

	// Mark initialized.
	initialized := true
	kccCP.Status.Ready = true
	kccCP.Status.Initialized = true
	if kccCP.Status.Initialization == nil {
		kccCP.Status.Initialization = &infrav1exp.GCPKCCManagedControlPlaneInitializationStatus{}
	}
	kccCP.Status.Initialization.ControlPlaneInitialized = &initialized

	apimeta.SetStatusCondition(&kccCP.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             metav1.ConditionTrue,
		Reason:             "Initialized",
		Message:            "Control plane is initialized",
		ObservedGeneration: kccCP.Generation,
	})

	log.Info("GCPKCCManagedControlPlane is ready")
	return ctrl.Result{}, nil
}

func (r *GCPKCCManagedControlPlaneReconciler) reconcileDelete(ctx context.Context, _ *clusterv1.Cluster, kccCP *infrav1exp.GCPKCCManagedControlPlane) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("controller", "gcpkccmanagedcontrolplane", "action", "delete")
	log.Info("Reconciling delete GCPKCCManagedControlPlane")

	if !controllerutil.ContainsFinalizer(kccCP, infrav1exp.KCCControlPlaneFinalizer) {
		return ctrl.Result{}, nil
	}

	clusterDeleted, err := r.deleteContainerCluster(ctx, kccCP)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !clusterDeleted {
		log.Info("Waiting for ContainerCluster to be deleted")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	controllerutil.RemoveFinalizer(kccCP, infrav1exp.KCCControlPlaneFinalizer)
	return ctrl.Result{}, nil
}

// reconcileContainerCluster creates or retrieves the ContainerCluster KCC resource.
func (r *GCPKCCManagedControlPlaneReconciler) reconcileContainerCluster(ctx context.Context, _ *clusterv1.Cluster, kccCP *infrav1exp.GCPKCCManagedControlPlane) (*unstructured.Unstructured, error) {
	desired, err := rawExtensionToUnstructured(kccCP.Spec.Cluster)
	if err != nil {
		return nil, fmt.Errorf("parsing cluster spec: %w", err)
	}
	desired.SetGroupVersionKind(containerClusterGVK)
	desired.SetNamespace(kccCP.Namespace)
	setKCCOwnerReference(desired, kccCP, "GCPKCCManagedControlPlane")

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(containerClusterGVK)
	err = r.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if apierrors.IsNotFound(err) {
		if createErr := r.Create(ctx, desired); createErr != nil {
			return nil, fmt.Errorf("creating ContainerCluster %s: %w", desired.GetName(), createErr)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting ContainerCluster %s: %w", desired.GetName(), err)
	}
	return existing, nil
}

// reconcileKubeconfig generates or updates the kubeconfig secret for the workload cluster.
// Secret name: <cluster>-kubeconfig, type: cluster.x-k8s.io/secret,
// label: cluster.x-k8s.io/cluster-name=<cluster>, key: value
func (r *GCPKCCManagedControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, cluster *clusterv1.Cluster, kccCP *infrav1exp.GCPKCCManagedControlPlane, containerCluster *unstructured.Unstructured) error {
	log := log.FromContext(ctx)

	// Extract CA cert from ContainerCluster status.masterAuth.clusterCaCertificate (base64-encoded PEM).
	caCertB64, _, _ := unstructured.NestedString(containerCluster.Object, "status", "masterAuth", "clusterCaCertificate")
	if caCertB64 == "" {
		log.Info("ContainerCluster CA cert not yet available, skipping kubeconfig generation")
		return nil
	}
	caCert, err := base64.StdEncoding.DecodeString(caCertB64)
	if err != nil {
		return fmt.Errorf("decoding cluster CA cert: %w", err)
	}

	endpoint := kccCP.Spec.ControlPlaneEndpoint.Host
	if endpoint == "" {
		return nil
	}

	server := fmt.Sprintf("https://%s", endpoint)
	clusterName := cluster.Name

	// Build a kubeconfig using exec credentials with gke-gcloud-auth-plugin.
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[clusterName] = &clientcmdapi.Cluster{
		Server:                   server,
		CertificateAuthorityData: caCert,
	}
	cfg.AuthInfos[clusterName] = &clientcmdapi.AuthInfo{
		Exec: &clientcmdapi.ExecConfig{
			APIVersion: "client.authentication.k8s.io/v1beta1",
			Command:    "gke-gcloud-auth-plugin",
			InstallHint: "Install gke-gcloud-auth-plugin for use with kubectl by following" +
				" https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl#install_plugin",
			ProvideClusterInfo: true,
			InteractiveMode:    clientcmdapi.IfAvailableExecInteractiveMode,
		},
	}
	cfg.Contexts[clusterName] = &clientcmdapi.Context{
		Cluster:  clusterName,
		AuthInfo: clusterName,
	}
	cfg.CurrentContext = clusterName

	kubeconfigBytes, err := k8sruntime.Encode(clientcmdlatest.Codec, cfg)
	if err != nil {
		return fmt.Errorf("encoding kubeconfig: %w", err)
	}

	// Write secret following CAPI conventions.
	secretName := secret.Name(clusterName, secret.Kubeconfig)
	kubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: kccCP.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: clusterName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: infrav1exp.GroupVersion.String(),
					Kind:       "GCPKCCManagedControlPlane",
					Name:       kccCP.Name,
					UID:        kccCP.UID,
				},
			},
		},
		Type: clusterv1.ClusterSecretType,
		Data: map[string][]byte{
			secret.KubeconfigDataName: kubeconfigBytes,
		},
	}

	existing := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: kccCP.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, kubeconfigSecret)
	}
	if err != nil {
		return err
	}

	// Update if data changed.
	existing.Data = kubeconfigSecret.Data
	return r.Update(ctx, existing)
}

// deleteContainerCluster deletes the ContainerCluster and returns true when it is gone.
func (r *GCPKCCManagedControlPlaneReconciler) deleteContainerCluster(ctx context.Context, kccCP *infrav1exp.GCPKCCManagedControlPlane) (bool, error) {
	obj, err := rawExtensionToUnstructured(kccCP.Spec.Cluster)
	if err != nil {
		return false, err
	}
	obj.SetGroupVersionKind(containerClusterGVK)
	obj.SetNamespace(kccCP.Namespace)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(containerClusterGVK)
	err = r.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, existing)
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if existing.GetDeletionTimestamp().IsZero() {
		if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("deleting ContainerCluster %s: %w", existing.GetName(), err)
		}
	}
	return false, nil
}

// isInfraClusterProvisioned returns true if the GCPKCCManagedCluster is provisioned.
func (r *GCPKCCManagedControlPlaneReconciler) isInfraClusterProvisioned(ctx context.Context, cluster *clusterv1.Cluster) (bool, error) {
	if !cluster.Spec.InfrastructureRef.IsDefined() {
		return false, nil
	}
	infraRef := cluster.Spec.InfrastructureRef
	kccCluster := &infrav1exp.GCPKCCManagedCluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: infraRef.Name, Namespace: cluster.Namespace}, kccCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if kccCluster.Status.Initialization == nil || kccCluster.Status.Initialization.Provisioned == nil {
		return false, nil
	}
	return *kccCluster.Status.Initialization.Provisioned, nil
}

// extractClusterEndpoint returns the GKE cluster endpoint from ContainerCluster status.
func extractClusterEndpoint(containerCluster *unstructured.Unstructured) (string, error) {
	endpoint, found, err := unstructured.NestedString(containerCluster.Object, "status", "endpoint")
	if err != nil {
		return "", fmt.Errorf("reading endpoint from ContainerCluster status: %w", err)
	}
	if !found || endpoint == "" {
		return "", fmt.Errorf("ContainerCluster endpoint not yet available")
	}
	return endpoint, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GCPKCCManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	// Verify that KCC CRDs are present.
	if err := verifyKCCCRDs(ctx, mgr.GetClient(), containerClusterGVK); err != nil {
		return fmt.Errorf("KCC CRDs not found — install Config Connector before enabling the ConfigConnector feature gate: %w", err)
	}

	_, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1exp.GCPKCCManagedControlPlane{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}
	return nil
}
