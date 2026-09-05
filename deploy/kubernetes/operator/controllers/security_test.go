package controllers

import (
	"strings"
	"testing"

	streambusv1alpha1 "github.com/gstreamio/streambus/deploy/kubernetes/operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// secureCluster builds a CR with security switched on, so each test can adjust
// only the part it cares about.
func secureCluster(mutate func(*streambusv1alpha1.StreamBusCluster)) *streambusv1alpha1.StreamBusCluster {
	cluster := &streambusv1alpha1.StreamBusCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "prod"},
		Spec: streambusv1alpha1.StreamBusClusterSpec{
			Replicas: 3,
			Config: streambusv1alpha1.ConfigSpec{
				LogLevel: "info", Port: 9092, HTTPPort: 8080, GRPCPort: 9093,
			},
			Storage: streambusv1alpha1.StorageSpec{
				Class: "standard", Size: "10Gi", RaftSize: "5Gi",
			},
			Security: streambusv1alpha1.SecuritySpec{
				Enabled: true,
				TLS: streambusv1alpha1.TLSSpec{
					Enabled:    true,
					SecretName: "demo-tls",
				},
				Authentication: streambusv1alpha1.AuthenticationSpec{
					Enabled: true,
					SASL: streambusv1alpha1.SASLSpec{
						Mechanism:  "SCRAM-SHA-512",
						SecretName: "demo-sasl",
					},
				},
			},
		},
	}
	if mutate != nil {
		mutate(cluster)
	}
	return cluster
}

func generatedConfig(cluster *streambusv1alpha1.StreamBusCluster) string {
	r := &StreamBusClusterReconciler{}
	return r.generateConfig(cluster)["broker.yaml"]
}

// TestGenerateConfig_UsesTheKeysTheBrokerReads guards the defect this change
// fixes. The broker looks up server.port, storage.data_dir,
// cluster.raft.data_dir and observability.logging.level; an earlier version
// emitted those as flat top-level keys, which viper never matched, so the
// whole ConfigMap was inert and the broker refused to start.
func TestGenerateConfig_UsesTheKeysTheBrokerReads(t *testing.T) {
	config := generatedConfig(secureCluster(nil))

	for _, want := range []string{
		"server:", "  port: 9092", "  http_port: 8080", "  grpc_port: 9093",
		"storage:", "  data_dir: /data",
		"cluster:", "  raft:", "    data_dir: /raft",
		"observability:", "  logging:", "    level: info",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("config is missing %q:\n%s", want, config)
		}
	}

	// The flat spellings must not come back.
	for _, unwanted := range []string{"\nlog_level:", "\nport:", "\nhttp_port:", "\ndata_dir:", "\nraft_data_dir:"} {
		if strings.Contains(config, unwanted) {
			t.Errorf("config still emits the flat key %q, which the broker never reads:\n%s", unwanted, config)
		}
	}
}

// TestGenerateConfig_PeersAddressPodsThroughTheHeadlessService pins the peer
// format the broker parses ("id:host:port") and the +1 offset, which exists
// because the broker rejects broker id 0 as unset.
func TestGenerateConfig_PeersAddressPodsThroughTheHeadlessService(t *testing.T) {
	config := generatedConfig(secureCluster(nil))

	for i, want := range []string{
		`"1:demo-0.demo-headless.prod.svc.cluster.local:7000"`,
		`"2:demo-1.demo-headless.prod.svc.cluster.local:7000"`,
		`"3:demo-2.demo-headless.prod.svc.cluster.local:7000"`,
	} {
		if !strings.Contains(config, want) {
			t.Errorf("peer %d missing %s:\n%s", i, want, config)
		}
	}
	if strings.Contains(config, `"0:`) {
		t.Error("a peer was given id 0, which the broker treats as unset and refuses to start on")
	}
}

// TestGenerateConfig_SecurityRendered is the core of the fix: spec.security
// previously reached nothing at all.
func TestGenerateConfig_SecurityRendered(t *testing.T) {
	config := generatedConfig(secureCluster(nil))

	for _, want := range []string{
		"security:",
		"  enabled: true",
		"  tls:",
		"    cert_file: /etc/streambus/tls/tls.crt",
		"    key_file: /etc/streambus/tls/tls.key",
		"    ca_file: /etc/streambus/tls/ca.crt",
		"  sasl:",
		`    mechanisms: ["SCRAM-SHA-512"]`,
		"    users_dir: /etc/streambus/sasl",
	} {
		if !strings.Contains(config, want) {
			t.Errorf("security config missing %q:\n%s", want, config)
		}
	}
}

