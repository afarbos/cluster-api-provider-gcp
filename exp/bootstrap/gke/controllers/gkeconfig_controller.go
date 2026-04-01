/*
Copyright 2020 The Kubernetes Authors.

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

// Package controllers provides a way to reconcile GKEConfig objects.
package controllers

import (
	"context"
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	infrav1v2 "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta2"
	bootstrapv1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/bootstrap/gke/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/util/reconciler"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	exputil "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// GKEConfigReconciler reconciles a GKEConfig object.
type GKEConfigReconciler struct {
	client.Client
	WatchFilterValue string
	ReconcileTimeout time.Duration
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=gkeconfigs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=gkeconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;clusters,verbs=get;list;watch

func (r *GKEConfigReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, option controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	b := ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1exp.GKEConfig{}).
		WithOptions(option).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), log, r.WatchFilterValue)).
		Watches(
			&infrav1exp.GCPManagedMachinePool{},
			handler.EnqueueRequestsFromMapFunc(r.infraMachinePoolToGKEConfigMapFunc),
		).
		Watches(
			&infrav1v2.GCPKCCManagedMachinePool{},
			handler.EnqueueRequestsFromMapFunc(r.infraMachinePoolToGKEConfigMapFunc),
		)

	_, err := b.Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}

func (r *GKEConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	config := &bootstrapv1exp.GKEConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get config")
		return ctrl.Result{}, err
	}
	log = log.WithValues("GKEConfig", config.GetName())

	machinePool, err := exputil.GetOwnerMachinePool(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner MachinePool")
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("No owner MachinePool found yet, requeueing")
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	// Fetch the infrastructure machine pool and check readiness.
	infraRef := machinePool.Spec.Template.Spec.InfrastructureRef
	ready, err := getInfraMachinePoolReady(ctx, r.Client, infraRef, machinePool.Namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Infrastructure machine pool not found, requeueing", "kind", infraRef.Kind, "name", infraRef.Name)
			return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
		}
		return ctrl.Result{}, err
	}
	if !ready {
		log.Info("Waiting for infrastructure machine pool to be ready, requeueing", "kind", infraRef.Kind, "name", infraRef.Name)
		return ctrl.Result{RequeueAfter: reconciler.DefaultRetryTime}, nil
	}

	config.Status.Ready = true
	if err := r.Status().Update(ctx, config); err != nil {
		log.Error(err, "Failed to update GKEConfig status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled GKEConfig", "MachinePool", machinePool.GetName())
	return ctrl.Result{}, nil
}

// getInfraMachinePoolReady fetches the infrastructure machine pool and returns
// whether it is ready. Uses typed clients — no CRD RBAC required.
func getInfraMachinePoolReady(ctx context.Context, c client.Client, ref clusterv1.ContractVersionedObjectReference, namespace string) (bool, error) {
	var obj client.Object
	var getReady func() bool
	switch ref.Kind {
	case "GCPKCCManagedMachinePool":
		t := &infrav1v2.GCPKCCManagedMachinePool{}
		obj, getReady = t, func() bool { return t.Status.Ready }
	default:
		t := &infrav1exp.GCPManagedMachinePool{}
		obj, getReady = t, func() bool { return t.Status.Ready }
	}
	if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, obj); err != nil {
		return false, err
	}
	return getReady(), nil
}

// infraMachinePoolToGKEConfigMapFunc maps infrastructure machine pool changes
// to the GKEConfig via the owner MachinePool chain. Works for any infra type.
func (r *GKEConfigReconciler) infraMachinePoolToGKEConfigMapFunc(_ context.Context, o client.Object) []ctrl.Request {
	machinePool, err := exputil.GetOwnerMachinePool(context.Background(), r.Client, metav1.ObjectMeta{
		OwnerReferences: o.GetOwnerReferences(),
		Namespace:       o.GetNamespace(),
	})
	if err != nil {
		klog.Errorf("Failed to get owner MachinePool for %T %s/%s: %v", o, o.GetNamespace(), o.GetName(), err)
		return nil
	}
	if machinePool == nil {
		return nil
	}

	bootstrapRef := machinePool.Spec.Template.Spec.Bootstrap.ConfigRef
	if !bootstrapRef.IsDefined() {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      bootstrapRef.Name,
				Namespace: machinePool.Namespace,
			},
		},
	}
}
