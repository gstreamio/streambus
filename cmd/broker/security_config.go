package main

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gstreamio/streambus/pkg/security"
	"github.com/spf13/viper"
)

// parseSecurityConfig builds the broker's security configuration from the
// `security` section of the config file.
//
// Before this existed, broker.Config.Security was never populated by any code
// path, so TLS, SASL and authorization could not be switched on from a config
// file at all - the fields existed and were simply never read. Anything that
// accepted security settings (the Kubernetes operator's spec.security, most
// visibly) was therefore configuring a broker that came up fully open.
//
// Returning (nil, nil) when security is disabled is deliberate: a nil
// SecurityConfig is how the broker already expresses "no security", so an
// absent section keeps exactly the previous behaviour rather than
// constructing an empty-but-present config whose meaning would be ambiguous.
func parseSecurityConfig(v *viper.Viper) (*security.SecurityConfig, error) {
	if !v.GetBool("security.enabled") {
		return nil, nil
	}

	cfg := &security.SecurityConfig{
		AuthzEnabled:   v.GetBool("security.authz_enabled"),
		SuperUsers:     v.GetStringSlice("security.super_users"),
		AllowAnonymous: v.GetBool("security.allow_anonymous"),
		AuditEnabled:   v.GetBool("security.audit.enabled"),
		AuditLogFile:   v.GetString("security.audit.log_file"),
		APIKeyEnabled:  v.GetBool("security.api_key_enabled"),
		UseDefaultACLs: v.GetBool("security.use_default_acls"),
	}

	tlsCfg, err := parseTLSConfig(v)
	if err != nil {
		return nil, err
	}
	cfg.TLS = tlsCfg

	saslCfg, err := parseSASLConfig(v)
	if err != nil {
		return nil, err
	}
	cfg.SASL = saslCfg

	// Security enabled with nothing actually switched on is almost certainly a
	// misconfiguration, and it is the failure mode this whole change exists to
	// prevent: an operator sets security.enabled and believes the broker is
	// protected. Refuse rather than start up looking secure.
	if cfg.TLS == nil && cfg.SASL == nil && !cfg.AuthzEnabled {
		return nil, fmt.Errorf(
			"security.enabled is set but none of security.tls.enabled, " +
				"security.sasl.enabled or security.authz_enabled are - the broker " +
				"would start with no protection")
	}

	return cfg, nil
}

// parseTLSConfig reads the security.tls section. It returns nil when TLS is
// not enabled, so callers can treat nil as "no TLS" without a second flag.
func parseTLSConfig(v *viper.Viper) (*security.TLSConfig, error) {
	if !v.GetBool("security.tls.enabled") {
		return nil, nil
	}

	certFile := v.GetString("security.tls.cert_file")
	keyFile := v.GetString("security.tls.key_file")
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf(
			"security.tls.enabled is set but security.tls.cert_file and " +
				"security.tls.key_file are both required")
	}

	minVersion, err := parseTLSMinVersion(v.GetString("security.tls.min_version"))
	if err != nil {
		return nil, err
	}

	requireClientCert := v.GetBool("security.tls.require_client_cert")
	caFile := v.GetString("security.tls.ca_file")

	// mTLS without a CA cannot verify anything it is asked to require, so it
	// would silently accept or reject on the wrong basis. Fail instead.
	if requireClientCert && caFile == "" {
		return nil, fmt.Errorf(
			"security.tls.require_client_cert is set but security.tls.ca_file " +
				"is empty - client certificates cannot be verified without a CA")
	}

	return &security.TLSConfig{
		Enabled:           true,
		CertFile:          certFile,
		KeyFile:           keyFile,
		CAFile:            caFile,
		RequireClientCert: requireClientCert,
		VerifyClientCert:  v.GetBool("security.tls.verify_client_cert"),
		AllowedClientCNs:  v.GetStringSlice("security.tls.allowed_client_cns"),
		MinVersion:        minVersion,
	}, nil
}

