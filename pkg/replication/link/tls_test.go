package link

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gstreamio/streambus/pkg/protocol"
)

// testCA is a self-signed certificate authority generated in-process for
// exercising real TLS handshakes, rather than mocking them. Its cert is
// written to disk (TLSCAFile only ever names a path) and kept in memory to
// sign leaf certificates.
type testCA struct {
	cert     *x509.Certificate
	priv     *ecdsa.PrivateKey
	certFile string
}

// newTestCA creates a CA certificate/key pair and writes the certificate as
// a PEM file under dir.
func newTestCA(t *testing.T, dir string) *testCA {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "streambus-replication-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}

	certFile := filepath.Join(dir, "ca-cert.pem")
	writePEM(t, certFile, "CERTIFICATE", der)

	return &testCA{cert: cert, priv: priv, certFile: certFile}
}

// issueLeaf issues a certificate signed by ca for either server or client
// use, writing both the cert and key as PEM files under dir and returning
// their paths.
func (ca *testCA) issueLeaf(t *testing.T, dir, name string, ips []net.IP, serverAuth bool) (certFile, keyFile string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate %s key: %v", name, err)
	}

	extKeyUsage := x509.ExtKeyUsageClientAuth
	if serverAuth {
		extKeyUsage = x509.ExtKeyUsageServerAuth
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{extKeyUsage},
		IPAddresses:  ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &priv.PublicKey, ca.priv)
	if err != nil {
		t.Fatalf("failed to create %s certificate: %v", name, err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal %s key: %v", name, err)
	}

	certFile = filepath.Join(dir, name+"-cert.pem")
	keyFile = filepath.Join(dir, name+"-key.pem")
	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)

	return certFile, keyFile
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("failed to create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// startFakeTLSBroker starts a minimal in-process broker over a TLS listener
// built from tlsConfig. It speaks just enough of the StreamBus wire protocol
// to answer a HealthCheckRequest, which is what connectToCluster's
// reachability check sends - so a successful test here proves a real TLS
// handshake (dial, certificate verification, and optional client
// authentication) completed, not merely that a bare socket connected.
func startFakeTLSBroker(t *testing.T, tlsConfig *tls.Config) string {
	t.Helper()

	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("failed to start TLS listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go respondToOneHealthCheck(conn)
		}
	}()

	return listener.Addr().String()
}

// respondToOneHealthCheck answers a single request on conn as a healthy
// HealthCheckResponse, then closes the connection. Any failure (including a
// TLS handshake failure driven lazily by the first Read) simply closes the
// connection - the client side of that failure is what the "fails" tests
// assert on.
func respondToOneHealthCheck(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	codec := protocol.NewCodec()
	req, err := codec.DecodeRequest(conn)
	if err != nil {
		return
	}

	resp := &protocol.Response{
		Header: protocol.ResponseHeader{
			RequestID: req.Header.RequestID,
			Status:    protocol.StatusOK,
		},
		Payload: &protocol.HealthCheckResponse{Status: "healthy"},
	}
	_ = codec.EncodeResponse(conn, resp)
}

// testClusterConfig returns a ClusterConfig pointed at addr with fast
// timeouts and no retries, so a test that expects connectToCluster to fail
// does not sit through retry backoff.
func testClusterConfig(addr string) *ClusterConfig {
	return &ClusterConfig{
		ClusterID:         "tls-test-cluster",
		Brokers:           []string{addr},
		ConnectionTimeout: 5 * time.Second,
		RequestTimeout:    5 * time.Second,
		RetryBackoff:      50 * time.Millisecond,
		MaxRetries:        0,
	}
}

func newTLSTestHandler(t *testing.T) *StreamHandler {
	t.Helper()

	link := createTestLink("tls-test", "TLS Test")
	handler, err := NewStreamHandler(link, NewMemoryStorage())
	if err != nil {
		t.Fatalf("NewStreamHandler failed: %v", err)
	}
	t.Cleanup(func() { _ = handler.Stop() })

	return handler
}

// TestStreamHandler_ConnectToCluster_TLS_WithCA_Succeeds proves a link
// configured with the CA that signed the broker's certificate can connect
// over real TLS.
func TestStreamHandler_ConnectToCluster_TLS_WithCA_Succeeds(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, dir)
	serverCertFile, serverKeyFile := ca.issueLeaf(t, dir, "server", []net.IP{net.ParseIP("127.0.0.1")}, true)

	serverCert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
	if err != nil {
		t.Fatalf("failed to load server cert: %v", err)
	}

	addr := startFakeTLSBroker(t, &tls.Config{Certificates: []tls.Certificate{serverCert}})

	handler := newTLSTestHandler(t)

	config := testClusterConfig(addr)
	config.Security = &SecurityConfig{
		EnableTLS: true,
		TLSCAFile: ca.certFile,
	}

	c, err := handler.connectToCluster(config)
	if err != nil {
		t.Fatalf("expected TLS connection with the correct CA to succeed, got: %v", err)
	}
	_ = c.Close()
}

