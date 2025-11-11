package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StreamBusClusterSpec defines the desired state of StreamBusCluster
type StreamBusClusterSpec struct {
	// Replicas is the number of broker instances
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas"`

	// Version is the StreamBus version to deploy
	// +kubebuilder:default="latest"
	// +optional
	Version string `json:"version,omitempty"`

	// Image configuration
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Resources defines resource requirements
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Config is the StreamBus configuration
	// +optional
	Config ConfigSpec `json:"config,omitempty"`

	// Security configuration
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// MultiTenancy configuration
	// +optional
	MultiTenancy MultiTenancySpec `json:"multiTenancy,omitempty"`

	// Observability configuration
	// +optional
	Observability ObservabilitySpec `json:"observability,omitempty"`

	// Affinity rules for pod scheduling
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations for pod scheduling
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector for pod scheduling
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// PodAnnotations are annotations to add to broker pods
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// PodLabels are labels to add to broker pods
	// +optional
	PodLabels map[string]string `json:"podLabels,omitempty"`
}

// ImageSpec defines container image configuration
type ImageSpec struct {
	// Repository is the image repository
	// +kubebuilder:default="streambus/broker"
	Repository string `json:"repository,omitempty"`

	// Tag is the image tag
	// +kubebuilder:default="latest"
	Tag string `json:"tag,omitempty"`

	// PullPolicy is the image pull policy
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +kubebuilder:default="IfNotPresent"
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`

	// PullSecrets are image pull secrets
	// +optional
	PullSecrets []string `json:"pullSecrets,omitempty"`
}

// StorageSpec defines storage configuration
type StorageSpec struct {
	// Class is the StorageClass name
	// +kubebuilder:default="standard"
	Class string `json:"class,omitempty"`

	// Size is the data volume size
	// +kubebuilder:default="10Gi"
	Size string `json:"size,omitempty"`

	// RaftSize is the Raft data volume size
	// +kubebuilder:default="5Gi"
	RaftSize string `json:"raftSize,omitempty"`
}

// ConfigSpec defines StreamBus configuration
type ConfigSpec struct {
	// LogLevel sets the logging level
	// +kubebuilder:validation:Enum=debug;info;warn;error
	// +kubebuilder:default="info"
	LogLevel string `json:"logLevel,omitempty"`

	// Port is the broker port
	// +kubebuilder:default=9092
	Port int32 `json:"port,omitempty"`

	// HTTPPort is the HTTP/metrics port
	// +kubebuilder:default=8081
	HTTPPort int32 `json:"httpPort,omitempty"`

	// GRPCPort is the gRPC port
	// +kubebuilder:default=9093
	GRPCPort int32 `json:"grpcPort,omitempty"`
}

// SecuritySpec defines security configuration
type SecuritySpec struct {
	// Enabled enables security features
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// TLS configuration
	// +optional
	TLS TLSSpec `json:"tls,omitempty"`

	// Authentication configuration
	// +optional
	Authentication AuthenticationSpec `json:"authentication,omitempty"`
}

// TLSSpec defines TLS configuration
type TLSSpec struct {
	// Enabled enables TLS
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// SecretName is the name of the secret containing TLS certificates
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// AuthenticationSpec defines authentication configuration
type AuthenticationSpec struct {
	// Enabled enables authentication
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// SASL configuration
	// +optional
	SASL SASLSpec `json:"sasl,omitempty"`
}

// SASLSpec defines SASL configuration
type SASLSpec struct {
	// Mechanism is the SASL mechanism
	// +kubebuilder:validation:Enum=PLAIN;SCRAM-SHA-256;SCRAM-SHA-512
	// +kubebuilder:default="SCRAM-SHA-256"
	Mechanism string `json:"mechanism,omitempty"`

	// SecretName is the name of the secret containing SASL credentials
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// MultiTenancySpec defines multi-tenancy configuration
type MultiTenancySpec struct {
	// Enabled enables multi-tenancy
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

// ObservabilitySpec defines observability configuration
type ObservabilitySpec struct {
	// Metrics configuration
	// +optional
	Metrics MetricsSpec `json:"metrics,omitempty"`

	// Tracing configuration
	// +optional
	Tracing TracingSpec `json:"tracing,omitempty"`
}

// MetricsSpec defines metrics configuration
type MetricsSpec struct {
	// Enabled enables metrics
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// ServiceMonitor creates a Prometheus ServiceMonitor
	// +kubebuilder:default=false
	ServiceMonitor bool `json:"serviceMonitor,omitempty"`
}

// TracingSpec defines tracing configuration
type TracingSpec struct {
	// Enabled enables tracing
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Endpoint is the OTLP endpoint
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// ClusterPhase represents the current phase of the cluster
type ClusterPhase string

const (
	ClusterPhasePending  ClusterPhase = "Pending"
	ClusterPhaseCreating ClusterPhase = "Creating"
	ClusterPhaseRunning  ClusterPhase = "Running"
	ClusterPhaseUpdating ClusterPhase = "Updating"
	ClusterPhaseDegraded ClusterPhase = "Degraded"
	ClusterPhaseFailed   ClusterPhase = "Failed"
)

// StreamBusClusterStatus defines the observed state of StreamBusCluster
type StreamBusClusterStatus struct {
	// Phase is the current phase of the cluster
	// +optional
	Phase ClusterPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations of the cluster's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Replicas is the total number of non-terminated pods
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of ready replicas
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// ObservedGeneration is the most recent generation observed by the operator
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Endpoints contains cluster endpoints
	// +optional
	Endpoints EndpointsStatus `json:"endpoints,omitempty"`
}

// EndpointsStatus contains cluster endpoint information
type EndpointsStatus struct {
	// Brokers is a comma-separated list of broker addresses
	// +optional
	Brokers string `json:"brokers,omitempty"`

	// HTTP is the HTTP endpoint for management
	// +optional
	HTTP string `json:"http,omitempty"`

	// Metrics is the metrics endpoint
	// +optional
	Metrics string `json:"metrics,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sbc;streambus
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// StreamBusCluster is the Schema for the streambusclusters API
type StreamBusCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StreamBusClusterSpec   `json:"spec,omitempty"`
	Status StreamBusClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StreamBusClusterList contains a list of StreamBusCluster
type StreamBusClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StreamBusCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StreamBusCluster{}, &StreamBusClusterList{})
}
