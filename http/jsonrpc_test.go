package http

import (
	"testing"
)

// TestDetectBatchArray verifies that DetectBatch returns true for a JSON
// array payload, which indicates a JSON-RPC 2.0 batch request.
func TestDetectBatchArray(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"array", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, true},
		{"array with whitespace", `  [{"jsonrpc":"2.0","id":1,"method":"ping"}]`, true},
		{"object", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, false},
		{"object with whitespace", `  {"jsonrpc":"2.0","id":1,"method":"ping"}`, false},
		{"empty", ``, false},
		{"whitespace only", `   `, false},
		{"empty array", `[]`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectBatch([]byte(tc.body))
			if got != tc.want {
				t.Errorf("DetectBatch(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// TestSplitBatchValid verifies that SplitBatch correctly splits a JSON
// array into individual raw messages that can be independently parsed.
func TestSplitBatchValid(t *testing.T) {
	body := `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","id":2,"method":"tools/list"}]`
	parts, err := SplitBatch([]byte(body))
	if err != nil {
		t.Fatalf("SplitBatch returned error: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
}

// TestSplitBatchEmpty verifies that an empty JSON array returns an empty
// slice without error.
func TestSplitBatchEmpty(t *testing.T) {
	parts, err := SplitBatch([]byte(`[]`))
	if err != nil {
		t.Fatalf("SplitBatch returned error: %v", err)
	}
	if len(parts) != 0 {
		t.Errorf("expected 0 parts for empty array, got %d", len(parts))
	}
}

// TestSplitBatchInvalid verifies that SplitBatch returns an error for
// non-array JSON input.
func TestSplitBatchInvalid(t *testing.T) {
	_, err := SplitBatch([]byte(`{"jsonrpc":"2.0"}`))
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

// TestSplitBatchMalformed verifies that SplitBatch returns an error for
// malformed JSON.
func TestSplitBatchMalformed(t *testing.T) {
	_, err := SplitBatch([]byte(`[{broken`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// TestSplitBatchMixedTypes verifies that SplitBatch handles arrays
// containing mixed JSON-RPC message types (requests and notifications).
func TestSplitBatchMixedTypes(t *testing.T) {
	body := `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"}]`
	parts, err := SplitBatch([]byte(body))
	if err != nil {
		t.Fatalf("SplitBatch returned error: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
}
