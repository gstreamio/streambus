package controllers

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	streambusv1alpha1 "github.com/gstreamio/streambus/deploy/kubernetes/operator/api/v1alpha1"
)

// metricsCluster builds a CR with spec.observability.metrics.serviceMonitor
// requested, so each test can adjust only the part it cares about.
func metricsCluster(mutate func(*streambusv1alpha1.StreamBusCluster)) *streambusv1alpha1.StreamBusCluster {
	cluster := newTestCluster("demo", "default")
	cluster.Spec.Observability.Metrics = streambusv1alpha1.MetricsSpec{
		Enabled:        true,
		ServiceMonitor: true,
	}
	if mutate != nil {
		mutate(cluster)
	}
	return cluster
}

func getServiceMonitor(t *testing.T, ctx context.Context, r *StreamBusClusterReconciler, nn types.NamespacedName) *unstructured.Unstructured {
	t.Helper()
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(serviceMonitorGVK)
	if err := r.Get(ctx, nn, sm); err != nil {
		t.Fatalf("get ServiceMonitor %s: %v", nn, err)
	}
	return sm
}

// TestReconcile_CreatesServiceMonitorWhenRequested guards the defect this
// change fixes: spec.observability.metrics was accepted by the API server,
// validated, and then read by nothing - no ServiceMonitor was ever created
// no matter what a user asked for.
func TestReconcile_CreatesServiceMonitorWhenRequested(t *testing.T) {
	cluster := metricsCluster(nil)
	r, _ := newReconciler(t, cluster)
	ctx := context.Background()
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, reconcileRequest(cluster)); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	sm := getServiceMonitor(t, ctx, r, nn)
	assertOwnedBy(t, sm, cluster)

	selector, found, err := unstructured.NestedStringMap(sm.Object, "spec", "selector", "matchLabels")
	if err != nil || !found {
		t.Fatalf("spec.selector.matchLabels not found or wrong type: found=%v err=%v", found, err)
	}
	want := r.labelsForCluster(cluster)
	for k, v := range want {
		if selector[k] != v {
			t.Errorf("selector.matchLabels[%q] = %q, want %q", k, selector[k], v)
		}
	}

	endpoints, found, err := unstructured.NestedSlice(sm.Object, "spec", "endpoints")
	if err != nil || !found || len(endpoints) == 0 {
		t.Fatalf("spec.endpoints missing or empty: found=%v err=%v endpoints=%v", found, err, endpoints)
	}
}

// TestReconcile_NoServiceMonitorWhenNotRequested is the negative case: metrics
// enabled without asking for a ServiceMonitor must not create one.
func TestReconcile_NoServiceMonitorWhenNotRequested(t *testing.T) {
	cluster := metricsCluster(func(c *streambusv1alpha1.StreamBusCluster) {
		c.Spec.Observability.Metrics.ServiceMonitor = false
	})
	r, _ := newReconciler(t, cluster)
	ctx := context.Background()
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, reconcileRequest(cluster)); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(serviceMonitorGVK)
	if err := r.Get(ctx, nn, sm); err == nil {
		t.Fatalf("expected no ServiceMonitor to be created when serviceMonitor is not requested, got %+v", sm.Object)
	}
}

// TestReconcile_ServiceMonitorIdempotent guards the same failure mode
// TestReconcile_IdempotentOnRepeatedReconcile guards for the other owned
// resources: a no-op reconcile must not rewrite the ServiceMonitor either.
func TestReconcile_ServiceMonitorIdempotent(t *testing.T) {
	cluster := metricsCluster(nil)
	r, _ := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}
	rvBefore := getServiceMonitor(t, ctx, r, nn).GetResourceVersion()

	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile #%d failed: %v", i+2, err)
		}
		if rvAfter := getServiceMonitor(t, ctx, r, nn).GetResourceVersion(); rvAfter != rvBefore {
			t.Fatalf("ServiceMonitor was rewritten on a no-op reconcile #%d: %s -> %s", i+2, rvBefore, rvAfter)
		}
	}
}

// noMatchForKindClient wraps a client.Client and makes Get/Create fail with
// the same meta.NoKindMatchError a real RESTMapper returns for a GVK whose
// CRD is not installed in the cluster. The fake client has no RESTMapper of
// its own, so this is how the "CRD absent" path gets exercised at all.
type noMatchForKindClient struct {
	client.Client
	gvk schema.GroupVersionKind
}

func (n *noMatchForKindClient) noMatchErr() error {
	return &meta.NoKindMatchError{GroupKind: n.gvk.GroupKind(), SearchedVersions: []string{n.gvk.Version}}
}

func (n *noMatchForKindClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if obj.GetObjectKind().GroupVersionKind() == n.gvk {
		return n.noMatchErr()
	}
	return n.Client.Get(ctx, key, obj, opts...)
}

func (n *noMatchForKindClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if obj.GetObjectKind().GroupVersionKind() == n.gvk {
		return n.noMatchErr()
	}
	return n.Client.Create(ctx, obj, opts...)
}

// TestReconcile_ServiceMonitorMissingCRDSurfacesError is the constraint that
// matters most here: if the Prometheus Operator CRD is not installed, the API
// server (a real RESTMapper) returns "no matches for kind" for the
// ServiceMonitor GVK. That must reach the caller as an error - never be
// swallowed - or a user who asked for a ServiceMonitor gets no monitoring and
// no indication why.
func TestReconcile_ServiceMonitorMissingCRDSurfacesError(t *testing.T) {
	cluster := metricsCluster(nil)
	r, _ := newReconciler(t, cluster)
	r.Client = &noMatchForKindClient{Client: r.Client, gvk: serviceMonitorGVK}

	err := r.reconcileServiceMonitor(context.Background(), cluster)
	if err == nil {
		t.Fatal("expected reconcileServiceMonitor to return an error when the ServiceMonitor CRD is not installed")
	}
}

// TestReconcile_ServiceMonitorMissingCRDFailsWholeReconcile checks the failure
// reaches the top-level Reconcile loop too, not just the helper - a swallowed
// error here would leave Reconcile reporting success while quietly never
// creating the ServiceMonitor the user asked for.
func TestReconcile_ServiceMonitorMissingCRDFailsWholeReconcile(t *testing.T) {
	cluster := metricsCluster(nil)
	r, _ := newReconciler(t, cluster)
	r.Client = &noMatchForKindClient{Client: r.Client, gvk: serviceMonitorGVK}

	if _, err := r.Reconcile(context.Background(), reconcileRequest(cluster)); err == nil {
		t.Fatal("expected Reconcile to return an error when the ServiceMonitor CRD is not installed")
	}
}
