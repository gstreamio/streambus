package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test certificate generation helpers
func generateTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"StreamBus Test CA"},
			CommonName:   "StreamBus Test CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("Failed to create CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse CA certificate: %v", err)
	}

	return cert, priv
}

func generateTestCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, cn string, dnsNames []string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"StreamBus Test"},
			CommonName:   cn,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    dnsNames,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert, priv
}

func writeCertToFile(t *testing.T, cert *x509.Certificate, filename string) {
	t.Helper()

	certOut, err := os.Create(filename)
	if err != nil {
		t.Fatalf("Failed to create cert file: %v", err)
	}
	defer certOut.Close()

	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		t.Fatalf("Failed to write cert: %v", err)
	}
}

func writeKeyToFile(t *testing.T, key *rsa.PrivateKey, filename string) {
	t.Helper()

	keyOut, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("Failed to create key file: %v", err)
	}
	defer keyOut.Close()

	privBytes := x509.MarshalPKCS1PrivateKey(key)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}); err != nil {
		t.Fatalf("Failed to write key: %v", err)
	}
}

func TestNewTLSConfig(t *testing.T) {
	// Create temp directory for test certificates
	tmpDir, err := os.MkdirTemp("", "streambus-tls-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate CA
	caCert, caKey := generateTestCA(t)
	caFile := filepath.Join(tmpDir, "ca.pem")
	writeCertToFile(t, caCert, caFile)

	// Generate server certificate
	serverCert, serverKey := generateTestCert(t, caCert, caKey, "localhost", []string{"localhost"})
	certFile := filepath.Join(tmpDir, "server.pem")
	keyFile := filepath.Join(tmpDir, "server-key.pem")
	writeCertToFile(t, serverCert, certFile)
	writeKeyToFile(t, serverKey, keyFile)

	tests := []struct {
		name        string
		cfg         *TLSConfig
		expectError bool
		validate    func(*testing.T, *tls.Config)
	}{
		{
			name: "disabled TLS",
			cfg: &TLSConfig{
				Enabled: false,
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg != nil {
					t.Error("Expected nil config for disabled TLS")
				}
			},
		},
		{
			name: "basic TLS with cert",
			cfg: &TLSConfig{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  keyFile,
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if len(cfg.Certificates) != 1 {
					t.Errorf("Expected 1 certificate, got %d", len(cfg.Certificates))
				}
				if cfg.MinVersion != tls.VersionTLS12 {
					t.Errorf("Expected TLS 1.2 minimum, got %x", cfg.MinVersion)
				}
			},
		},
		{
			name: "TLS with CA",
			cfg: &TLSConfig{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if cfg.ClientCAs == nil {
					t.Error("Expected ClientCAs to be set")
				}
			},
		},
		{
			name: "require and verify client cert",
			cfg: &TLSConfig{
				Enabled:           true,
				CertFile:          certFile,
				KeyFile:           keyFile,
				CAFile:            caFile,
				RequireClientCert: true,
				VerifyClientCert:  true,
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
					t.Errorf("Expected RequireAndVerifyClientCert, got %v", cfg.ClientAuth)
				}
			},
		},
		{
			name: "require but don't verify client cert",
			cfg: &TLSConfig{
				Enabled:           true,
				CertFile:          certFile,
				KeyFile:           keyFile,
				RequireClientCert: true,
				VerifyClientCert:  false,
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if cfg.ClientAuth != tls.RequireAnyClientCert {
					t.Errorf("Expected RequireAnyClientCert, got %v", cfg.ClientAuth)
				}
			},
		},
		{
			name: "verify if given",
			cfg: &TLSConfig{
				Enabled:           true,
				CertFile:          certFile,
				KeyFile:           keyFile,
				CAFile:            caFile,
				RequireClientCert: false,
				VerifyClientCert:  true,
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if cfg.ClientAuth != tls.VerifyClientCertIfGiven {
					t.Errorf("Expected VerifyClientCertIfGiven, got %v", cfg.ClientAuth)
				}
			},
		},
		{
			name: "custom cipher suites",
			cfg: &TLSConfig{
				Enabled:      true,
				CertFile:     certFile,
				KeyFile:      keyFile,
				CipherSuites: []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384},
			},
			expectError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				if cfg == nil {
					t.Fatal("Expected non-nil config")
				}
				if len(cfg.CipherSuites) != 2 {
					t.Errorf("Expected 2 cipher suites, got %d", len(cfg.CipherSuites))
				}
				if !cfg.PreferServerCipherSuites {
					t.Error("Expected PreferServerCipherSuites to be true")
				}
			},
		},
		{
			name: "invalid cert file",
			cfg: &TLSConfig{
				Enabled:  true,
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  keyFile,
			},
			expectError: true,
		},
		{
			name: "invalid CA file",
			cfg: &TLSConfig{
				Enabled:  true,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   "/nonexistent/ca.pem",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewTLSConfig(tt.cfg)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestTLSConfig_ValidateClientCert(t *testing.T) {
	// Generate test certificates
	caCert, caKey := generateTestCA(t)
	cert1, _ := generateTestCert(t, caCert, caKey, "client-1", nil)
	_, _ = generateTestCert(t, caCert, caKey, "client-2", nil)

	tests := []struct {
		name        string
		cfg         *TLSConfig
		cert        *x509.Certificate
		expectError bool
	}{
		{
			name: "no CN restrictions",
			cfg: &TLSConfig{
				AllowedClientCNs: nil,
			},
			cert:        cert1,
			expectError: false,
		},
		{
			name: "allowed CN",
			cfg: &TLSConfig{
				AllowedClientCNs: []string{"client-1", "client-2"},
			},
			cert:        cert1,
			expectError: false,
		},
		{
			name: "not allowed CN",
			cfg: &TLSConfig{
				AllowedClientCNs: []string{"client-2", "client-3"},
			},
			cert:        cert1,
			expectError: true,
		},
		{
			name: "nil certificate",
			cfg: &TLSConfig{
				AllowedClientCNs: []string{"client-1"},
			},
			cert:        nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateClientCert(tt.cert)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestTLSAuthenticator_Authenticate(t *testing.T) {
	// Generate test certificates
	caCert, caKey := generateTestCA(t)
	clientCert, clientKey := generateTestCert(t, caCert, caKey, "test-service", []string{"service.example.com"})

	// Create TLS certificate
	tlsCert := tls.Certificate{
		Certificate: [][]byte{clientCert.Raw},
		PrivateKey:  clientKey,
	}

	tests := []struct {
		name        string
		cfg         *TLSConfig
		creds       interface{}
		expectError bool
		validate    func(*testing.T, *Principal)
	}{
		{
			name: "valid mTLS authentication",
			cfg: &TLSConfig{
				Enabled: true,
			},
			creds: &TLSCredentials{
				ClientCert: &tlsCert,
				CommonName: "test-service",
				DNSNames:   []string{"service.example.com"},
			},
			expectError: false,
			validate: func(t *testing.T, p *Principal) {
				if p.ID != "test-service" {
					t.Errorf("Expected ID 'test-service', got '%s'", p.ID)
				}
				if p.Type != PrincipalTypeService {
					t.Errorf("Expected type SERVICE, got %s", p.Type)
				}
				if p.AuthMethod != AuthMethodMTLS {
					t.Errorf("Expected auth method MTLS, got %s", p.AuthMethod)
				}
				if p.ClientCertCN != "test-service" {
					t.Errorf("Expected ClientCertCN 'test-service', got '%s'", p.ClientCertCN)
				}
				if p.ExpiresAt == nil {
					t.Error("Expected ExpiresAt to be set")
				}
			},
		},
		{
			name: "CN validation success",
			cfg: &TLSConfig{
				Enabled:          true,
				AllowedClientCNs: []string{"test-service", "other-service"},
			},
			creds: &TLSCredentials{
				ClientCert: &tlsCert,
				CommonName: "test-service",
			},
			expectError: false,
		},
		{
			name: "CN validation failure",
			cfg: &TLSConfig{
				Enabled:          true,
				AllowedClientCNs: []string{"other-service"},
			},
			creds: &TLSCredentials{
				ClientCert: &tlsCert,
				CommonName: "test-service",
			},
			expectError: true,
		},
		{
			name: "invalid credentials type",
			cfg: &TLSConfig{
				Enabled: true,
			},
			creds:       &SASLCredentials{},
			expectError: true,
		},
		{
			name: "nil client certificate",
			cfg: &TLSConfig{
				Enabled: true,
			},
			creds: &TLSCredentials{
				ClientCert: nil,
			},
			expectError: true,
		},
		{
			name: "empty certificate chain",
			cfg: &TLSConfig{
				Enabled: true,
			},
			creds: &TLSCredentials{
				ClientCert: &tls.Certificate{
					Certificate: [][]byte{},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewTLSAuthenticator(tt.cfg)
			ctx := context.Background()

			principal, err := auth.Authenticate(ctx, tt.creds)
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if principal == nil {
				t.Fatal("Expected non-nil principal")
			}

			if tt.validate != nil {
				tt.validate(t, principal)
			}
		})
	}
}

func TestGetRecommendedCipherSuites(t *testing.T) {
	suites := GetRecommendedCipherSuites()

	if len(suites) == 0 {
		t.Error("Expected non-empty cipher suites")
	}

	// Check for TLS 1.3 suites
	hasTLS13 := false
	for _, suite := range suites {
		if suite == tls.TLS_AES_128_GCM_SHA256 || suite == tls.TLS_AES_256_GCM_SHA384 {
			hasTLS13 = true
			break
		}
	}

	if !hasTLS13 {
		t.Error("Expected TLS 1.3 cipher suites")
	}
}

func TestDefaultTLSConfig(t *testing.T) {
	cfg := DefaultTLSConfig()

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	if cfg.Enabled {
		t.Error("Expected TLS to be disabled by default")
	}

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected TLS 1.2 minimum, got %x", cfg.MinVersion)
	}

	if len(cfg.CipherSuites) == 0 {
		t.Error("Expected cipher suites to be set")
	}
}

func TestTLSAuthenticator_Method(t *testing.T) {
	auth := NewTLSAuthenticator(&TLSConfig{})
	if auth.Method() != AuthMethodMTLS {
		t.Errorf("Expected method MTLS, got %s", auth.Method())
	}
}
