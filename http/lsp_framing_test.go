package http

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
)

// TestWriteFrame_Format verifies that WriteFrame produces the correct
// Content-Length framed format: "Content-Length: N\r\n\r\n<body>".
// This is the LSP (Language Server Protocol) framing format also used
// by MCP's stdio transport.
func TestWriteFrame_Format(t *testing.T) {
	var buf bytes.Buffer
	data := []byte(`{"jsonrpc":"2.0","id":1}`)
	if err := WriteFrame(&buf, data); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got := buf.String()
	want := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(data), data)
	if got != want {
		t.Errorf("WriteFrame output = %q, want %q", got, want)
	}
}

// TestFrameRoundTrip verifies that a message written with WriteFrame can be
// read back with ReadFrame, preserving the original bytes exactly.
func TestFrameRoundTrip(t *testing.T) {
	data := []byte(`{"method":"initialize","params":{}}`)
	var buf bytes.Buffer
	if err := WriteFrame(&buf, data); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, err := ReadFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("ReadFrame = %q, want %q", got, data)
	}
}

// TestReadFrame_MultipleHeaders verifies that ReadFrame correctly parses
// frames with multiple headers, extracting only Content-Length and ignoring
// others (per the LSP spec, only Content-Length is required).
func TestReadFrame_MultipleHeaders(t *testing.T) {
	input := "Content-Type: application/json\r\nContent-Length: 5\r\n\r\nhello"
	got, err := ReadFrame(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("ReadFrame = %q, want %q", got, "hello")
	}
}

// TestReadFrame_MalformedHeader verifies that ReadFrame returns an error
// when a header line doesn't contain a colon separator.
func TestReadFrame_MalformedHeader(t *testing.T) {
	input := "bad header\r\n\r\nhello"
	_, err := ReadFrame(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("ReadFrame should fail on malformed header")
	}
	if !strings.Contains(err.Error(), "malformed header") {
		t.Errorf("error = %q, want to contain 'malformed header'", err.Error())
	}
}

// TestReadFrame_InvalidContentLength verifies that ReadFrame returns an error
// when Content-Length is not a valid integer.
func TestReadFrame_InvalidContentLength(t *testing.T) {
	input := "Content-Length: abc\r\n\r\nhello"
	_, err := ReadFrame(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("ReadFrame should fail on invalid Content-Length")
	}
}

// TestReadFrame_MissingContentLength verifies that ReadFrame returns an error
// when no Content-Length header is present (it's required per LSP spec).
func TestReadFrame_MissingContentLength(t *testing.T) {
	input := "Content-Type: text/plain\r\n\r\nhello"
	_, err := ReadFrame(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("ReadFrame should fail when Content-Length is missing")
	}
	if !strings.Contains(err.Error(), "missing Content-Length") {
		t.Errorf("error = %q, want to contain 'missing Content-Length'", err.Error())
	}
}

// TestReadFrame_NegativeContentLength verifies that ReadFrame rejects
// negative Content-Length values.
func TestReadFrame_NegativeContentLength(t *testing.T) {
	input := "Content-Length: -1\r\n\r\n"
	_, err := ReadFrame(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("ReadFrame should fail on negative Content-Length")
	}
}

// TestReadFrame_PartialRead verifies that ReadFrame returns an error when
// the body is shorter than the Content-Length header declares.
func TestReadFrame_PartialRead(t *testing.T) {
	input := "Content-Length: 100\r\n\r\nshort"
	_, err := ReadFrame(bufio.NewReader(strings.NewReader(input)))
	if err == nil {
		t.Fatal("ReadFrame should fail on partial body")
	}
}

// TestReadFrame_EOF verifies that ReadFrame returns io.EOF when the reader
// is empty (clean stream shutdown).
func TestReadFrame_EOF(t *testing.T) {
	_, err := ReadFrame(bufio.NewReader(strings.NewReader("")))
	if err == nil {
		t.Fatal("ReadFrame should return error on empty input")
	}
	if err != io.EOF {
		t.Errorf("err = %v, want io.EOF", err)
	}
}

// TestFrameRoundTrip_Multiple verifies that multiple messages can be
// written and read in sequence from the same stream.
func TestFrameRoundTrip_Multiple(t *testing.T) {
	messages := []string{`{"id":1}`, `{"id":2}`, `{"id":3}`}
	var buf bytes.Buffer
	for _, msg := range messages {
		if err := WriteFrame(&buf, []byte(msg)); err != nil {
			t.Fatalf("WriteFrame(%s): %v", msg, err)
		}
	}
	reader := bufio.NewReader(&buf)
	for i, want := range messages {
		got, err := ReadFrame(reader)
		if err != nil {
			t.Fatalf("ReadFrame #%d: %v", i, err)
		}
		if string(got) != want {
			t.Errorf("ReadFrame #%d = %q, want %q", i, got, want)
		}
	}
}
