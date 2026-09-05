package controllers

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	streambusv1alpha1 "github.com/gstreamio/streambus/deploy/kubernetes/operator/api/v1alpha1"
)

// newTestScheme registers every type the reconciler touches so the fake
// client can encode/decode them.
func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding corev1 to scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding appsv1 to scheme: %v", err)
	}
	if err := streambusv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("adding streambusv1alpha1 to scheme: %v", err)
	}
	return scheme
}

// newTestCluster returns a StreamBusCluster with every field the reconciler
// dereferences unconditionally (storage sizes must be valid resource.Quantity
// strings or buildStatefulSet's resource.MustParse panics).
func newTestCluster(name, namespace string) *streambusv1alpha1.StreamBusCluster {
	return &streambusv1alpha1.StreamBusCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(name + "-uid"),
		},
		Spec: streambusv1alpha1.StreamBusClusterSpec{
			Replicas: 3,
			Image: streambusv1alpha1.ImageSpec{
				Repository: "streambus/broker",
				Tag:        "v1",
			},
			Storage: streambusv1alpha1.StorageSpec{
				Class:    "standard",
				Size:     "10Gi",
				RaftSize: "5Gi",
			},
			Config: streambusv1alpha1.ConfigSpec{
				LogLevel: "info",
				Port:     9092,
				HTTPPort: 8081,
				GRPCPort: 9093,
			},
		},
	}
}

// newReconciler builds a StreamBusClusterReconciler backed by a fake client
// seeded with objs. The status subresource must be declared explicitly for
// custom resources (built-in types like StatefulSet get it automatically).
func newReconciler(t *testing.T, objs ...client.Object) (*StreamBusClusterReconciler, client.Client) {
	t.Helper()
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&streambusv1alpha1.StreamBusCluster{}).
		Build()
	return &StreamBusClusterReconciler{Client: c, Scheme: scheme}, c
}

func reconcileRequest(cluster *streambusv1alpha1.StreamBusCluster) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}}
}

// assertOwnedBy fails the test unless obj carries a controller owner
// reference back to owner. Without this, deleting the StreamBusCluster
// orphans everything it created instead of letting GC cascade the delete.
func assertOwnedBy(t *testing.T, obj metav1.Object, owner *streambusv1alpha1.StreamBusCluster) {
	t.Helper()
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == owner.UID && ref.Controller != nil && *ref.Controller {
			return
		}
	}
	t.Errorf("expected a controller owner reference to %s/%s, got %+v", owner.Namespace, owner.Name, obj.GetOwnerReferences())
}

// A StreamBusCluster can be deleted out from under a stale watch event before
// Reconcile runs. That must resolve quietly, not as an error that gets
// requeued forever.
func TestReconcile_ClusterNotFound(t *testing.T) {
	r, _ := newReconciler(t)

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("Reconcile returned error for a resource deleted mid-flight: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("expected an empty result for a not-found resource, got %+v", res)
	}
}

func TestReconcile_CreatesOwnedResourcesWithOwnerReferences(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, reconcileRequest(cluster)); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	var sts appsv1.StatefulSet
	if err := c.Get(ctx, nn, &sts); err != nil {
		t.Fatalf("expected StatefulSet to be created: %v", err)
	}
	assertOwnedBy(t, &sts, cluster)
	if *sts.Spec.Replicas != cluster.Spec.Replicas {
		t.Errorf("expected StatefulSet replicas %d, got %d", cluster.Spec.Replicas, *sts.Spec.Replicas)
	}

	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Name: cluster.Name + "-config", Namespace: cluster.Namespace}, &cm); err != nil {
		t.Fatalf("expected ConfigMap to be created: %v", err)
	}
	assertOwnedBy(t, &cm, cluster)

	var headlessSvc corev1.Service
	if err := c.Get(ctx, types.NamespacedName{Name: cluster.Name + "-headless", Namespace: cluster.Namespace}, &headlessSvc); err != nil {
		t.Fatalf("expected headless Service to be created: %v", err)
	}
	assertOwnedBy(t, &headlessSvc, cluster)

	var clientSvc corev1.Service
	if err := c.Get(ctx, nn, &clientSvc); err != nil {
		t.Fatalf("expected client Service to be created: %v", err)
	}
	assertOwnedBy(t, &clientSvc, cluster)

	var got streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &got); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if !controllerutil.ContainsFinalizer(&got, finalizerName) {
		t.Error("expected the finalizer to be added on first reconcile")
	}
	if got.Status.Phase != streambusv1alpha1.ClusterPhaseCreating {
		t.Errorf("expected phase Creating for a brand-new StatefulSet with no ready replicas, got %q", got.Status.Phase)
	}
}

