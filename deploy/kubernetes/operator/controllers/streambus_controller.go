package controllers

import (
	"context"
	"fmt"
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

	streambusv1alpha1 "github.com/shawntherrien/streambus/deploy/kubernetes/operator/api/v1alpha1"
)

const (
	finalizerName = "streambus.io/finalizer"
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
	if cluster.ObjectMeta.DeletionTimestamp != nil {
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
					TargetPort: intstr.FromString("broker"),
				},
				{
					Name:       "raft",
					Port:       7000,
					TargetPort: intstr.FromString("raft"),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cluster, service, r.Scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, service)
	}
	return err
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
					TargetPort: intstr.FromString("broker"),
				},
				{
					Name:       "http",
					Port:       cluster.Spec.Config.HTTPPort,
					TargetPort: intstr.FromString("http"),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cluster, service, r.Scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, service)
	}
	return err
}

// reconcileStatefulSet creates or updates the StatefulSet
func (r *StreamBusClusterReconciler) reconcileStatefulSet(ctx context.Context, cluster *streambusv1alpha1.StreamBusCluster) error {
	statefulSet := r.buildStatefulSet(cluster)

	if err := controllerutil.SetControllerReference(cluster, statefulSet, r.Scheme); err != nil {
		return err
	}

	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// Update if replicas changed
	if *found.Spec.Replicas != *statefulSet.Spec.Replicas {
		found.Spec.Replicas = statefulSet.Spec.Replicas
		return r.Update(ctx, found)
	}

	return nil
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
							Ports: []corev1.ContainerPort{
								{
									Name:          "broker",
									ContainerPort: cluster.Spec.Config.Port,
								},
								{
									Name:          "http",
									ContainerPort: cluster.Spec.Config.HTTPPort,
								},
								{
									Name:          "grpc",
									ContainerPort: cluster.Spec.Config.GRPCPort,
								},
								{
									Name:          "raft",
									ContainerPort: 7000,
								},
							},
							Env: r.buildEnv(cluster),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/data",
								},
								{
									Name:      "raft-data",
									MountPath: "/raft",
								},
								{
									Name:      "config",
									MountPath: "/etc/streambus",
								},
							},
							Resources: cluster.Spec.Resources,
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
					Volumes: []corev1.Volume{
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
					},
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
	config := map[string]string{
		"broker.yaml": fmt.Sprintf(`
log_level: %s
port: %d
http_port: %d
grpc_port: %d
data_dir: /data
raft_data_dir: /raft
multi_tenancy_enabled: %v
`, cluster.Spec.Config.LogLevel,
			cluster.Spec.Config.Port,
			cluster.Spec.Config.HTTPPort,
			cluster.Spec.Config.GRPCPort,
			cluster.Spec.MultiTenancy.Enabled),
	}

	return config
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
