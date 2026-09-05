package main

import (
	"errors"
	"testing"

	"github.com/gstreamio/streambus/pkg/storage"
	"github.com/spf13/viper"
)

// setMessageFormatVersion sets storage.message_format_version for the
// duration of the calling test and restores it afterward, so tests can run
// in any order against viper's process-global config.
func setMessageFormatVersion(t *testing.T, value string) {
	t.Helper()

	previous := viper.GetString("storage.message_format_version")
	viper.Set("storage.message_format_version", value)
	t.Cleanup(func() { viper.Set("storage.message_format_version", previous) })
}

// TestResolveMessageFormatVersion_Unset verifies that leaving
// storage.message_format_version unset - the state of every deployment that
// predates this setting - resolves to storage.MessageFormatUnset, which
// storage.Config treats as "keep writing the default (v3)". Nobody should be
// able to regress this to v2 by accident just by not setting the key.
func TestResolveMessageFormatVersion_Unset(t *testing.T) {
	setMessageFormatVersion(t, "")

	got, err := resolveMessageFormatVersion()
	if err != nil {
		t.Fatalf("resolveMessageFormatVersion failed: %v", err)
	}
	if got != storage.MessageFormatUnset {
		t.Errorf("resolveMessageFormatVersion() = %v, want MessageFormatUnset", got)
	}
}

// TestResolveMessageFormatVersion_Valid covers both real write versions an
// operator can pin the broker to.
func TestResolveMessageFormatVersion_Valid(t *testing.T) {
	tests := []struct {
		configured string
		want       storage.MessageFormatVersion
	}{
		{"v2", storage.MessageFormatV2},
		{"v3", storage.MessageFormatV3},
	}

	for _, tt := range tests {
		setMessageFormatVersion(t, tt.configured)

		got, err := resolveMessageFormatVersion()
		if err != nil {
			t.Errorf("resolveMessageFormatVersion() with %q configured failed: %v", tt.configured, err)
			continue
		}
		if got != tt.want {
			t.Errorf("resolveMessageFormatVersion() with %q configured = %v, want %v", tt.configured, got, tt.want)
		}
	}
}

// TestResolveMessageFormatVersion_Invalid is the write-gate's startup half:
// a typo in storage.message_format_version must fail broker startup with a
// clear error, not be silently treated as the default.
func TestResolveMessageFormatVersion_Invalid(t *testing.T) {
	setMessageFormatVersion(t, "v4")

	_, err := resolveMessageFormatVersion()
	if err == nil {
		t.Fatal("resolveMessageFormatVersion() with an invalid version succeeded, want an error")
	}
	if !errors.Is(err, storage.ErrInvalidMessageFormatVersion) {
		t.Errorf("resolveMessageFormatVersion() error = %v, want it to wrap storage.ErrInvalidMessageFormatVersion", err)
	}
}

// TestDescribeMessageFormatVersion documents the startup log line an
// operator actually reads: MessageFormatUnset must spell out what it
// resolves to (v3) rather than printing the ambiguous "unset".
func TestDescribeMessageFormatVersion(t *testing.T) {
	tests := []struct {
		version storage.MessageFormatVersion
		want    string
	}{
		{storage.MessageFormatUnset, "v3 (default)"},
		{storage.MessageFormatV2, "v2"},
		{storage.MessageFormatV3, "v3"},
	}

	for _, tt := range tests {
		if got := describeMessageFormatVersion(tt.version); got != tt.want {
			t.Errorf("describeMessageFormatVersion(%v) = %q, want %q", tt.version, got, tt.want)
		}
	}
}