// parseTLSMinVersion maps a human-written version to the tls package's
// constant. An empty value means TLS 1.2, matching Go's own default for
// servers rather than leaving MinVersion at 0, which would let the standard
// library pick and could admit TLS 1.0 on older toolchains.
func parseTLSMinVersion(s string) (uint16, error) {
	switch s {
	case "":
		return tls.VersionTLS12, nil
	case "1.2", "TLS1.2", "VersionTLS12":
		return tls.VersionTLS12, nil
	case "1.3", "TLS1.3", "VersionTLS13":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf(
			"security.tls.min_version %q is not supported (use \"1.2\" or \"1.3\")", s)
	}
}

// saslMechanisms maps the mechanism names an operator would write in config
// onto the AuthMethod values the security package uses internally. The config
// spelling follows the usual SASL naming (SCRAM-SHA-256) rather than the
// internal constant (SASL_SCRAM_SHA256), since that is what a person setting
// this up will expect to write.
var saslMechanisms = map[string]security.AuthMethod{
	"PLAIN":         security.AuthMethodSASLPlain,
	"SCRAM-SHA-256": security.AuthMethodSASLSCRAM256,
	"SCRAM-SHA-512": security.AuthMethodSASLSCRAM512,
}

// parseSASLConfig reads the security.sasl section, returning nil when SASL is
// not enabled.
func parseSASLConfig(v *viper.Viper) (*security.SASLConfig, error) {
	if !v.GetBool("security.sasl.enabled") {
		return nil, nil
	}

	names := v.GetStringSlice("security.sasl.mechanisms")
	if len(names) == 0 {
		return nil, fmt.Errorf(
			"security.sasl.enabled is set but security.sasl.mechanisms is empty")
	}

	mechanisms := make([]security.AuthMethod, 0, len(names))
	for _, name := range names {
		mechanism, ok := saslMechanisms[name]
		if !ok {
			return nil, fmt.Errorf(
				"security.sasl.mechanisms contains unsupported mechanism %q "+
					"(supported: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)", name)
		}
		mechanisms = append(mechanisms, mechanism)
	}

	users, err := loadSASLUsers(v.GetString("security.sasl.users_dir"), mechanisms[0])
	if err != nil {
		return nil, err
	}

	return &security.SASLConfig{
		Enabled:    true,
		Mechanisms: mechanisms,
		Users:      users,
	}, nil
}

// loadSASLUsers reads credentials from a directory of one-file-per-user, which
// is how a mounted Kubernetes Secret presents itself: each key becomes a file
// named for the user, containing that user's password.
//
// Without this, enabling SASL would produce an authenticator with no users at
// all. That fails closed rather than open, so it is not a security hole - but
// it is indistinguishable from a broken deployment, and an operator who
// mounted a credentials Secret would have no way to tell that nothing read it.
//
// Passwords are hashed by security.CreateUser at load time; the plaintext is
// never retained beyond building the user. Users are created for mechanism,
// the first configured SASL mechanism, because a SCRAM verifier is derived
// from the password for one specific hash and cannot serve another.
func loadSASLUsers(dir string, mechanism security.AuthMethod) (map[string]*security.User, error) {
	if dir == "" {
		return nil, fmt.Errorf(
			"security.sasl.enabled is set but security.sasl.users_dir is empty - " +
				"the broker would start with SASL advertised and no credentials to accept")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading security.sasl.users_dir %q: %w", dir, err)
	}

	users := make(map[string]*security.User)
	for _, entry := range entries {
		// A mounted Secret keeps its real files behind a ..data symlink and
		// hides the bookkeeping in dot-prefixed entries; skip those and any
		// nested directory rather than inventing users named "..data".
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "..") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		password, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 -- path is an operator-supplied config directory, not user input
		if err != nil {
			return nil, fmt.Errorf("reading SASL credentials for %q: %w", entry.Name(), err)
		}

		// Trailing newlines are near-universal in files written by hand or by
		// `kubectl create secret --from-file`, and would otherwise become part
		// of the password.
		user, err := security.CreateUser(entry.Name(), strings.TrimRight(string(password), "\r\n"), mechanism, nil)
		if err != nil {
			return nil, fmt.Errorf("creating SASL user %q: %w", entry.Name(), err)
		}
		users[entry.Name()] = user
	}

	if len(users) == 0 {
		return nil, fmt.Errorf(
			"security.sasl.users_dir %q contains no credentials - the broker "+
				"would advertise SASL and reject every client", dir)
	}

	return users, nil
}