// TestStreamHandler_ConnectToCluster_TLS_WithoutCA_Fails is the most
// important test here: it proves certificate verification is actually
// enforced, by connecting to the same broker without configuring the CA that
// signed its certificate and expecting the connection to be refused rather
// than silently accepted.
func TestStreamHandler_ConnectToCluster_TLS_WithoutCA_Fails(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, dir)
	serverCertFile, serverKeyFile := ca.issueLeaf(t, dir, "server", []net.IP{net.ParseIP("127.0.0.1")}, true)

	serverCert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
	if err != nil {
		t.Fatalf("failed to load server cert: %v", err)
	}

	addr := startFakeTLSBroker(t, &tls.Config{Certificates: []tls.Certificate{serverCert}})

	handler := newTLSTestHandler(t)

	config := testClusterConfig(addr)
	config.Security = &SecurityConfig{
		EnableTLS: true,
		// Deliberately no TLSCAFile: the broker's certificate is signed by
		// a CA the client has no reason to trust, and is not in any real
		// system trust store either.
	}

	if _, err := handler.connectToCluster(config); err == nil {
		t.Fatal("expected connection without the signing CA to fail certificate verification, but it succeeded")
	}
}

// TestStreamHandler_ConnectToCluster_MTLS exercises mutual TLS: the broker
// requires a client certificate signed by the same CA, and rejects a client
// that presents none while accepting one that presents a valid certificate.
func TestStreamHandler_ConnectToCluster_MTLS(t *testing.T) {
	dir := t.TempDir()
	ca := newTestCA(t, dir)
	serverCertFile, serverKeyFile := ca.issueLeaf(t, dir, "server", []net.IP{net.ParseIP("127.0.0.1")}, true)
	clientCertFile, clientKeyFile := ca.issueLeaf(t, dir, "client", nil, false)

	serverCert, err := tls.LoadX509KeyPair(serverCertFile, serverKeyFile)
	if err != nil {
		t.Fatalf("failed to load server cert: %v", err)
	}

	clientCAPool := x509.NewCertPool()
	clientCAPool.AddCert(ca.cert)

	addr := startFakeTLSBroker(t, &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	})

	handler := newTLSTestHandler(t)

	t.Run("no client certificate is rejected", func(t *testing.T) {
		config := testClusterConfig(addr)
		config.Security = &SecurityConfig{
			EnableTLS: true,
			TLSCAFile: ca.certFile,
			// No TLSCertFile/TLSKeyFile: the mTLS server must reject this.
		}

		if _, err := handler.connectToCluster(config); err == nil {
			t.Fatal("expected mTLS server to reject a client presenting no certificate")
		}
	})

	t.Run("valid client certificate is accepted", func(t *testing.T) {
		config := testClusterConfig(addr)
		config.Security = &SecurityConfig{
			EnableTLS:   true,
			TLSCAFile:   ca.certFile,
			TLSCertFile: clientCertFile,
			TLSKeyFile:  clientKeyFile,
		}

		c, err := handler.connectToCluster(config)
		if err != nil {
			t.Fatalf("expected mTLS server to accept a valid client certificate, got: %v", err)
		}
		_ = c.Close()
	})
}

// TestBuildClientSecurityConfig is a fast unit test of the pure translation
// from a link's SecurityConfig to the client's SecurityConfig, independent
// of any real network connection.
func TestBuildClientSecurityConfig(t *testing.T) {
	if got := buildClientSecurityConfig(nil); got != nil {
		t.Errorf("expected nil for nil SecurityConfig, got %+v", got)
	}

	if got := buildClientSecurityConfig(&SecurityConfig{EnableTLS: false}); got != nil {
		t.Errorf("expected nil when EnableTLS is false, got %+v", got)
	}

	sec := &SecurityConfig{
		EnableTLS:     true,
		TLSCertFile:   "/cert.pem",
		TLSKeyFile:    "/key.pem",
		TLSCAFile:     "/ca.pem",
		TLSSkipVerify: false,
		TLSServerName: "broker.example.com",
	}
	got := buildClientSecurityConfig(sec)
	if got == nil || got.TLS == nil {
		t.Fatalf("expected a populated client TLS config, got %+v", got)
	}
	if !got.TLS.Enabled {
		t.Error("expected TLS.Enabled to be true")
	}
	if got.TLS.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to default to false - verification must be on unless explicitly disabled")
	}
	if got.TLS.CertFile != sec.TLSCertFile || got.TLS.KeyFile != sec.TLSKeyFile ||
		got.TLS.CAFile != sec.TLSCAFile || got.TLS.ServerName != sec.TLSServerName {
		t.Errorf("expected fields to be carried across unchanged, got %+v", got.TLS)
	}

	skipVerify := buildClientSecurityConfig(&SecurityConfig{EnableTLS: true, TLSSkipVerify: true})
	if !skipVerify.TLS.InsecureSkipVerify {
		t.Error("expected an explicit TLSSkipVerify to translate to InsecureSkipVerify")
	}
}
