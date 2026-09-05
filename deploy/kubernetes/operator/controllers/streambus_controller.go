package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	streambusv1alpha1 "github.com/gstreamio/streambus/deploy/kubernetes/operator/api/v1alpha1"
)

const (
	finalizerName = "streambus.io/finalizer"
)

const (
	// raftPort is the port brokers use to reach each other for Raft. It is
	// also published as a container port, and the peer list in the ConfigMap
	// must agree with it - a broker binds its Raft transport to whatever
	// address the peer list gives for its own id.
	raftPort = 7000

	// configMountPath is where the broker's ConfigMap is mounted. The image's
	// default command points at /config/broker.yaml, which is not where this
	// operator mounts anything, so the container command below passes an
	// explicit --config rather than relying on that default.
	configMountPath = "/etc/streambus"

	// tlsMountPath and saslMountPath are where spec.security's Secrets are
	// mounted. The broker reads certificates from the first and one file per
	// user from the second.
	tlsMountPath  = "/etc/streambus/tls"
	saslMountPath = "/etc/streambus/sasl"

	// brokerBinary is the image's entrypoint. The command below wraps it in a
	// shell to derive the per-pod broker id, so it has to name the binary.
	brokerBinary = "/app/streambus-broker"
)

// StreamBusClusterReconciler reconciles a StreamBusCluster object
type StreamBusClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=streambus.io,resources=streambusclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streambus.io,resources=streambusclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=streambus.io,resources=streambusclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch

