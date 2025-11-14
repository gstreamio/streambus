package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// NewTLSConfig creates a TLS configuration from the security config
func NewTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: cfg.MinVersion,
	}

	// Set default to TLS 1.2 if not specified
	if tlsConfig.MinVersion == 0 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}

	// Load server certificate and key
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate for client verification
	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.ClientCAs = caCertPool
	}

	// Configure client certificate requirements
	if cfg.RequireClientCert {
		if cfg.VerifyClientCert {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = tls.RequireAnyClientCert
		}
	} else if cfg.VerifyClientCert {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	} else {
		tlsConfig.ClientAuth = tls.NoClientCert
	}

	// Set cipher suites if specified
	if len(cfg.CipherSuites) > 0 {
		tlsConfig.CipherSuites = cfg.CipherSuites
	}

	// Note: PreferServerCipherSuites is deprecated since Go 1.18 and is now ignored
	// The server's cipher suite preference is always used when the client supports it

	cfg.config = tlsConfig
	return tlsConfig, nil
}

// GetTLSConfig returns the internal TLS config
func (cfg *TLSConfig) GetTLSConfig() *tls.Config {
	return cfg.config
}

// ValidateClientCert validates a client certificate
func (cfg *TLSConfig) ValidateClientCert(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("no client certificate provided")
	}

	// Check if CN is in allowed list
	if len(cfg.AllowedClientCNs) > 0 {
		allowed := false
		for _, cn := range cfg.AllowedClientCNs {
			if cert.Subject.CommonName == cn {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("client certificate CN '%s' not in allowed list", cert.Subject.CommonName)
		}
	}

	return nil
}

// GetRecommendedCipherSuites returns secure cipher suites for TLS 1.2
func GetRecommendedCipherSuites() []uint16 {
	return []uint16{
		// TLS 1.3 suites (automatically used if TLS 1.3)
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,

		// TLS 1.2 suites
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	}
}

// DefaultTLSConfig returns a secure default TLS configuration
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		Enabled:           false,
		RequireClientCert: false,
		VerifyClientCert:  false,
		MinVersion:        tls.VersionTLS12,
		CipherSuites:      GetRecommendedCipherSuites(),
	}
}

// TLSAuthenticator authenticates using client certificates
type TLSAuthenticator struct {
	tlsConfig *TLSConfig
}

// NewTLSAuthenticator creates a new TLS authenticator
func NewTLSAuthenticator(cfg *TLSConfig) *TLSAuthenticator {
	return &TLSAuthenticator{
		tlsConfig: cfg,
	}
}

// Authenticate authenticates using TLS client certificate
func (a *TLSAuthenticator) Authenticate(ctx context.Context, credentials interface{}) (*Principal, error) {
	tlsCreds, ok := credentials.(*TLSCredentials)
	if !ok {
		return nil, fmt.Errorf("invalid credentials type for TLS authentication")
	}

	if tlsCreds.ClientCert == nil {
		return nil, fmt.Errorf("no client certificate provided")
	}

	// Validate the certificate
	if len(tlsCreds.ClientCert.Certificate) == 0 {
		return nil, fmt.Errorf("invalid client certificate")
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(tlsCreds.ClientCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse client certificate: %w", err)
	}

	// Validate against allowed CNs
	if err := a.tlsConfig.ValidateClientCert(cert); err != nil {
		return nil, err
	}

	// Create principal from certificate
	principal := &Principal{
		ID:           cert.Subject.CommonName,
		Type:         PrincipalTypeService, // Assume service for mTLS
		AuthMethod:   AuthMethodMTLS,
		Attributes:   make(map[string]string),
		AuthTime:     cert.NotBefore,
		ClientCertCN: cert.Subject.CommonName,
	}

	// Add certificate details to attributes
	principal.Attributes["cert_subject"] = cert.Subject.String()
	principal.Attributes["cert_issuer"] = cert.Issuer.String()
	if len(cert.DNSNames) > 0 {
		principal.Attributes["cert_dns_names"] = fmt.Sprintf("%v", cert.DNSNames)
	}

	// Set expiration from certificate
	expiresAt := cert.NotAfter
	principal.ExpiresAt = &expiresAt

	return principal, nil
}

// Method returns the authentication method
func (a *TLSAuthenticator) Method() AuthMethod {
	return AuthMethodMTLS
}