// The single most valuable operator test: reconciling twice with nothing
// changed must not rewrite the owned resources. An unconditional Update
// would bump resourceVersion every loop and could thrash a real rollout.
func TestReconcile_IdempotentOnRepeatedReconcile(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	headlessNN := types.NamespacedName{Name: cluster.Name + "-headless", Namespace: cluster.Namespace}
	configMapNN := types.NamespacedName{Name: cluster.Name + "-config", Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	rvBefore := ownedResourceVersions(t, ctx, c, nn, headlessNN, configMapNN)

	// Reconcile several more times: a diff that's just slightly too eager
	// (e.g. missing the container-port Protocol default, or comparing a
	// field the API server adds on its own) can still look stable after one
	// extra pass and only start oscillating on the third or later loop.
	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile #%d failed: %v", i+2, err)
		}
		rvAfter := ownedResourceVersions(t, ctx, c, nn, headlessNN, configMapNN)
		if rvAfter != rvBefore {
			t.Fatalf("owned resources were rewritten on a no-op reconcile #%d: %+v -> %+v", i+2, rvBefore, rvAfter)
		}
	}

	var svcList corev1.ServiceList
	if err := c.List(ctx, &svcList, client.InNamespace(cluster.Namespace)); err != nil {
		t.Fatalf("list services: %v", err)
	}
	if len(svcList.Items) != 2 {
		t.Errorf("expected exactly 2 services (client + headless) after repeated reconciles, got %d", len(svcList.Items))
	}

	var cmList corev1.ConfigMapList
	if err := c.List(ctx, &cmList, client.InNamespace(cluster.Namespace)); err != nil {
		t.Fatalf("list configmaps: %v", err)
	}
	if len(cmList.Items) != 1 {
		t.Errorf("expected exactly 1 ConfigMap after repeated reconciles, got %d", len(cmList.Items))
	}
}

// resourceVersions is a snapshot of the owned resources' resourceVersion
// fields, used to detect a no-op reconcile that rewrites something anyway.
type resourceVersions struct {
	statefulSet, headlessService, configMap string
}

