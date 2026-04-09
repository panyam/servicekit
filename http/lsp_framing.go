package http

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// WriteFrame writes a Content-Length framed message in LSP format:
//
//	Content-Length: <n>\r\n\r\n<body>
//
// This framing is used by the Language Server Protocol (LSP) and MCP's stdio
// transport to delimit JSON-RPC messages over byte streams (pipes, stdin/stdout).
//
// See: https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#headerPart
func WriteFrame(w io.Writer, data []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// ReadFrame reads a Content-Length framed message from a buffered reader.
// It parses HTTP-style headers until an empty line (\r\n\r\n), extracts the
// Content-Length value, and reads exactly that many bytes as the message body.
//
// Headers other than Content-Length are accepted but ignored, matching the
// LSP spec which allows Content-Type as an optional header.
//
// Returns io.EOF if the reader is empty (clean stream shutdown).
//
// See: https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#headerPart
func ReadFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed header: %q", line)
		}
		name := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if strings.EqualFold(name, "Content-Length") {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", value, err)
			}
			if n < 0 {
				return nil, fmt.Errorf("negative Content-Length: %d", n)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("reading body (%d bytes): %w", contentLength, err)
	}

	return body, nil
}
