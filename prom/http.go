package prom

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics is request middleware recording rate, errors and duration:
//
//	servicekit_http_requests_total             counter    method, route, code
//	servicekit_http_request_duration_seconds   histogram  method, route
//	servicekit_http_request_size_bytes         histogram  method, route
//	servicekit_http_response_size_bytes        histogram  method, route
//
// route is the matched gorilla/mux path template ("/v1/{gameId}/subscribe"),
// not the raw path, so ids in the URL don't create a series each. That needs
// the middleware installed with router.Use, inside the router, where the
// match is known. Outside a mux router every request is route="unmatched"
// unless HTTPMetrics.Route says otherwise.
//
// A request the handler hijacks (a WebSocket upgrade) is counted with code
// "101" but left out of the duration and size histograms, since its duration
// is the connection's lifetime. ConnMetrics covers those.
type HTTPMetrics struct {
	// Route returns the route label for a request. Nil uses the mux template.
	// It must return a small, fixed set of values.
	Route func(r *http.Request) string

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	reqSize  *prometheus.HistogramVec
	respSize *prometheus.HistogramVec
}

// NewHTTPMetrics creates the metrics and registers them with reg, or with
// prometheus.DefaultRegisterer when reg is nil. It panics if they are already
// registered there.
func NewHTTPMetrics(reg prometheus.Registerer) *HTTPMetrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	labels := []string{"method", "route"}
	sizes := prometheus.ExponentialBuckets(64, 4, 9) // 64B to 4MB
	m := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "servicekit_http_requests_total",
			Help: "HTTP requests, by status code.",
		}, append(labels, "code")),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "servicekit_http_request_duration_seconds",
			Help:    "Time to serve an HTTP request.",
			Buckets: prometheus.DefBuckets,
		}, labels),
		reqSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "servicekit_http_request_size_bytes",
			Help:    "Request body size, from Content-Length.",
			Buckets: sizes,
		}, labels),
		respSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "servicekit_http_response_size_bytes",
			Help:    "Response body bytes written.",
			Buckets: sizes,
		}, labels),
	}
	reg.MustRegister(m.requests, m.duration, m.reqSize, m.respSize)
	return m
}

// Middleware wraps next. Its signature matches mux.MiddlewareFunc.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := m.route(r)
		rec := &recorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)

		code := rec.code
		if rec.hijacked {
			code = http.StatusSwitchingProtocols
		}
		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(code)).Inc()
		if rec.hijacked {
			return
		}
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		if r.ContentLength >= 0 {
			m.reqSize.WithLabelValues(r.Method, route).Observe(float64(r.ContentLength))
		}
		m.respSize.WithLabelValues(r.Method, route).Observe(float64(rec.bytes))
	})
}

func (m *HTTPMetrics) route(r *http.Request) string {
	if m.Route != nil {
		return m.Route(r)
	}
	if cur := mux.CurrentRoute(r); cur != nil {
		if tmpl, err := cur.GetPathTemplate(); err == nil {
			return tmpl
		}
	}
	return "unmatched"
}

// recorder captures the status and body size. It passes Hijack and Flush
// through, since gorilla/websocket asserts http.Hijacker on the writer
// directly and SSE needs http.Flusher.
type recorder struct {
	http.ResponseWriter
	code        int
	bytes       int64
	wroteHeader bool
	hijacked    bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.code, r.wroteHeader = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *recorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	conn, rw, err := h.Hijack()
	if err == nil {
		r.hijacked = true
	}
	return conn, rw, err
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
