package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
