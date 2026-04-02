package middleware

import (
	"net/http"
	"testing"
	"time"
)

// TestApplyDefaults_SetsAllZero verifies that ApplyDefaults sets all four
// timeout fields on a zero-value http.Server to their expected defaults:
// ReadTimeout=10s, WriteTimeout=30s, IdleTimeout=120s, ReadHeaderTimeout=5s.
func TestApplyDefaults_SetsAllZero(t *testing.T) {
	srv := &http.Server{}
	ApplyDefaults(srv)

	checks := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"ReadTimeout", srv.ReadTimeout, 10 * time.Second},
		{"WriteTimeout", srv.WriteTimeout, 30 * time.Second},
		{"IdleTimeout", srv.IdleTimeout, 120 * time.Second},
		{"ReadHeaderTimeout", srv.ReadHeaderTimeout, 5 * time.Second},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestApplyDefaults_PreservesExisting verifies that ApplyDefaults does not
// overwrite timeout fields that already have non-zero values.
func TestApplyDefaults_PreservesExisting(t *testing.T) {
	srv := &http.Server{
		ReadTimeout:       99 * time.Second,
		WriteTimeout:      99 * time.Second,
		IdleTimeout:       99 * time.Second,
		ReadHeaderTimeout: 99 * time.Second,
	}
	ApplyDefaults(srv)

	checks := []struct {
		name string
		got  time.Duration
	}{
		{"ReadTimeout", srv.ReadTimeout},
		{"WriteTimeout", srv.WriteTimeout},
		{"IdleTimeout", srv.IdleTimeout},
		{"ReadHeaderTimeout", srv.ReadHeaderTimeout},
	}
	for _, c := range checks {
		if c.got != 99*time.Second {
			t.Errorf("%s = %v, want 99s (should not be overwritten)", c.name, c.got)
		}
	}
}

// TestApplyDefaults_NilServerNoOp verifies that calling ApplyDefaults with
// a nil *http.Server does not panic.
func TestApplyDefaults_NilServerNoOp(t *testing.T) {
	// Should not panic
	ApplyDefaults(nil)
}

// TestApplyDefaults_PartialOverride verifies that ApplyDefaults only sets
// defaults for zero-valued fields and leaves non-zero fields untouched,
// even when some fields are zero and others are not.
func TestApplyDefaults_PartialOverride(t *testing.T) {
	srv := &http.Server{
		ReadTimeout: 42 * time.Second,
		// WriteTimeout, IdleTimeout, ReadHeaderTimeout left as zero
	}
	ApplyDefaults(srv)

	if srv.ReadTimeout != 42*time.Second {
		t.Errorf("ReadTimeout = %v, want 42s (should be preserved)", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s (should be defaulted)", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want 120s (should be defaulted)", srv.IdleTimeout)
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 5s (should be defaulted)", srv.ReadHeaderTimeout)
	}
}
