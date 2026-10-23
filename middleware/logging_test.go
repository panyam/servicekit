package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// A wrapped handler must still be able to stream. RequestLogger embeds
// http.ResponseWriter to capture the status code, and a plain embedding
// strips http.Flusher, which silently converts every SSE response into a
// buffered one. The bytes are identical either way, so only a timing test
// or an interface check catches it.
func TestRequestLoggerPreservesFlusher(t *testing.T) {
	var sawFlusher, sawController bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawFlusher = w.(http.Flusher)
		sawController = http.NewResponseController(w).Flush() == nil
	})

	rec := httptest.NewRecorder()
	RequestLogger()(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	if !sawFlusher {
		t.Error("handler behind RequestLogger cannot assert http.Flusher; SSE will buffer")
	}
	if !sawController {
		t.Error("http.ResponseController could not flush through RequestLogger")
	}
}

// A skipped path bypasses the wrap entirely, so it keeps whatever the
// original writer had. Asserted so the skip branch cannot regress into
// wrapping.
func TestRequestLoggerSkippedPathPreservesFlusher(t *testing.T) {
	var sawFlusher bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawFlusher = w.(http.Flusher)
	})
	rec := httptest.NewRecorder()
	RequestLogger("/health")(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if !sawFlusher {
		t.Error("skipped path lost http.Flusher")
	}
}

// Flushing must be a no-op rather than a panic when the writer underneath
// cannot flush.
func TestRequestLoggerFlushOnNonFlusher(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("statusRecorder should always expose Flush")
		}
		f.Flush()
	})
	w := nonFlusherWriter{rec: httptest.NewRecorder()}
	RequestLogger()(inner).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
}

// Status capture must keep working alongside the added methods.
func TestRequestLoggerStillCapturesStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rec := httptest.NewRecorder()
	RequestLogger()(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
}

// nonFlusherWriter is an http.ResponseWriter with no Flush method.
type nonFlusherWriter struct{ rec *httptest.ResponseRecorder }

func (w nonFlusherWriter) Header() http.Header         { return w.rec.Header() }
func (w nonFlusherWriter) Write(b []byte) (int, error) { return w.rec.Write(b) }
func (w nonFlusherWriter) WriteHeader(code int)        { w.rec.WriteHeader(code) }

// A WebSocket upgrade behind RequestLogger must still work. gorilla/websocket
// (through v1.5.3) asserts http.Hijacker directly on the writer it is handed,
// so a wrapper that only offers Unwrap fails every upgrade with a 400.
func TestRequestLoggerAllowsWebSocketUpgrade(t *testing.T) {
	upgrader := websocket.Upgrader{}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mt, msg, err := conn.ReadMessage()
		if err == nil {
			conn.WriteMessage(mt, msg)
		}
	})
	srv := httptest.NewServer(RequestLogger()(inner))
	defer srv.Close()

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("upgrade through RequestLogger failed (status %d): %v", status, err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	if _, msg, err := conn.ReadMessage(); err != nil || string(msg) != "ping" {
		t.Fatalf("echo = %q, %v", msg, err)
	}
}

// After a successful hijack the handler writes the 101 itself, bypassing
// WriteHeader, so the recorder sets it to keep the access log truthful.
func TestRequestLoggerRecordsSwitchingProtocols(t *testing.T) {
	status := make(chan int, 1)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { status <- w.(*statusRecorder).status }()
		h, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		if conn, _, err := h.Hijack(); err == nil {
			conn.Close()
		}
	})
	srv := httptest.NewServer(RequestLogger()(inner))
	defer srv.Close()
	if resp, err := http.Get(srv.URL); err == nil {
		resp.Body.Close()
	}
	if got := <-status; got != http.StatusSwitchingProtocols {
		t.Fatalf("recorded status = %d, want 101", got)
	}
}

// Hijacking a writer that cannot be hijacked returns an error instead of
// panicking, so the upgrader reports it the normal way.
func TestRequestLoggerHijackOnNonHijacker(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("statusRecorder should always expose Hijack")
		}
		if _, _, err := h.Hijack(); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("Hijack on non-hijacker = %v, want http.ErrNotSupported", err)
		}
	})
	RequestLogger()(inner).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
}
