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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	infrav1exp "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-gcp/feature"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newKCCClusterScheme(g *WithT) *k8sruntime.Scheme {
	s := k8sruntime.NewScheme()
	g.Expect(infrav1exp.AddToScheme(s)).To(Succeed())
	g.Expect(clusterv1.AddToScheme(s)).To(Succeed())
	return s
}

func newTestCluster(name, ns string) *clusterv1.Cluster {
	return &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: clusterv1.ContractVersionedObjectReference{
				Kind:     "GCPKCCManagedCluster",
				Name:     name + "-infra",
				APIGroup: infrav1exp.GroupVersion.Group,
			},
		},
	}
}

func newTestKCCCluster(name, ns string, ownerCluster *clusterv1.Cluster) *infrav1exp.GCPKCCManagedCluster {
	kccCluster := &infrav1exp.GCPKCCManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       ownerCluster.Name,
					UID:        ownerCluster.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: infrav1exp.GCPKCCManagedClusterSpec{
			Network:    mustMarshalRaw(map[string]interface{}{"metadata": map[string]interface{}{"name": "my-network"}}),
			Subnetwork: mustMarshalRaw(map[string]interface{}{"metadata": map[string]interface{}{"name": "my-subnet"}}),
		},
	}
	return kccCluster
}

func boolPtr(b bool) *bool { return &b }

// TestGCPKCCManagedClusterReconciler_FeatureGateDisabled verifies the reconciler
// is a no-op when the ConfigConnector feature gate is off.
func TestGCPKCCManagedClusterReconciler_FeatureGateDisabled(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Ensure gate is off for this test.
	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	cluster := newTestCluster("test-cluster", "default")
	kccCluster := newTestKCCCluster("test-cluster-infra", "default", cluster)

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(cluster, kccCluster).WithStatusSubresource(kccCluster).Build()
	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// Finalizer must NOT have been added.
	updated := &infrav1exp.GCPKCCManagedCluster{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}, updated)).To(Succeed())
	g.Expect(updated.Finalizers).To(BeEmpty())
}

// TestGCPKCCManagedClusterReconciler_NotFound verifies the reconciler handles
// a missing resource gracefully.
func TestGCPKCCManagedClusterReconciler_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": true})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
}

// TestGCPKCCManagedClusterReconciler_NoOwnerCluster verifies the reconciler
// requeues when the owner Cluster has not yet set the OwnerRef.
func TestGCPKCCManagedClusterReconciler_NoOwnerCluster(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": true})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	// GCPKCCManagedCluster has no OwnerReferences.
	kccCluster := &infrav1exp.GCPKCCManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"},
		Spec: infrav1exp.GCPKCCManagedClusterSpec{
			Network:    mustMarshalRaw(map[string]interface{}{"metadata": map[string]interface{}{"name": "net"}}),
			Subnetwork: mustMarshalRaw(map[string]interface{}{"metadata": map[string]interface{}{"name": "sub"}}),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(kccCluster).WithStatusSubresource(kccCluster).Build()
	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "orphan", Namespace: "default"}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).NotTo(BeZero())
}

// TestGCPKCCManagedClusterReconciler_NormalReconcile verifies that a normal reconcile
// adds the finalizer and creates ComputeNetwork and ComputeSubnetwork resources.
func TestGCPKCCManagedClusterReconciler_NormalReconcile(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": true})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	cluster := newTestCluster("my-cluster", "default")
	kccCluster := newTestKCCCluster("my-infra", "default", cluster)

	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(cluster, kccCluster).WithStatusSubresource(kccCluster).Build()
	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}

	// First reconcile: should add finalizer, create KCC resources, requeue waiting for readiness.
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())

	// Finalizer should be present.
	updated := &infrav1exp.GCPKCCManagedCluster{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}, updated)).To(Succeed())
	g.Expect(updated.Finalizers).To(ContainElement(infrav1exp.KCCClusterFinalizer))

	// ComputeNetwork should have been created.
	network := &unstructured.Unstructured{}
	network.SetGroupVersionKind(computeNetworkGVK)
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "my-network", Namespace: kccCluster.Namespace}, network)).To(Succeed())

	// ComputeSubnetwork should have been created.
	subnet := &unstructured.Unstructured{}
	subnet.SetGroupVersionKind(computeSubnetworkGVK)
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "my-subnet", Namespace: kccCluster.Namespace}, subnet)).To(Succeed())

	// Status should not yet be ready (KCC resources have no Ready condition).
	g.Expect(updated.Status.Ready).To(BeFalse())
}

