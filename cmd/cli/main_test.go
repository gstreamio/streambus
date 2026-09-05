package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// poisonReader panics if anything ever reads from it. It is used to prove a
// code path does not touch stdin at all - unlike an empty reader (which just
// yields EOF), a read from this one fails the test loudly instead of
// silently passing for the wrong reason.
type poisonReader struct{}

func (poisonReader) Read(p []byte) (int, error) {
	panic("unexpected read from stdin")
}

// TestConfirmDestructive_SkipBypassesPrompt covers --yes: it must return
// true without ever reading stdin, so scripting with --yes can't block on
// input that will never arrive.
func TestConfirmDestructive_SkipBypassesPrompt(t *testing.T) {
	if !confirmDestructive(poisonReader{}, "delete everything", true) {
		t.Fatal("confirmDestructive(skip=true) = false, want true")
	}
}

// TestConfirmDestructive_ReadsAnswer covers the interactive path: only an
// explicit "y"/"yes" (any case) proceeds, and anything else - including no
// input at all - is treated as declining, so a broken pipe fails safe rather
// than deleting by default.
func TestConfirmDestructive_ReadsAnswer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"lowercase y", "y\n", true},
		{"full word yes", "yes\n", true},
		{"uppercase Y", "Y\n", true},
		{"explicit no", "n\n", false},
		{"empty line", "\n", false},
		{"unrecognized text", "sure\n", false},
		{"no input at all", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := confirmDestructive(strings.NewReader(tc.input), "delete something", false)
			if got != tc.want {
				t.Errorf("confirmDestructive(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// TestTopicDeleteCmd_RequiresConfirmation runs the actual "topic delete"
// command tree against a fake admin API, declining the prompt, and asserts
// the DELETE request is never sent.
func TestTopicDeleteCmd_RequiresConfirmation(t *testing.T) {
	var deleteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"topic", "delete", "orders", "--admin-addr", addrOf(t, srv)})
	rootCmd.SetIn(strings.NewReader("n\n"))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("delete requests sent = %d, want 0 (confirmation was declined)", deleteCalls)
	}
}

// TestTopicDeleteCmd_YesBypassesConfirmation runs the same command tree with
// --yes and a poisoned stdin, asserting the DELETE request is sent without
// ever touching stdin.
func TestTopicDeleteCmd_YesBypassesConfirmation(t *testing.T) {
	var deleteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"topic", "delete", "orders", "--admin-addr", addrOf(t, srv), "--yes"})
	rootCmd.SetIn(poisonReader{})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("delete requests sent = %d, want 1 (--yes should bypass confirmation)", deleteCalls)
	}
}

// TestGroupDeleteCmd_RequiresConfirmation runs the actual "group delete"
// command tree against a fake admin API, declining the prompt, and asserts
// the DELETE request is never sent.
func TestGroupDeleteCmd_RequiresConfirmation(t *testing.T) {
	var deleteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"group", "delete", "my-group", "--admin-addr", addrOf(t, srv)})
	rootCmd.SetIn(strings.NewReader("n\n"))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("delete requests sent = %d, want 0 (confirmation was declined)", deleteCalls)
	}
}

// TestGroupDeleteCmd_YesBypassesConfirmation runs the same command tree with
// --yes and a poisoned stdin, asserting the DELETE request is sent without
// ever touching stdin.
func TestGroupDeleteCmd_YesBypassesConfirmation(t *testing.T) {
	var deleteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rootCmd.SetArgs([]string{"group", "delete", "my-group", "--admin-addr", addrOf(t, srv), "--yes"})
	rootCmd.SetIn(poisonReader{})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("delete requests sent = %d, want 1 (--yes should bypass confirmation)", deleteCalls)
	}
}
