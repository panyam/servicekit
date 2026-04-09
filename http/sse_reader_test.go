package http

import (
	"io"
	"strings"
	"testing"
)

// SSE Event Reader tests
//
// These tests verify SSEEventReader against the WHATWG Server-Sent Events spec:
// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
//
// Section references use anchors from the spec's "Parsing an event stream" algorithm.

func TestReadEvent_Basic(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// An event with both "event:" and "data:" fields, terminated by a blank line.
	r := NewSSEEventReader(strings.NewReader("event: foo\ndata: bar\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "foo" {
		t.Errorf("Event = %q, want %q", ev.Event, "foo")
	}
	if ev.Data != "bar" {
		t.Errorf("Data = %q, want %q", ev.Data, "bar")
	}
}

func TestReadEvent_DataOnly(t *testing.T) {
	// An event with only a "data:" field — event type defaults to "message" on
	// the client side per spec, but the reader just returns an empty Event field.
	r := NewSSEEventReader(strings.NewReader("data: hello\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "" {
		t.Errorf("Event = %q, want empty", ev.Event)
	}
	if ev.Data != "hello" {
		t.Errorf("Data = %q, want %q", ev.Data, "hello")
	}
}

func TestReadEvent_MultiLineData(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the field name is 'data': [...] append the field value to the data buffer,
	// then append a single U+000A LINE FEED character to the data buffer."
	//
	// Multiple "data:" lines within one event are concatenated with "\n".
	input := "data: line1\ndata: line2\ndata: line3\n\n"
	r := NewSSEEventReader(strings.NewReader(input))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "line1\nline2\nline3" {
		t.Errorf("Data = %q, want %q", ev.Data, "line1\nline2\nline3")
	}
}

func TestReadEvent_CommentOnly(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the line starts with a U+003A COLON character: [...] ignore the line."
	//
	// Comments are typically used as keepalives. We surface them so callers can
	// assert on keepalive behavior if needed. A comment followed by a blank line
	// produces a comment-only event.
	r := NewSSEEventReader(strings.NewReader(": keepalive\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Comment != "keepalive" {
		t.Errorf("Comment = %q, want %q", ev.Comment, "keepalive")
	}
	if ev.Data != "" {
		t.Errorf("Data = %q, want empty", ev.Data)
	}
}

func TestReadEvent_IDField(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the field name is 'id': [...] set the last event ID buffer to the field value."
	r := NewSSEEventReader(strings.NewReader("id: 42\ndata: x\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID != "42" {
		t.Errorf("ID = %q, want %q", ev.ID, "42")
	}
	if ev.Data != "x" {
		t.Errorf("Data = %q, want %q", ev.Data, "x")
	}
}

func TestReadEvent_RetryField(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the field name is 'retry': If the field value consists of only ASCII digits,
	// then [...] set the event stream's reconnection time to that integer."
	r := NewSSEEventReader(strings.NewReader("retry: 3000\ndata: x\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Retry != 3000 {
		t.Errorf("Retry = %d, want %d", ev.Retry, 3000)
	}
	if ev.Data != "x" {
		t.Errorf("Data = %q, want %q", ev.Data, "x")
	}
}

func TestReadEvent_RetryInvalid(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the field value does not consist of only ASCII digits, then ignore the field."
	r := NewSSEEventReader(strings.NewReader("retry: abc\ndata: x\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Retry != 0 {
		t.Errorf("Retry = %d, want 0 (invalid retry should be ignored)", ev.Retry)
	}
	if ev.Data != "x" {
		t.Errorf("Data = %q, want %q", ev.Data, "x")
	}
}

func TestReadEvent_MultipleEvents(t *testing.T) {
	// Two events separated by blank lines. Each ReadEvent() call returns one event.
	input := "event: first\ndata: 1\n\nevent: second\ndata: 2\n\n"
	r := NewSSEEventReader(strings.NewReader(input))

	ev1, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("event 1: unexpected error: %v", err)
	}
	if ev1.Event != "first" || ev1.Data != "1" {
		t.Errorf("event 1 = {%q, %q}, want {%q, %q}", ev1.Event, ev1.Data, "first", "1")
	}

	ev2, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("event 2: unexpected error: %v", err)
	}
	if ev2.Event != "second" || ev2.Data != "2" {
		t.Errorf("event 2 = {%q, %q}, want {%q, %q}", ev2.Event, ev2.Data, "second", "2")
	}
}

func TestReadEvent_CommentsBeforeData(t *testing.T) {
	// Comments within an event (before/between fields) are absorbed into the event.
	// The comment text is captured; data fields are parsed normally.
	input := ": comment\nevent: e\ndata: d\n\n"
	r := NewSSEEventReader(strings.NewReader(input))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "e" {
		t.Errorf("Event = %q, want %q", ev.Event, "e")
	}
	if ev.Data != "d" {
		t.Errorf("Data = %q, want %q", ev.Data, "d")
	}
}

func TestReadEvent_EmptyDataField(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "data:" with no value — the data buffer gets an empty string appended then "\n".
	// After trimming the trailing "\n" per the dispatch step, data is "".
	r := NewSSEEventReader(strings.NewReader("data:\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "" {
		t.Errorf("Data = %q, want empty", ev.Data)
	}
}

func TestReadEvent_EOFMidEvent(t *testing.T) {
	// EOF before a blank line terminator. The reader returns any accumulated fields
	// along with io.EOF, so callers (like readSSEResponse) can use the last event
	// even when the server closes without a trailing blank line.
	r := NewSSEEventReader(strings.NewReader("data: partial"))
	ev, err := r.ReadEvent()
	if err != io.EOF {
		t.Fatalf("err = %v, want io.EOF", err)
	}
	if ev.Data != "partial" {
		t.Errorf("Data = %q, want %q", ev.Data, "partial")
	}
}

func TestReadEvent_EOFAfterEvent(t *testing.T) {
	// Complete event followed by EOF. First call returns the event; second returns EOF.
	r := NewSSEEventReader(strings.NewReader("data: x\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("event: unexpected error: %v", err)
	}
	if ev.Data != "x" {
		t.Errorf("Data = %q, want %q", ev.Data, "x")
	}

	_, err = r.ReadEvent()
	if err != io.EOF {
		t.Fatalf("second call: err = %v, want io.EOF", err)
	}
}

func TestReadEvent_AllFields(t *testing.T) {
	// An event with all four spec fields set.
	input := "event: e\nid: 1\nretry: 5000\ndata: d\n\n"
	r := NewSSEEventReader(strings.NewReader(input))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "e" {
		t.Errorf("Event = %q, want %q", ev.Event, "e")
	}
	if ev.ID != "1" {
		t.Errorf("ID = %q, want %q", ev.ID, "1")
	}
	if ev.Retry != 5000 {
		t.Errorf("Retry = %d, want %d", ev.Retry, 5000)
	}
	if ev.Data != "d" {
		t.Errorf("Data = %q, want %q", ev.Data, "d")
	}
}

func TestReadEvent_CRLFLineEndings(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
	// "Lines must be separated by either a U+000D CARRIAGE RETURN U+000A LINE FEED
	// (CRLF) character pair, a single U+000A LINE FEED (LF) character, or a single
	// U+000D CARRIAGE RETURN (CR) character."
	r := NewSSEEventReader(strings.NewReader("data: hello\r\n\r\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "hello" {
		t.Errorf("Data = %q, want %q", ev.Data, "hello")
	}
}

func TestReadEvent_FieldWithNoValue(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the line contains a U+003A COLON character: [...] If there is a value,
	// then [process it]. If the value is empty, process the field with an empty string."
	r := NewSSEEventReader(strings.NewReader("event:\ndata:\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Event != "" {
		t.Errorf("Event = %q, want empty", ev.Event)
	}
}

func TestReadEvent_SpaceAfterColon(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If value starts with a U+0020 SPACE character, remove it from value."
	// Only ONE leading space is stripped. "data:  foo" → " foo" (one space remains).
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with space", "data: hello\n\n", "hello"},
		{"without space", "data:hello\n\n", "hello"},
		{"double space", "data:  hello\n\n", " hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewSSEEventReader(strings.NewReader(tt.input))
			ev, err := r.ReadEvent()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Data != tt.want {
				t.Errorf("Data = %q, want %q", ev.Data, tt.want)
			}
		})
	}
}

func TestReadEvent_UnknownFieldIgnored(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "Otherwise: The field is ignored."
	// Unknown field names are silently ignored per spec.
	r := NewSSEEventReader(strings.NewReader("foo: bar\ndata: x\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "x" {
		t.Errorf("Data = %q, want %q", ev.Data, "x")
	}
}

func TestReadEvent_IDWithNullIgnored(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the field value does not contain U+0000 NULL, then set the last event
	// ID buffer to the field value. Otherwise, ignore the field."
	r := NewSSEEventReader(strings.NewReader("id: ab\x00cd\ndata: x\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID != "" {
		t.Errorf("ID = %q, want empty (null in id should be ignored)", ev.ID)
	}
}

func TestReadEvent_MultipleComments(t *testing.T) {
	// Multiple comment lines within a single event. The last comment text is kept.
	r := NewSSEEventReader(strings.NewReader(": first\n: second\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Comment != "second" {
		t.Errorf("Comment = %q, want %q", ev.Comment, "second")
	}
}

func TestReadEvent_ConsecutiveBlankLines(t *testing.T) {
	// Consecutive blank lines between events. Empty events (no fields accumulated)
	// should be skipped per spec — ReadEvent blocks until a non-empty event.
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#dispatchMessage
	// "If the data buffer is an empty string, set the data buffer and event type
	// buffer to the empty string and return." — i.e., don't dispatch.
	//
	// However, we DO return comment-only events so callers can assert keepalives.
	// Pure blank lines (no fields at all) produce nothing.
	r := NewSSEEventReader(strings.NewReader("\n\n\ndata: found\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "found" {
		t.Errorf("Data = %q, want %q", ev.Data, "found")
	}
}

func TestReadEvent_BOMPrefix(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
	// "If the stream starts with a UTF-8 BOM (U+FEFF), it must be ignored."
	r := NewSSEEventReader(strings.NewReader("\xEF\xBB\xBFdata: hello\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Data != "hello" {
		t.Errorf("Data = %q, want %q", ev.Data, "hello")
	}
}

func TestReadEvent_LineWithNoColon(t *testing.T) {
	// https://html.spec.whatwg.org/multipage/server-sent-events.html#event-stream-interpretation
	// "If the line is not empty but does not contain a U+003A COLON character:
	// Process the field using the whole line as the field name, and an empty
	// string as the field value."
	//
	// e.g., a line "data" (no colon) is treated as field "data" with value "".
	r := NewSSEEventReader(strings.NewReader("data\n\n"))
	ev, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "data" with no colon → field "data", value "" → appends "" to data buffer.
	if ev.Data != "" {
		t.Errorf("Data = %q, want empty", ev.Data)
	}
}
