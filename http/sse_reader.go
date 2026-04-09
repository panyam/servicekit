package http

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// ============================================================================
// SSE Event Reader
// ============================================================================
//
// Implements the client-side parsing algorithm from the WHATWG Server-Sent
// Events specification:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
//
// This is the read-side counterpart to BaseSSEConn (write-side) and SSEHub
// (connection management). It parses an SSE byte stream into discrete events,
// handling all spec-defined fields: event, data, id, retry, and comments.

// SSEReadEvent represents a single parsed event from an SSE stream.
//
// Fields follow the WHATWG Server-Sent Events spec:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#concept-event-stream-event-type
//
//   - Event: the "event:" field (empty = unnamed, defaults to "message" on client side)
//   - Data: the "data:" field(s), joined with "\n" for multi-line data
//   - ID: the "id:" field (empty = not set)
//   - Retry: the "retry:" field in milliseconds (0 = not set or invalid)
//   - Comment: text from comment lines (lines starting with ":"), last value kept
type SSEReadEvent struct {
	Event   string // "event:" field value
	Data    string // "data:" field(s), joined with "\n" for multi-line
	ID      string // "id:" field value
	Retry   int    // "retry:" field value in ms (0 = not set)
	Comment string // Comment text (lines starting with ":")
}

// SSEEventReader reads Server-Sent Events from an io.Reader.
//
// Usage:
//
//	reader := NewSSEEventReader(resp.Body)
//	for {
//	    ev, err := reader.ReadEvent()
//	    if err != nil {
//	        break
//	    }
//	    // process ev
//	}
//
// The reader handles all WHATWG SSE spec details: field parsing, multi-line
// data concatenation, comment skipping, retry parsing, and BOM stripping.
//
// See: https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
type SSEEventReader struct {
	reader  *bufio.Reader
	bomDone bool // whether we've checked for the leading BOM
}

// NewSSEEventReader creates an SSEEventReader that parses SSE events from r.
func NewSSEEventReader(r io.Reader) *SSEEventReader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &SSEEventReader{reader: br}
}

// ReadEvent reads the next SSE event from the stream.
//
// It blocks until a complete event (terminated by a blank line) is available,
// or an error occurs. Empty events (no fields accumulated between blank lines)
// are skipped automatically per the WHATWG dispatch algorithm:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage
//
// On EOF mid-event (fields accumulated but no trailing blank line), the
// partial event is returned along with io.EOF. This supports servers that
// close the response body without a trailing blank line.
//
// On EOF with no fields accumulated, returns a zero SSEReadEvent and io.EOF.
func (r *SSEEventReader) ReadEvent() (SSEReadEvent, error) {
	// Strip leading UTF-8 BOM if present (first call only).
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
	// "If the stream starts with a UTF-8 Byte Order Mark (U+FEFF), it must be ignored."
	if !r.bomDone {
		r.bomDone = true
		b, err := r.reader.Peek(3)
		if err == nil && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
			r.reader.Discard(3)
		}
	}

	var ev sseReadState
	for {
		line, err := r.reader.ReadString('\n')

		// ReadString may return data AND an error (typically io.EOF for the
		// last line without a trailing newline). Process the line first if
		// non-empty, then handle the error.
		if line == "" && err != nil {
			if ev.hasFields() {
				return ev.finish(), err
			}
			return SSEReadEvent{}, err
		}

		// Strip line ending (LF or CRLF).
		// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
		// "Lines must be separated by either a U+000D CARRIAGE RETURN U+000A
		// LINE FEED (CRLF) character pair, a single U+000A LINE FEED (LF)
		// character, or a single U+000D CARRIAGE RETURN (CR) character."
		line = strings.TrimRight(line, "\r\n")

		// Blank line → dispatch event.
		if line == "" {
			if ev.hasFields() {
				return ev.finish(), nil
			}
			// No fields accumulated — skip (per spec, don't dispatch empty events).
			continue
		}

		// Comment line.
		// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
		// "If the line starts with a U+003A COLON character (:) — ignore the line."
		// We capture the comment text for callers that need to assert on keepalives.
		if line[0] == ':' {
			text := line[1:]
			if len(text) > 0 && text[0] == ' ' {
				text = text[1:] // strip single leading space
			}
			ev.comment = text
			ev.touched = true
			continue
		}

		// Parse field name and value.
		// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
		// "If the line contains a U+003A COLON character (:) — Collect the
		// characters up to but not including the first colon as the field,
		// and the rest after the colon as the value."
		// "If the line does not contain a colon — process the field using the
		// whole line as the field name, and an empty string as the field value."
		field, value := line, ""
		if i := strings.IndexByte(line, ':'); i >= 0 {
			field = line[:i]
			value = line[i+1:]
			// "If value starts with a U+0020 SPACE character, remove it from value."
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
		}

		ev.touched = true
		switch field {
		case "event":
			ev.event = value
		case "data":
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			// "Append the field value to the data buffer, then append a single
			// U+000A LINE FEED character to the data buffer."
			ev.dataLines = append(ev.dataLines, value)
		case "id":
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			// "If the field value does not contain U+0000 NULL, then set the
			// last event ID buffer to the field value. Otherwise, ignore the field."
			if !strings.ContainsRune(value, '\x00') {
				ev.id = value
			}
		case "retry":
			// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
			// "If the field value consists of only ASCII digits, then [...] set
			// the event stream's reconnection time to that integer. Otherwise,
			// ignore the field."
			if n, err := strconv.Atoi(value); err == nil && n >= 0 && isAllDigits(value) {
				ev.retry = n
			}
		default:
			// Unknown field — ignore per spec.
		}

		// If ReadString returned an error alongside data (e.g., EOF on the
		// last line with no trailing newline), return accumulated state now.
		if err != nil {
			if ev.hasFields() {
				return ev.finish(), err
			}
			return SSEReadEvent{}, err
		}
	}
}

// sseReadState accumulates fields while parsing a single event.
type sseReadState struct {
	event     string
	dataLines []string
	id        string
	retry     int
	comment   string
	touched   bool // true if any line was processed for this event
}

func (s *sseReadState) hasFields() bool {
	return s.touched
}

// finish builds the final SSEReadEvent from accumulated state.
// Multi-line data is joined with "\n" per the WHATWG spec.
func (s *sseReadState) finish() SSEReadEvent {
	return SSEReadEvent{
		Event:   s.event,
		Data:    strings.Join(s.dataLines, "\n"),
		ID:      s.id,
		Retry:   s.retry,
		Comment: s.comment,
	}
}

// isAllDigits reports whether s is non-empty and consists entirely of ASCII
// digits. Used for retry field validation per the WHATWG spec.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
