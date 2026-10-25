// Package prom exports servicekit's connection and HTTP metrics to
// Prometheus.
//
// ConnMetrics implements http.ConnObserver, so one value set on
// WSConnConfig.Observer, SSEConnConfig.Observer or grpcws.ConnOptions.Observer
// counts every connection those serve. HTTPMetrics is middleware for the
// request side. Serve both with promhttp.Handler (or promhttp.HandlerFor a
// custom registry) on /metrics.
//
// servicekit/http and servicekit/middleware do not import this package, so a
// server that doesn't use it links no Prometheus code. An app that already
// uses OpenTelemetry metrics can implement http.ConnObserver over its own
// meter instead.
package prom

import (
	"time"

	gohttp "github.com/panyam/servicekit/http"
	"github.com/prometheus/client_golang/prometheus"
)

// ConnMetrics records connection and message counts for WebSocket, SSE and
// grpcws connections. Every series is labelled with transport ("ws", "sse",
// "grpcws") and mode (the grpcws stream kind, empty otherwise):
//
//	servicekit_connections_active                gauge
//	servicekit_connections_opened_total          counter
//	servicekit_connections_closed_total          counter, plus reason
//	servicekit_connection_duration_seconds       histogram
//	servicekit_messages_sent_total               counter, plus kind
//	servicekit_messages_received_total           counter
//
// reason is a gohttp.CloseReason, so a client dropped for not reading shows
// up as reason="write_timeout" and a grpcws CloseOnStreamEnd close as
// reason="server_close". Labels never carry per-connection values, so the
// series count stays small.
//
// It is safe for concurrent use.
type ConnMetrics struct {
	active   *prometheus.GaugeVec
	opened   *prometheus.CounterVec
	closed   *prometheus.CounterVec
	duration *prometheus.HistogramVec
	sent     *prometheus.CounterVec
	received *prometheus.CounterVec
}

// DurationBuckets are the connection-lifetime histogram buckets, from 100ms
// to about 7 hours.
var DurationBuckets = prometheus.ExponentialBuckets(0.1, 4, 10)

// NewConnMetrics creates the metrics and registers them with reg, or with
// prometheus.DefaultRegisterer when reg is nil. It panics if they are already
// registered there, so create one per registry and share it.
func NewConnMetrics(reg prometheus.Registerer) *ConnMetrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	conn := []string{"transport", "mode"}
	m := &ConnMetrics{
		active: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "servicekit_connections_active",
			Help: "Connections currently open.",
		}, conn),
		opened: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "servicekit_connections_opened_total",
			Help: "Connections opened.",
		}, conn),
		closed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "servicekit_connections_closed_total",
			Help: "Connections closed, by reason.",
		}, append(conn, "reason")),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "servicekit_connection_duration_seconds",
			Help:    "How long connections stayed open.",
			Buckets: DurationBuckets,
		}, conn),
		sent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "servicekit_messages_sent_total",
			Help: "Frames written to clients, by kind.",
		}, append(conn, "kind")),
		received: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "servicekit_messages_received_total",
			Help: "Frames read from clients.",
		}, conn),
	}
	reg.MustRegister(m.active, m.opened, m.closed, m.duration, m.sent, m.received)
	return m
}

// ConnOpened implements gohttp.ConnObserver.
func (m *ConnMetrics) ConnOpened(info gohttp.ConnInfo) {
	m.active.WithLabelValues(info.Transport, info.Mode).Inc()
	m.opened.WithLabelValues(info.Transport, info.Mode).Inc()
}

// ConnClosed implements gohttp.ConnObserver.
func (m *ConnMetrics) ConnClosed(info gohttp.ConnInfo, reason gohttp.CloseReason, lifetime time.Duration) {
	m.active.WithLabelValues(info.Transport, info.Mode).Dec()
	m.closed.WithLabelValues(info.Transport, info.Mode, string(reason)).Inc()
	m.duration.WithLabelValues(info.Transport, info.Mode).Observe(lifetime.Seconds())
}

// MessageSent implements gohttp.ConnObserver.
func (m *ConnMetrics) MessageSent(info gohttp.ConnInfo, kind string) {
	m.sent.WithLabelValues(info.Transport, info.Mode, kind).Inc()
}

// MessageReceived implements gohttp.ConnObserver.
func (m *ConnMetrics) MessageReceived(info gohttp.ConnInfo) {
	m.received.WithLabelValues(info.Transport, info.Mode).Inc()
}

var _ gohttp.ConnObserver = (*ConnMetrics)(nil)