// Reconcile is the main reconciliation loop
func (r *StreamBusClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the StreamBusCluster instance
	cluster := &streambusv1alpha1.StreamBusCluster{}
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return without error
			logger.Info("StreamBusCluster resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get StreamBusCluster")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if cluster.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, cluster)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(cluster, finalizerName) {
		controllerutil.AddFinalizer(cluster, finalizerName)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, cluster); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile headless service
	if err := r.reconcileHeadlessService(ctx, cluster); err != nil {
		logger.Error(err, "Failed to reconcile headless service")
		return ctrl.Result{}, err
	}

	// Reconcile client service
	if err := r.reconcileClientService(ctx, cluster); err != nil {
		logger.Error(err, "Failed to reconcile client service")
		return ctrl.Result{}, err
	}

	// Reconcile StatefulSet
	if err := r.reconcileStatefulSet(ctx, cluster); err != nil {
		logger.Error(err, "Failed to reconcile StatefulSet")
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.updateStatus(ctx, cluster); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue after 30 seconds to check status
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// handleDeletion handles deletion of the cluster
func (r *StreamBusClusterReconciler) handleDeletion(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(cluster, finalizerName) {
		// Perform cleanup
		logger.Info("Performing cleanup before deletion")

		// Remove finalizer
		controllerutil.RemoveFinalizer(cluster, finalizerName)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// reconcileConfigMap creates or updates the configuration ConfigMap
func (r *StreamBusClusterReconciler) reconcileConfigMap(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-config",
			Namespace: cluster.Namespace,
			Labels:    r.labelsForCluster(cluster),
		},
		Data: r.generateConfig(cluster),
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(cluster, configMap, r.Scheme); err != nil {
		return err
	}

	// Check if ConfigMap exists
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// Update if needed
	if !equality.Semantic.DeepEqual(found.Data, configMap.Data) {
		found.Data = configMap.Data
		return r.Update(ctx, found)
	}

	return nil
}

// reconcileHeadlessService creates or updates the headless service for StatefulSet
func (r *StreamBusClusterReconciler) reconcileHeadlessService(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-headless",
			Namespace: cluster.Namespace,
			Labels:    r.labelsForCluster(cluster),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  r.labelsForCluster(cluster),
			Ports: []corev1.ServicePort{
				{
					Name:       "broker",
					Port:       cluster.Spec.Config.Port,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("broker"),
				},
				{
					Name:       "raft",
					Port:       7000,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("raft"),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cluster, service, r.Scheme); err != nil {
		return err
	}

	return r.reconcileService(ctx, service)
}

// reconcileClientService creates or updates the client service
func (r *StreamBusClusterReconciler) reconcileClientService(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Labels:    r.labelsForCluster(cluster),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.labelsForCluster(cluster),
			Ports: []corev1.ServicePort{
				{
					Name:       "broker",
					Port:       cluster.Spec.Config.Port,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("broker"),
				},
				{
					Name:       "http",
					Port:       cluster.Spec.Config.HTTPPort,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cluster, service, r.Scheme); err != nil {
		return err
	}

	return r.reconcileService(ctx, service)
}

// reconcileService creates the desired Service if it doesn't exist yet, or
// updates just the fields this operator owns (labels, selector, ports) on
// the live object when they've drifted. It never submits desired wholesale:
// ClusterIP and other fields are assigned by the API server and immutable
// once set, so overwriting the object outright would blank them out (and,
// for ClusterIP, be rejected).
func (r *StreamBusClusterReconciler) reconcileService(ctx context.Context, desired *corev1.Service) error {
	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	if serviceNeedsUpdate(found, desired) {
		found.Labels = desired.Labels
		found.Spec.Selector = desired.Spec.Selector
		found.Spec.Ports = desired.Spec.Ports
		return r.Update(ctx, found)
	}

	return nil
}

// serviceNeedsUpdate reports whether the fields reconcileService owns have
// drifted between the live and desired Service.
func serviceNeedsUpdate(found, desired *corev1.Service) bool {
	return !equality.Semantic.DeepEqual(found.Labels, desired.Labels) ||
		!equality.Semantic.DeepEqual(found.Spec.Selector, desired.Spec.Selector) ||
		!equality.Semantic.DeepEqual(found.Spec.Ports, desired.Spec.Ports)
}

// reconcileStatefulSet creates or updates the StatefulSet
func (r *StreamBusClusterReconciler) reconcileStatefulSet(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) error {
	desired := r.buildStatefulSet(cluster)

	if err := controllerutil.SetControllerReference(cluster, desired, r.Scheme); err != nil {
		return err
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.Create(ctx, desired)
		}
		return err
	}

	if err := checkStatefulSetImmutableFields(found, desired); err != nil {
		return err
	}

	if statefulSetNeedsUpdate(found, desired) {
		applyStatefulSetUpdate(found, desired)
		return r.Update(ctx, found)
	}

	return nil
}

// checkStatefulSetImmutableFields reports an error if converging to desired
// would require changing a field the Kubernetes API server rejects on
// Update (spec.serviceName, spec.selector, spec.volumeClaimTemplates).
// Silently ignoring such drift would leave the cluster permanently out of
// sync with its spec while Reconcile kept reporting success — the same
// failure mode as never diffing at all, just one step further along.
func checkStatefulSetImmutableFields(found, desired *appsv1.StatefulSet) error {
	if found.Spec.ServiceName != desired.Spec.ServiceName {
		return fmt.Errorf("cannot converge StatefulSet %s/%s: spec.serviceName is immutable", found.Namespace, found.Name)
	}
	if !equality.Semantic.DeepEqual(found.Spec.Selector, desired.Spec.Selector) {
		return fmt.Errorf("cannot converge StatefulSet %s/%s: spec.selector is immutable", found.Namespace, found.Name)
	}
	if !equality.Semantic.DeepEqual(found.Spec.VolumeClaimTemplates, desired.Spec.VolumeClaimTemplates) {
		return fmt.Errorf("cannot converge StatefulSet %s/%s: spec.volumeClaimTemplates is immutable, "+
			"storage class/size cannot be changed after creation", found.Namespace, found.Name)
	}
	return nil
}

// statefulSetNeedsUpdate reports whether any field this operator keeps in
// sync — replicas, the broker image, resources, container ports, and the
// pod template's annotations/labels — has drifted from the live object.
func statefulSetNeedsUpdate(found, desired *appsv1.StatefulSet) bool {
	if *found.Spec.Replicas != *desired.Spec.Replicas {
		return true
	}
	if !equality.Semantic.DeepEqual(found.Spec.Template.Annotations, desired.Spec.Template.Annotations) {
		return true
	}
	if !equality.Semantic.DeepEqual(found.Spec.Template.Labels, desired.Spec.Template.Labels) {
		return true
	}

	foundContainer := found.Spec.Template.Spec.Containers[0]
	desiredContainer := desired.Spec.Template.Spec.Containers[0]
	if foundContainer.Image != desiredContainer.Image {
		return true
	}
	if !equality.Semantic.DeepEqual(foundContainer.Resources, desiredContainer.Resources) {
		return true
	}
	return !equality.Semantic.DeepEqual(foundContainer.Ports, desiredContainer.Ports)
}

// applyStatefulSetUpdate copies the operator-owned fields from desired onto
// found. It deliberately does not replace found.Spec.Template.Spec wholesale:
// the API server can set fields this operator never does (e.g. a default
// ServiceAccountName), and overwriting them every reconcile would make the
// StatefulSet look perpetually out of sync, thrashing on every 30s loop.
func applyStatefulSetUpdate(found, desired *appsv1.StatefulSet) {
	found.Spec.Replicas = desired.Spec.Replicas
	found.Spec.Template.Annotations = desired.Spec.Template.Annotations
	found.Spec.Template.Labels = desired.Spec.Template.Labels
	found.Spec.Template.Spec.Containers[0].Image = desired.Spec.Template.Spec.Containers[0].Image
	found.Spec.Template.Spec.Containers[0].Resources = desired.Spec.Template.Spec.Containers[0].Resources
	found.Spec.Template.Spec.Containers[0].Ports = desired.Spec.Template.Spec.Containers[0].Ports
}

// buildStatefulSet builds the StatefulSet specification
func (r *StreamBusClusterReconciler) buildStatefulSet(cluster *streambusv1alpha1.StreamBusCluster) *appsv1.StatefulSet {
	labels := r.labelsForCluster(cluster)

	// Merge user-provided pod labels
	for k, v := range cluster.Spec.PodLabels {
		labels[k] = v
	}

	replicas := cluster.Spec.Replicas

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: cluster.Name + "-headless",
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: r.labelsForCluster(cluster),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: cluster.Spec.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "streambus",
							Image:           r.getImage(cluster),
							ImagePullPolicy: cluster.Spec.Image.PullPolicy,
							// Protocol is set explicitly (rather than left for the API
							// server to default to TCP) so a fetched StatefulSet compares
							// equal to a freshly built one in statefulSetNeedsUpdate instead
							// of looking like permanent drift.
							Ports: []corev1.ContainerPort{
								{
									Name:          "broker",
									ContainerPort: cluster.Spec.Config.Port,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "http",
									ContainerPort: cluster.Spec.Config.HTTPPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "grpc",
									ContainerPort: cluster.Spec.Config.GRPCPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "raft",
									ContainerPort: 7000,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Command:      buildCommand(),
							Env:          r.buildEnv(cluster),
							VolumeMounts: buildVolumeMounts(cluster),
							Resources:    cluster.Spec.Resources,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health/ready",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
						},
					},
					Affinity:     cluster.Spec.Affinity,
					Tolerations:  cluster.Spec.Tolerations,
					NodeSelector: cluster.Spec.NodeSelector,
					Volumes:      buildVolumes(cluster),
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: &cluster.Spec.Storage.Class,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(cluster.Spec.Storage.Size),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "raft-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: &cluster.Spec.Storage.Class,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(cluster.Spec.Storage.RaftSize),
							},
						},
					},
				},
			},
		},
	}
}