func TestGenerateConfig_SecurityOmittedWhenDisabled(t *testing.T) {
	// An absent section is how the broker expresses "no security"; emitting an
	// empty block would make it refuse to start.
	config := generatedConfig(secureCluster(func(c *streambusv1alpha1.StreamBusCluster) {
		c.Spec.Security.Enabled = false
	}))
	if strings.Contains(config, "security:") {
		t.Errorf("security section emitted despite being disabled:\n%s", config)
	}
}

func TestGenerateConfig_SecurityEnabledWithoutTLSOrSASL(t *testing.T) {
	// Enabling security without TLS or SASL must not silently render an empty
	// security block - the broker rejects that, and rightly so.
	config := generatedConfig(secureCluster(func(c *streambusv1alpha1.StreamBusCluster) {
		c.Spec.Security.TLS.Enabled = false
		c.Spec.Security.Authentication.Enabled = false
	}))
	if strings.Contains(config, "tls:") || strings.Contains(config, "sasl:") {
		t.Errorf("rendered TLS or SASL that was not requested:\n%s", config)
	}
}

// findVolume and findMount keep the assertions below readable.
func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

func findMount(mounts []corev1.VolumeMount, name string) *corev1.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i]
		}
	}
	return nil
}

func TestBuildStatefulSet_MountsSecuritySecrets(t *testing.T) {
	r := &StreamBusClusterReconciler{}
	sts := r.buildStatefulSet(secureCluster(nil))
	spec := sts.Spec.Template.Spec

	tlsVol := findVolume(spec.Volumes, "tls")
	if tlsVol == nil || tlsVol.Secret == nil {
		t.Fatalf("no TLS secret volume: %+v", spec.Volumes)
	}
	if tlsVol.Secret.SecretName != "demo-tls" {
		t.Errorf("TLS volume references %q, want demo-tls", tlsVol.Secret.SecretName)
	}

	saslVol := findVolume(spec.Volumes, "sasl")
	if saslVol == nil || saslVol.Secret == nil {
		t.Fatalf("no SASL secret volume: %+v", spec.Volumes)
	}
	if saslVol.Secret.SecretName != "demo-sasl" {
		t.Errorf("SASL volume references %q, want demo-sasl", saslVol.Secret.SecretName)
	}

	mounts := spec.Containers[0].VolumeMounts
	tlsMount := findMount(mounts, "tls")
	if tlsMount == nil {
		t.Fatal("TLS secret is not mounted into the container")
	}
	if tlsMount.MountPath != tlsMountPath {
		t.Errorf("TLS mounted at %q, want %q - the config file points at the latter",
			tlsMount.MountPath, tlsMountPath)
	}
	if !tlsMount.ReadOnly {
		t.Error("TLS secret should be mounted read-only")
	}

	saslMount := findMount(mounts, "sasl")
	if saslMount == nil {
		t.Fatal("SASL secret is not mounted into the container")
	}
	if saslMount.MountPath != saslMountPath {
		t.Errorf("SASL mounted at %q, want %q", saslMount.MountPath, saslMountPath)
	}
}

func TestBuildStatefulSet_NoSecretVolumesWhenSecurityDisabled(t *testing.T) {
	r := &StreamBusClusterReconciler{}
	sts := r.buildStatefulSet(secureCluster(func(c *streambusv1alpha1.StreamBusCluster) {
		c.Spec.Security.Enabled = false
	}))
	spec := sts.Spec.Template.Spec

	if findVolume(spec.Volumes, "tls") != nil || findVolume(spec.Volumes, "sasl") != nil {
		t.Errorf("security volumes present despite security being disabled: %+v", spec.Volumes)
	}
	if findMount(spec.Containers[0].VolumeMounts, "tls") != nil {
		t.Error("TLS mount present despite security being disabled")
	}
}

// TestBuildStatefulSet_SuppliesPerPodBrokerID covers the other reason an
// operator-deployed broker could not start: server.broker_id is required, is
// per-pod, and therefore cannot live in the shared ConfigMap.
func TestBuildStatefulSet_SuppliesPerPodBrokerID(t *testing.T) {
	r := &StreamBusClusterReconciler{}
	sts := r.buildStatefulSet(secureCluster(nil))
	command := strings.Join(sts.Spec.Template.Spec.Containers[0].Command, " ")

	if !strings.Contains(command, "STREAMBUS_SERVER_BROKER_ID") {
		t.Errorf("container command does not set the broker id:\n%s", command)
	}
	if !strings.Contains(command, "POD_NAME") {
		t.Errorf("broker id is not derived from the pod ordinal:\n%s", command)
	}
	// The image's default command points at /config/broker.yaml, which this
	// operator does not mount; the broker exits non-zero on a missing explicit
	// config file, so the path has to be overridden.
	if !strings.Contains(command, configMountPath+"/broker.yaml") {
		t.Errorf("container command does not point at the mounted config:\n%s", command)
	}
}