// TestGCPKCCManagedClusterReconciler_ReadyOnceKCCResourcesReady verifies that the
// cluster becomes ready once both KCC resources report Ready=True.
func TestGCPKCCManagedClusterReconciler_ReadyOnceKCCResourcesReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": true})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	cluster := newTestCluster("my-cluster", "default")
	kccCluster := newTestKCCCluster("my-infra", "default", cluster)

	// Pre-create KCC resources with Ready=True already set in the object.
	network := kccResourceWithReadyCondition(computeNetworkGVK, "my-network", "default")
	subnet := kccResourceWithReadyCondition(computeSubnetworkGVK, "my-subnet", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cluster, kccCluster).
		WithStatusSubresource(kccCluster).
		Build()

	// Create KCC resources — status is embedded in the object directly (no subresource).
	g.Expect(fakeClient.Create(ctx, network)).To(Succeed())
	g.Expect(fakeClient.Create(ctx, subnet)).To(Succeed())
	// Update with Ready=True status directly on the objects.
	g.Expect(setUnstructuredReadyStatus(ctx, fakeClient, network)).To(Succeed())
	g.Expect(setUnstructuredReadyStatus(ctx, fakeClient, subnet)).To(Succeed())

	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeZero(), "should not requeue once ready")

	updated := &infrav1exp.GCPKCCManagedCluster{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}, updated)).To(Succeed())
	g.Expect(updated.Status.Ready).To(BeTrue())
	g.Expect(updated.Status.Initialization).NotTo(BeNil())
	g.Expect(*updated.Status.Initialization.Provisioned).To(BeTrue())
	g.Expect(updated.Status.NetworkName).To(Equal("my-network"))
	g.Expect(updated.Status.SubnetworkName).To(Equal("my-subnet"))
}

// TestGCPKCCManagedClusterReconciler_DeleteWaitsForKCCResources verifies that
// deletion waits for KCC resources to be gone before removing the finalizer.
func TestGCPKCCManagedClusterReconciler_DeleteWaitsForKCCResources(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": true})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	cluster := newTestCluster("my-cluster", "default")
	kccCluster := newTestKCCCluster("my-infra", "default", cluster)
	kccCluster.Finalizers = []string{infrav1exp.KCCClusterFinalizer}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cluster, kccCluster).
		WithStatusSubresource(kccCluster).
		Build()

	// Issue a Delete to set DeletionTimestamp (fake client sets it when finalizers exist).
	g.Expect(fakeClient.Delete(ctx, kccCluster)).To(Succeed())

	// Pre-create a ComputeNetwork that still exists (not yet deleted by KCC).
	network := kccResourceWithReadyCondition(computeNetworkGVK, "my-network", "default")
	g.Expect(fakeClient.Create(ctx, network)).To(Succeed())

	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).NotTo(BeZero(), "should requeue while KCC resources exist")

	// Finalizer must still be present.
	updated := &infrav1exp.GCPKCCManagedCluster{}
	g.Expect(fakeClient.Get(ctx, types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}, updated)).To(Succeed())
	g.Expect(updated.Finalizers).To(ContainElement(infrav1exp.KCCClusterFinalizer))
}

// TestGCPKCCManagedClusterReconciler_DeleteRemovesFinalizer verifies that the
// finalizer is removed once all KCC resources are gone.
func TestGCPKCCManagedClusterReconciler_DeleteRemovesFinalizer(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	g.Expect(feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": true})).To(Succeed())
	t.Cleanup(func() {
		_ = feature.MutableGates.SetFromMap(map[string]bool{"ConfigConnector": false})
	})

	s := newKCCClusterScheme(g)
	cluster := newTestCluster("my-cluster", "default")
	kccCluster := newTestKCCCluster("my-infra", "default", cluster)
	kccCluster.Finalizers = []string{infrav1exp.KCCClusterFinalizer}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(cluster, kccCluster).
		WithStatusSubresource(kccCluster).
		Build()

	// Issue a Delete to set DeletionTimestamp; no KCC resources exist so deletion completes.
	g.Expect(fakeClient.Delete(ctx, kccCluster)).To(Succeed())

	r := &GCPKCCManagedClusterReconciler{Client: fakeClient}
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result.RequeueAfter).To(BeZero())

	// The fake client removes objects once their last finalizer is cleared and DeletionTimestamp is set.
	updated := &infrav1exp.GCPKCCManagedCluster{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: kccCluster.Name, Namespace: kccCluster.Namespace}, updated)
	g.Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue(), "expected object to be gone or have no finalizer")
	if err == nil {
		g.Expect(updated.Finalizers).NotTo(ContainElement(infrav1exp.KCCClusterFinalizer))
	}
}

// --- test helpers ---

// kccResourceWithReadyCondition creates an unstructured KCC resource with a Ready=True condition.
func kccResourceWithReadyCondition(gvk schema.GroupVersionKind, name, ns string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(ns)
	return obj
}

// setUnstructuredReadyStatus sets Ready=True on an unstructured object using a direct Update.
// The fake client does not enforce status-subresource isolation for unstructured types.
func setUnstructuredReadyStatus(ctx context.Context, c client.Client, obj *unstructured.Unstructured) error {
	updated := obj.DeepCopy()
	if err := unstructured.SetNestedSlice(updated.Object, []interface{}{kccCondition("Ready", "True")}, "status", "conditions"); err != nil {
		return err
	}
	return c.Update(ctx, updated)
}
