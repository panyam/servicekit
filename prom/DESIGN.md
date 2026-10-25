---
package: prom
purpose: Exports servicekit's connection and HTTP request metrics to Prometheus.
depends_on:
  - folder: http
    entities: [ConnObserver, ConnInfo, CloseReason]
entities:
  - name: ConnMetrics
    kind: struct
    role: An http.ConnObserver that keeps active/opened/closed-by-reason/duration/message series, labelled transport and mode.
    why: One value covers WebSocket, SSE and grpcws because they all report through BaseConn or BaseSSEConn. Labels never carry per-connection values.
  - name: HTTPMetrics
    kind: struct
    role: RED middleware (requests by code, duration, request and response size) labelled method and route.
    why: route is the gorilla/mux path template, so it must be installed with router.Use. Hijacked requests (WebSocket upgrades) count as 101 and stay out of the histograms, since their duration is the connection's lifetime.
---

# prom

Lives in the root module, but `http/` and `middleware/` don't import it, so
a server that doesn't use it links no Prometheus code (`make check-deps`
fails if that changes). client_golang is pinned to v1.24.1, the version
massrelay already uses, so requiring servicekit doesn't force it up.

The recorder in `http.go` passes `Hijack` and `Flush` through. gorilla/websocket
asserts `http.Hijacker` on the writer directly, so without it every upgrade
behind the middleware fails, which is what happened to `RequestLogger` before
v0.1.7.
