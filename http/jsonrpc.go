package http

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// DetectBatch returns true if body is a JSON array, indicating a JSON-RPC
// 2.0 batch request. Skips leading whitespace before checking the first
// non-whitespace byte for '['.
//
// Per JSON-RPC 2.0 spec (Section 6): "To send several Request objects at
// the same time, the Client MAY send an Array filled with Request objects."
func DetectBatch(body []byte) bool {
	body = bytes.TrimLeft(body, " \t\r\n")
	return len(body) > 0 && body[0] == '['
}

// SplitBatch splits a JSON array into individual raw JSON messages.
// Returns an error if the body is not a valid JSON array.
//
// Each element in the returned slice is a complete JSON value (object or
// otherwise) that can be independently unmarshaled as a JSON-RPC request.
func SplitBatch(body []byte) ([]json.RawMessage, error) {
	var batch []json.RawMessage
	if err := json.Unmarshal(body, &batch); err != nil {
		return nil, fmt.Errorf("invalid JSON-RPC batch: %w", err)
	}
	return batch, nil
}