// buildEnv builds environment variables for the container
func (r *StreamBusClusterReconciler) buildEnv(cluster *streambusv1alpha1.StreamBusCluster) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "STREAMBUS_LOG_LEVEL",
			Value: cluster.Spec.Config.LogLevel,
		},
		{
			Name:  "STREAMBUS_PORT",
			Value: fmt.Sprintf("%d", cluster.Spec.Config.Port),
		},
		{
			Name:  "STREAMBUS_HTTP_PORT",
			Value: fmt.Sprintf("%d", cluster.Spec.Config.HTTPPort),
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}

	if cluster.Spec.MultiTenancy.Enabled {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "STREAMBUS_MULTI_TENANCY_ENABLED",
			Value: "true",
		})
	}

	if cluster.Spec.Observability.Tracing.Enabled {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "STREAMBUS_TRACING_ENABLED",
			Value: "true",
		})
		if cluster.Spec.Observability.Tracing.Endpoint != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  "STREAMBUS_TRACING_ENDPOINT",
				Value: cluster.Spec.Observability.Tracing.Endpoint,
			})
		}
	}

	return envVars
}

// generateConfig generates configuration for the cluster
func (r *StreamBusClusterReconciler) generateConfig(cluster *streambusv1alpha1.StreamBusCluster) map[string]string {
	var b strings.Builder

	// These keys are nested because that is what the broker actually reads
	// (cmd/broker/main.go looks up server.port, storage.data_dir,
	// cluster.raft.data_dir and observability.logging.level). An earlier
	// version of this function emitted the same settings as flat top-level
	// keys, which viper never matched, so every value here was silently
	// ignored and the broker fell back to defaults - or, for the required
	// ones, refused to start at all.
	//
	// server.broker_id is deliberately absent: every pod mounts this same
	// ConfigMap, so the id has to be per-pod and is supplied through the
	// environment instead (see buildCommand).
	fmt.Fprintf(&b, "server:\n  host: 0.0.0.0\n  port: %d\n  http_port: %d\n  grpc_port: %d\n",
		cluster.Spec.Config.Port, cluster.Spec.Config.HTTPPort, cluster.Spec.Config.GRPCPort)
	b.WriteString("storage:\n  data_dir: /data\n")
	b.WriteString("cluster:\n  raft:\n    data_dir: /raft\n  peers:\n")
	for _, peer := range raftPeers(cluster) {
		fmt.Fprintf(&b, "    - %q\n", peer)
	}
	fmt.Fprintf(&b, "observability:\n  logging:\n    level: %s\n", cluster.Spec.Config.LogLevel)
	fmt.Fprintf(&b, "multi_tenancy_enabled: %v\n", cluster.Spec.MultiTenancy.Enabled)

	writeSecurityConfig(&b, cluster)

	return map[string]string{"broker.yaml": b.String()}
}

