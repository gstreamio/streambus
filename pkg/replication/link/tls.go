package link

import (
	"github.com/gstreamio/streambus/pkg/client"
)

// buildClientSecurityConfig translates a replication link's cluster security
// settings into the client security configuration used to dial that
// cluster. It returns nil when the cluster config has no security section
// or TLS is not enabled there, so callers can assign the result straight to
// client.Config.Security without a nil check of their own.
//
// Certificate verification is on by default: TLSSkipVerify must be set
// explicitly to disable it, and this function never does so on its own.
// The client's connection pool (pkg/client/pool.go) is what actually loads
// TLSCAFile/TLSCertFile/TLSKeyFile and dials with the result; this function
// only carries the operator's configuration across the package boundary
// unchanged.
func buildClientSecurityConfig(sec *SecurityConfig) *client.SecurityConfig {
	if sec == nil || !sec.EnableTLS {
		return nil
	}

	return &client.SecurityConfig{
		TLS: &client.TLSConfig{
			Enabled:            true,
			CertFile:           sec.TLSCertFile,
			KeyFile:            sec.TLSKeyFile,
			CAFile:             sec.TLSCAFile,
			InsecureSkipVerify: sec.TLSSkipVerify,
			ServerName:         sec.TLSServerName,
		},
	}
}
