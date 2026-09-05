package main

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gstreamio/streambus/pkg/security"
	"github.com/spf13/viper"
)

// newViper builds an isolated viper instance from a YAML fragment, so these
// tests never touch the package-global viper the real command uses.
func newViper(t *testing.T, yaml string) *viper.Viper {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(yaml)); err != nil {
		t.Fatalf("reading test config: %v", err)
	}
	return v
}

func TestParseSecurityConfig_DisabledReturnsNil(t *testing.T) {
	// A nil SecurityConfig is how the broker expresses "no security", so an
	// absent or disabled section must preserve exactly the old behaviour
	// rather than returning an empty-but-present config.
	for name, yaml := range map[string]string{
		"absent":   "server:\n  port: 9092\n",
		"disabled": "security:\n  enabled: false\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseSecurityConfig(newViper(t, yaml))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg != nil {
				t.Errorf("got %+v, want nil", cfg)
			}
		})
	}
}

func TestParseSecurityConfig_EnabledButNothingOnIsRejected(t *testing.T) {
	// The exact failure this change exists to prevent: someone switches
	// security on, the broker starts, and nothing is actually protected.
	_, err := parseSecurityConfig(newViper(t, "security:\n  enabled: true\n"))
	if err == nil {
		t.Fatal("expected an error when security is enabled with nothing switched on")
	}
	if !strings.Contains(err.Error(), "no protection") {
		t.Errorf("error %q should explain that the broker would be unprotected", err)
	}
}

