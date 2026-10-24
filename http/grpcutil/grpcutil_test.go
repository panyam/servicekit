package grpcutil

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorToHttpCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, http.StatusOK},
		{errors.New("plain"), http.StatusInternalServerError},
		{status.Error(codes.PermissionDenied, "x"), http.StatusForbidden},
		{status.Error(codes.NotFound, "x"), http.StatusNotFound},
		{status.Error(codes.AlreadyExists, "x"), http.StatusConflict},
		{status.Error(codes.InvalidArgument, "x"), http.StatusBadRequest},
		{status.Error(codes.Unavailable, "x"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := ErrorToHttpCode(c.err); got != c.want {
			t.Errorf("ErrorToHttpCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestSendJsonResponse(t *testing.T) {
	cases := []struct {
		name     string
		resp     any
		err      error
		wantCode int
		wantBody string
	}{
		{"ok", map[string]int{"n": 1}, nil, http.StatusOK, `{"n":1}`},
		{"grpc status", nil, status.Error(codes.NotFound, "no game"), http.StatusNotFound, `{"error":5,"message":"no game"}`},
		{"plain error", nil, errors.New("boom"), http.StatusInternalServerError, `{"error":"boom"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SendJsonResponse(rec, c.resp, c.err)
			if rec.Code != c.wantCode {
				t.Errorf("status %d, want %d", rec.Code, c.wantCode)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type %q", ct)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != c.wantBody {
				t.Errorf("body %s, want %s", got, c.wantBody)
			}
		})
	}
}

func TestWSConnWriteError_SendsStatusCode(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		WSConnWriteError(conn, status.Error(codes.NotFound, "x"))
		conn.ReadMessage()
	}))
	defer srv.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got map[string]any
	if err := client.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if code, _ := got["error"].(float64); int(code) != int(codes.NotFound) {
		t.Fatalf("got %v, want error=%d", got, codes.NotFound)
	}
	b, _ := json.Marshal(got)
	if string(b) != `{"error":5}` {
		t.Fatalf("frame %s, want {\"error\":5}", b)
	}
}
