package prom

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	gohttp "github.com/panyam/servicekit/http"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// waitFor polls until cond holds, since ConnClosed runs on the server's
// connection goroutine after the client has gone.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type floodConn struct {
	gohttp.JSONConn
}

func (c *floodConn) OnStart(conn *websocket.Conn) error {
	if err := c.JSONConn.OnStart(conn); err != nil {
		return err
	}
	big := strings.Repeat("x", 1<<20)
	go func() {
		for c.Writer.IsRunning() {
			c.SendOutput(any(big))
		}
	}()
	return nil
}

type floodHandler struct{}

func (floodHandler) Validate(w http.ResponseWriter, r *http.Request) (*floodConn, bool) {
	c := &floodConn{}
	c.Codec = &gohttp.JSONCodec{}
	c.WriteTimeout = 200 * time.Millisecond
	return c, true
}

func TestConnMetrics_CountsADroppedClient(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewConnMetrics(reg)
	config := gohttp.DefaultWSConnConfig()
	config.Observer = m
	server := httptest.NewServer(gohttp.WSServe(floodHandler{}, config))
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ws := []string{"ws", ""}
	waitFor(t, "the dropped client", func() bool {
		return testutil.ToFloat64(m.closed.WithLabelValues("ws", "", "write_timeout")) == 1
	})
	if got := testutil.ToFloat64(m.active.WithLabelValues(ws...)); got != 0 {
		t.Errorf("active = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.opened.WithLabelValues(ws...)); got != 1 {
		t.Errorf("opened = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(m.duration); got != 1 {
		t.Errorf("duration series = %d, want 1", got)
	}
	if got := testutil.ToFloat64(m.sent.WithLabelValues("ws", "", "data")); got < 1 {
		t.Errorf("sent data = %v, want at least 1", got)
	}
}

func newRoutedServer(t *testing.T, m *HTTPMetrics) *httptest.Server {
	t.Helper()
	router := mux.NewRouter()
	router.Use(m.Middleware)
	router.HandleFunc("/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello")
	})
	router.HandleFunc("/missing/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	router.HandleFunc("/ws", gohttp.WSServe(&gohttp.JSONHandler{}, nil))
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

// Two ids on one route make one series, labelled with the template.
func TestHTTPMetrics_LabelsByRouteTemplate(t *testing.T) {
	m := NewHTTPMetrics(prometheus.NewRegistry())
	server := newRoutedServer(t, m)
	for _, p := range []string{"/games/1", "/games/2", "/missing/3"} {
		resp, err := http.Get(server.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/games/{id}", "200")); got != 2 {
		t.Errorf("/games/{id} 200 = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/missing/{id}", "404")); got != 1 {
		t.Errorf("/missing/{id} 404 = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(m.requests); got != 2 {
		t.Errorf("request series = %d, want 2", got)
	}
}

// The middleware must pass Hijack through, or every upgrade behind it fails
// (the v0.1.7 RequestLogger bug).
func TestHTTPMetrics_WebSocketUpgradeWorksAndCounts101(t *testing.T) {
	m := NewHTTPMetrics(prometheus.NewRegistry())
	server := newRoutedServer(t, m)
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatalf("upgrade through the middleware failed: %v", err)
	}
	client.Close()
	waitFor(t, "the upgrade to be counted", func() bool {
		return testutil.ToFloat64(m.requests.WithLabelValues("GET", "/ws", "101")) == 1
	})
	if got := testutil.CollectAndCount(m.duration); got != 0 {
		t.Errorf("duration series = %d, want 0 for a hijacked request", got)
	}
}