// raftPeers builds the peer list every broker in the cluster shares. Each
// entry is "id:host:port" as the broker parses it, addressing pods through the
// headless Service so a restarted pod keeps a stable name.
//
// Broker ids are the pod ordinal plus one, because the broker treats id 0 as
// unset and refuses to start on it.
func raftPeers(cluster *streambusv1alpha1.StreamBusCluster) []string {
	peers := make([]string, 0, cluster.Spec.Replicas)
	for i := int32(0); i < cluster.Spec.Replicas; i++ {
		peers = append(peers, fmt.Sprintf("%d:%s-%d.%s-headless.%s.svc.cluster.local:%d",
			i+1, cluster.Name, i, cluster.Name, cluster.Namespace, raftPort))
	}
	return peers
}

// writeSecurityConfig renders spec.security into the broker's security
// section. Before this existed the whole spec.security block was inert: the
// fields were accepted by the API server, validated, and then read by nothing,
// so a cluster declaring TLS and SASL came up completely open.
//
// Nothing is written when security is disabled, so the broker keeps treating
// an absent section as "no security" rather than receiving an empty block it
// would reject.
func writeSecurityConfig(b *strings.Builder, cluster *streambusv1alpha1.StreamBusCluster) {
	sec := cluster.Spec.Security
	if !sec.Enabled {
		return
	}

	b.WriteString("security:\n  enabled: true\n")

	if sec.TLS.Enabled {
		// Paths point at the mounted Secret, whose keys follow the convention
		// of a kubernetes.io/tls Secret (tls.crt, tls.key) plus an optional
		// ca.crt used to verify client certificates.
		b.WriteString("  tls:\n    enabled: true\n")
		fmt.Fprintf(b, "    cert_file: %s/tls.crt\n", tlsMountPath)
		fmt.Fprintf(b, "    key_file: %s/tls.key\n", tlsMountPath)
		fmt.Fprintf(b, "    ca_file: %s/ca.crt\n", tlsMountPath)
	}

	if sec.Authentication.Enabled {
		b.WriteString("  sasl:\n    enabled: true\n")
		fmt.Fprintf(b, "    mechanisms: [%q]\n", sec.Authentication.SASL.Mechanism)
		fmt.Fprintf(b, "    users_dir: %s\n", saslMountPath)
	}
}

// buildCommand wraps the broker binary in a shell so the per-pod Raft id can
// be derived from the pod's own name.
//
// Every pod mounts the same ConfigMap, so server.broker_id cannot live there.
// The downward API can supply the pod name but cannot do arithmetic, and the
// broker rejects id 0 as "unset" - so the ordinal has to be incremented
// somewhere, and a shell is the only place available without an init
// container. STREAMBUS_SERVER_BROKER_ID reaches viper's nested server.broker_id
// through the env key replacer installed in cmd/broker.
//
// The explicit --config matters too: the image's default command points at
// /config/broker.yaml, which this operator does not mount, and the broker
// exits non-zero when an explicitly named config file is missing.
func buildCommand() []string {
	return []string{
		"/bin/sh",
		"-c",
		fmt.Sprintf(
			"set -e; export STREAMBUS_SERVER_BROKER_ID=$(( ${POD_NAME##*-} + 1 )); "+
				"exec %s --config=%s/broker.yaml",
			brokerBinary, configMountPath),
	}
}