func TestParseSecurityConfig_TLS(t *testing.T) {
	cfg, err := parseSecurityConfig(newViper(t, `
security:
  enabled: true
  tls:
    enabled: true
    cert_file: /certs/tls.crt
    key_file: /certs/tls.key
    ca_file: /certs/ca.crt
    require_client_cert: true
    verify_client_cert: true
    min_version: "1.3"
    allowed_client_cns: ["client-a", "client-b"]
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLS == nil {
		t.Fatal("TLS config is nil")
	}
	if !cfg.TLS.Enabled || cfg.TLS.CertFile != "/certs/tls.crt" || cfg.TLS.KeyFile != "/certs/tls.key" {
		t.Errorf("cert/key not carried through: %+v", cfg.TLS)
	}
	if cfg.TLS.CAFile != "/certs/ca.crt" || !cfg.TLS.RequireClientCert || !cfg.TLS.VerifyClientCert {
		t.Errorf("mTLS settings not carried through: %+v", cfg.TLS)
	}
	if cfg.TLS.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %#x, want TLS 1.3 (%#x)", cfg.TLS.MinVersion, tls.VersionTLS13)
	}
	if len(cfg.TLS.AllowedClientCNs) != 2 {
		t.Errorf("AllowedClientCNs = %v, want 2 entries", cfg.TLS.AllowedClientCNs)
	}
}

func TestParseSecurityConfig_TLSDefaultsToTLS12(t *testing.T) {
	// Leaving MinVersion at 0 would let the standard library choose, which on
	// an older toolchain can admit TLS 1.0. An unset value must mean 1.2.
	cfg, err := parseSecurityConfig(newViper(t, `
security:
  enabled: true
  tls:
    enabled: true
    cert_file: /certs/tls.crt
    key_file: /certs/tls.key
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TLS.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2 (%#x)", cfg.TLS.MinVersion, tls.VersionTLS12)
	}
}

func TestParseSecurityConfig_TLSRejectsIncompleteSettings(t *testing.T) {
	tests := map[string]struct{ yaml, want string }{
		"missing key file": {`
security:
  enabled: true
  tls:
    enabled: true
    cert_file: /certs/tls.crt
`, "key_file"},
		"missing cert file": {`
security:
  enabled: true
  tls:
    enabled: true
    key_file: /certs/tls.key
`, "cert_file"},
		// Requiring a client certificate with no CA cannot verify anything;
		// accepting it would authenticate on the wrong basis.
		"mTLS without a CA": {`
security:
  enabled: true
  tls:
    enabled: true
    cert_file: /certs/tls.crt
    key_file: /certs/tls.key
    require_client_cert: true
`, "ca_file"},
		"unsupported min version": {`
security:
  enabled: true
  tls:
    enabled: true
    cert_file: /certs/tls.crt
    key_file: /certs/tls.key
    min_version: "1.1"
`, "min_version"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseSecurityConfig(newViper(t, tt.yaml))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

// writeUsersDir builds a directory shaped like a mounted Kubernetes Secret:
// one file per user, named for the user, containing the password.
func writeUsersDir(t *testing.T, users map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, password := range users {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(password), 0o600); err != nil {
			t.Fatalf("writing user %q: %v", name, err)
		}
	}
	return dir
}

func TestParseSecurityConfig_SASL(t *testing.T) {
	dir := writeUsersDir(t, map[string]string{"alice": "hunter2", "bob": "s3cret"})
	cfg, err := parseSecurityConfig(newViper(t, `
security:
  enabled: true
  sasl:
    enabled: true
    mechanisms: ["SCRAM-SHA-256", "PLAIN"]
    users_dir: `+dir+`
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SASL == nil || !cfg.SASL.Enabled {
		t.Fatal("SASL not enabled")
	}
	want := []security.AuthMethod{security.AuthMethodSASLSCRAM256, security.AuthMethodSASLPlain}
	if len(cfg.SASL.Mechanisms) != len(want) {
		t.Fatalf("Mechanisms = %v, want %v", cfg.SASL.Mechanisms, want)
	}
	for i := range want {
		if cfg.SASL.Mechanisms[i] != want[i] {
			t.Errorf("Mechanisms[%d] = %q, want %q", i, cfg.SASL.Mechanisms[i], want[i])
		}
	}

	if len(cfg.SASL.Users) != 2 {
		t.Fatalf("Users = %v, want alice and bob", cfg.SASL.Users)
	}
	alice, ok := cfg.SASL.Users["alice"]
	if !ok {
		t.Fatal("user alice was not loaded")
	}
	// The plaintext must not survive into the loaded user.
	if strings.Contains(string(alice.PasswordHash), "hunter2") {
		t.Error("password stored in plaintext")
	}
	if err := security.VerifyPassword(alice, "hunter2", security.AuthMethodSASLSCRAM256); err != nil {
		t.Errorf("alice's password does not verify: %v", err)
	}
	if err := security.VerifyPassword(alice, "wrong", security.AuthMethodSASLSCRAM256); err == nil {
		t.Error("a wrong password verified successfully")
	}
}

func TestLoadSASLUsers_TrimsTrailingNewline(t *testing.T) {
	// kubectl create secret --from-file and most editors append a newline; if
	// it is kept, every password silently fails to match.
	dir := writeUsersDir(t, map[string]string{"alice": "hunter2\n"})
	users, err := loadSASLUsers(dir, security.AuthMethodSASLSCRAM256)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := security.VerifyPassword(users["alice"], "hunter2", security.AuthMethodSASLSCRAM256); err != nil {
		t.Errorf("trailing newline was not trimmed: %v", err)
	}
}

func TestLoadSASLUsers_SkipsSecretBookkeeping(t *testing.T) {
	// A projected Secret contains a ..data symlink and ..timestamp directories
	// alongside the real keys. Those must not become users.
	dir := writeUsersDir(t, map[string]string{"alice": "hunter2", "..data": "ignored"})
	users, err := loadSASLUsers(dir, security.AuthMethodSASLPlain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, found := users[".."+"data"]; found {
		t.Error("..data was loaded as a user")
	}
	if len(users) != 1 {
		t.Errorf("loaded %d users, want only alice", len(users))
	}
}

func TestLoadSASLUsers_RejectsEmptyOrMissingDir(t *testing.T) {
	if _, err := loadSASLUsers("", security.AuthMethodSASLPlain); err == nil {
		t.Error("expected an error when users_dir is unset")
	}
	if _, err := loadSASLUsers(t.TempDir(), security.AuthMethodSASLPlain); err == nil {
		t.Error("expected an error when users_dir holds no credentials")
	}
	if _, err := loadSASLUsers("/nonexistent-users-dir", security.AuthMethodSASLPlain); err == nil {
		t.Error("expected an error when users_dir does not exist")
	}
}

func TestParseSecurityConfig_SASLRejectsBadMechanisms(t *testing.T) {
	// A typo'd mechanism must fail rather than be dropped: silently ignoring
	// it would leave a broker advertising fewer mechanisms than configured.
	_, err := parseSecurityConfig(newViper(t, `
security:
  enabled: true
  sasl:
    enabled: true
    mechanisms: ["SCRAM-SHA-1"]
`))
	if err == nil {
		t.Fatal("expected an error for an unsupported mechanism")
	}
	if !strings.Contains(err.Error(), "SCRAM-SHA-1") {
		t.Errorf("error %q should name the offending mechanism", err)
	}

	_, err = parseSecurityConfig(newViper(t, `
security:
  enabled: true
  sasl:
    enabled: true
    mechanisms: []
`))
	if err == nil {
		t.Fatal("expected an error when mechanisms is empty")
	}
}

func TestParseSecurityConfig_AuthorizationAndAudit(t *testing.T) {
	cfg, err := parseSecurityConfig(newViper(t, `
security:
  enabled: true
  authz_enabled: true
  allow_anonymous: false
  use_default_acls: true
  api_key_enabled: true
  super_users: ["admin", "ops"]
  audit:
    enabled: true
    log_file: /var/log/streambus/audit.log
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AuthzEnabled || !cfg.UseDefaultACLs || !cfg.APIKeyEnabled {
		t.Errorf("authorization flags not carried through: %+v", cfg)
	}
	if cfg.AllowAnonymous {
		t.Error("AllowAnonymous should be false")
	}
	if len(cfg.SuperUsers) != 2 || cfg.SuperUsers[0] != "admin" {
		t.Errorf("SuperUsers = %v, want [admin ops]", cfg.SuperUsers)
	}
	if !cfg.AuditEnabled || cfg.AuditLogFile != "/var/log/streambus/audit.log" {
		t.Errorf("audit settings not carried through: %+v", cfg)
	}
}

func TestParseSecurityConfig_AuthzAloneIsSufficient(t *testing.T) {
	// Authorization without TLS or SASL is unusual but is a real protection,
	// so it must not trip the "no protection" guard.
	cfg, err := parseSecurityConfig(newViper(t, `
security:
  enabled: true
  authz_enabled: true
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil || !cfg.AuthzEnabled {
		t.Fatal("expected an authorization-only config to be accepted")
	}
}