func ownedResourceVersions(t *testing.T, ctx context.Context, c client.Client, statefulSetNN, headlessSvcNN, configMapNN types.NamespacedName) resourceVersions {
	t.Helper()

	var sts appsv1.StatefulSet
	if err := c.Get(ctx, statefulSetNN, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	var svc corev1.Service
	if err := c.Get(ctx, headlessSvcNN, &svc); err != nil {
		t.Fatalf("get headless service: %v", err)
	}
	var cm corev1.ConfigMap
	if err := c.Get(ctx, configMapNN, &cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}

	return resourceVersions{
		statefulSet:     sts.ResourceVersion,
		headlessService: svc.ResourceVersion,
		configMap:       cm.ResourceVersion,
	}
}

func TestReconcile_ReplicaChangePropagatesToStatefulSet(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var current streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &current); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	current.Spec.Replicas = 5
	if err := c.Update(ctx, &current); err != nil {
		t.Fatalf("update cluster spec: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var sts appsv1.StatefulSet
	if err := c.Get(ctx, nn, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 5 {
		t.Errorf("expected StatefulSet replicas to be updated to 5, got %v", sts.Spec.Replicas)
	}
}

// reconcileConfigMap is the one owned-resource reconciler that does compare
// its full desired state against what's live, so a config change should
// actually land. This is the positive contrast to the gaps documented below.
func TestReconcile_ConfigMapUpdatesWhenConfigChanges(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var current streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &current); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	current.Spec.Config.LogLevel = "debug"
	if err := c.Update(ctx, &current); err != nil {
		t.Fatalf("update cluster spec: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Name: cluster.Name + "-config", Namespace: cluster.Namespace}, &cm); err != nil {
		t.Fatalf("get configmap: %v", err)
	}
	if !contains(cm.Data["broker.yaml"], "log_level: debug") {
		t.Errorf("expected ConfigMap to reflect the updated log level, got:\n%s", cm.Data["broker.yaml"])
	}
}

func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Status must reflect the StatefulSet's real, controller-reported readiness
// rather than being written optimistically. Before any pods are ready the
// phase must be Creating; only once the StatefulSet's own status subresource
// (simulating the kubelet/StatefulSet controller) reports ready replicas
// should the operator report Running.
func TestReconcile_StatusReflectsStatefulSetReadiness(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var afterCreate streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &afterCreate); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if afterCreate.Status.Phase != streambusv1alpha1.ClusterPhaseCreating {
		t.Fatalf("expected phase Creating before any pods are ready, got %q", afterCreate.Status.Phase)
	}

	var sts appsv1.StatefulSet
	if err := c.Get(ctx, nn, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	sts.Status.Replicas = cluster.Spec.Replicas
	sts.Status.ReadyReplicas = cluster.Spec.Replicas
	if err := c.Status().Update(ctx, &sts); err != nil {
		t.Fatalf("update statefulset status: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var afterReady streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &afterReady); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	if afterReady.Status.Phase != streambusv1alpha1.ClusterPhaseRunning {
		t.Errorf("expected phase Running once the StatefulSet reports ready replicas, got %q", afterReady.Status.Phase)
	}
	if afterReady.Status.ReadyReplicas != cluster.Spec.Replicas {
		t.Errorf("expected status.ReadyReplicas to mirror the StatefulSet's status, got %d", afterReady.Status.ReadyReplicas)
	}
}

// Deleting a cluster that still carries the finalizer must not error, and
// must actually clear the finalizer so Kubernetes can complete the delete.
func TestReconcile_DeletionRemovesFinalizerAndDoesNotError(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	cluster.Finalizers = []string{finalizerName}
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	// The fake tracker mirrors real API server behavior here: deleting an
	// object with finalizers present only sets DeletionTimestamp; the object
	// is only actually removed once a later Update clears the last finalizer.
	if err := c.Delete(ctx, cluster); err != nil {
		t.Fatalf("delete cluster: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile during deletion should not error: %v", err)
	}

	// Removing the last finalizer while DeletionTimestamp is set is exactly
	// what tells the API server (and this fake) to finish the delete, so the
	// object should now be gone rather than lingering with an empty list.
	var got streambusv1alpha1.StreamBusCluster
	err := c.Get(ctx, nn, &got)
	if err == nil {
		t.Fatalf("expected cluster to be fully deleted once the finalizer cleared, but it still exists: %+v", got)
	}
	if !apierrors.IsNotFound(err) {
		t.Fatalf("get cluster: %v", err)
	}
}

// --- Regression tests for gaps found during the earlier audit, now fixed ---
//
// reconcileHeadlessService, reconcileClientService and reconcileStatefulSet
// used to only check for the resource's *existence* (reconcileStatefulSet
// also compared Replicas); no other drift between spec and the live object
// was detected, so Reconcile returned nil (success) even though the owned
// resource never converged to the new spec. Both reconcilers now diff and
// update the fields they own.

func TestReconcile_ImageChangePropagatesToStatefulSet(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var current streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &current); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	current.Spec.Image.Tag = "v2"
	if err := c.Update(ctx, &current); err != nil {
		t.Fatalf("update cluster spec: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var sts appsv1.StatefulSet
	if err := c.Get(ctx, nn, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	wantImage := "streambus/broker:v2"
	if gotImage := sts.Spec.Template.Spec.Containers[0].Image; gotImage != wantImage {
		t.Errorf("expected StatefulSet image to be updated to %q, got %q", wantImage, gotImage)
	}
}

func TestReconcile_ServicePortChangePropagatesToService(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var current streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &current); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	current.Spec.Config.Port = 19092
	if err := c.Update(ctx, &current); err != nil {
		t.Fatalf("update cluster spec: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var svc corev1.Service
	if err := c.Get(ctx, nn, &svc); err != nil {
		t.Fatalf("get service: %v", err)
	}
	if svc.Spec.Ports[0].Port != 19092 {
		t.Errorf("expected client Service broker port to be updated to 19092, got %d", svc.Spec.Ports[0].Port)
	}
}

// A storage size/class change requires rewriting volumeClaimTemplates, which
// the Kubernetes API server rejects on Update. Reconcile must surface that
// as an error instead of silently leaving the StatefulSet on its original
// storage while still reporting success.
func TestReconcile_ImmutableStorageChangeReturnsError(t *testing.T) {
	cluster := newTestCluster("demo", "default")
	r, c := newReconciler(t, cluster)
	ctx := context.Background()
	req := reconcileRequest(cluster)
	nn := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	var current streambusv1alpha1.StreamBusCluster
	if err := c.Get(ctx, nn, &current); err != nil {
		t.Fatalf("get cluster: %v", err)
	}
	current.Spec.Storage.Size = "20Gi"
	if err := c.Update(ctx, &current); err != nil {
		t.Fatalf("update cluster spec: %v", err)
	}

	if _, err := r.Reconcile(ctx, req); err == nil {
		t.Fatal("expected Reconcile to return an error when the desired spec requires an immutable StatefulSet field to change")
	}

	// The StatefulSet must be left exactly as it was — no partial or
	// rejected write should reach the live object.
	var sts appsv1.StatefulSet
	if err := c.Get(ctx, nn, &sts); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	gotSize := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
	wantSize := resource.MustParse("10Gi")
	if gotSize.Cmp(wantSize) != 0 {
		t.Errorf("expected StatefulSet volume claim template to remain at 10Gi, got %s", gotSize.String())
	}
}