// buildVolumeMounts returns the container's mounts, adding the TLS and SASL
// Secret mounts only when spec.security actually asks for them.
func buildVolumeMounts(cluster *streambusv1alpha1.StreamBusCluster) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/data"},
		{Name: "raft-data", MountPath: "/raft"},
		{Name: "config", MountPath: configMountPath},
	}

	if tlsSecretName(cluster) != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name: "tls", MountPath: tlsMountPath, ReadOnly: true,
		})
	}
	if saslSecretName(cluster) != "" {
		mounts = append(mounts, corev1.VolumeMount{
			Name: "sasl", MountPath: saslMountPath, ReadOnly: true,
		})
	}
	return mounts
}

// buildVolumes returns the pod's volumes, including the security Secrets when
// they are configured.
func buildVolumes(cluster *streambusv1alpha1.StreamBusCluster) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cluster.Name + "-config",
					},
				},
			},
		},
	}

	if name := tlsSecretName(cluster); name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: name},
			},
		})
	}
	if name := saslSecretName(cluster); name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "sasl",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: name},
			},
		})
	}
	return volumes
}

// tlsSecretName returns the Secret holding TLS material, or "" when TLS is not
// enabled. The name is only meaningful when security and TLS are both on, so
// the checks live here rather than at each call site.
func tlsSecretName(cluster *streambusv1alpha1.StreamBusCluster) string {
	sec := cluster.Spec.Security
	if !sec.Enabled || !sec.TLS.Enabled {
		return ""
	}
	return sec.TLS.SecretName
}

// saslSecretName returns the Secret holding SASL credentials, or "" when
// authentication is not enabled.
func saslSecretName(cluster *streambusv1alpha1.StreamBusCluster) string {
	sec := cluster.Spec.Security
	if !sec.Enabled || !sec.Authentication.Enabled {
		return ""
	}
	return sec.Authentication.SASL.SecretName
}

// getImage returns the full image name
func (r *StreamBusClusterReconciler) getImage(cluster *streambusv1alpha1.StreamBusCluster) string {
	repo := cluster.Spec.Image.Repository
	if repo == "" {
		repo = "streambus/broker"
	}

	tag := cluster.Spec.Image.Tag
	if tag == "" {
		tag = "latest"
	}

	return fmt.Sprintf("%s:%s", repo, tag)
}

// updateStatus updates the cluster status
func (r *StreamBusClusterReconciler) updateStatus(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) error {
	// Get StatefulSet
	statefulSet := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, statefulSet)
	if err != nil {
		if errors.IsNotFound(err) {
			cluster.Status.Phase = streambusv1alpha1.ClusterPhasePending
			return r.Status().Update(ctx, cluster)
		}
		return err
	}

	// Update status
	cluster.Status.Replicas = statefulSet.Status.Replicas
	cluster.Status.ReadyReplicas = statefulSet.Status.ReadyReplicas
	cluster.Status.ObservedGeneration = cluster.Generation

	// Determine phase
	if cluster.Status.ReadyReplicas == 0 {
		cluster.Status.Phase = streambusv1alpha1.ClusterPhaseCreating
	} else if cluster.Status.ReadyReplicas == cluster.Spec.Replicas {
		cluster.Status.Phase = streambusv1alpha1.ClusterPhaseRunning
	} else if cluster.Status.ReadyReplicas < cluster.Spec.Replicas {
		cluster.Status.Phase = streambusv1alpha1.ClusterPhaseDegraded
	}

	// Update endpoints
	cluster.Status.Endpoints.Brokers = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		cluster.Name, cluster.Namespace, cluster.Spec.Config.Port)
	cluster.Status.Endpoints.HTTP = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		cluster.Name, cluster.Namespace, cluster.Spec.Config.HTTPPort)
	cluster.Status.Endpoints.Metrics = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/metrics",
		cluster.Name, cluster.Namespace, cluster.Spec.Config.HTTPPort)

	return r.Status().Update(ctx, cluster)
}

// labelsForCluster returns labels for cluster resources
func (r *StreamBusClusterReconciler) labelsForCluster(cluster *streambusv1alpha1.StreamBusCluster) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "streambus",
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/managed-by": "streambus-operator",
		"app.kubernetes.io/component":  "broker",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *StreamBusClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&streambusv1alpha1.StreamBusCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
