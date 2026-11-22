package demo

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Payloads is the same game fetched over each transport, each decoded with
// encoding/json so field names and emitted zero values can be compared
// directly. protojson randomizes whitespace, so raw bytes can't be compared.
type Payloads struct {
	Gateway map[string]any
	Connect map[string]any
	GRPCWeb map[string]any
	GRPCWS  map[string]any
	// GRPCWebProto is the HTTP version the gRPC-Web call used, such as
	// "HTTP/2.0" over TLS.
	GRPCWebProto string
}

// Fetch reads game gameID from a running demo at baseURL ("http://host:port")
// over gateway JSON, Connect JSON, gRPC-Web JSON and the grpcws watch stream.
func Fetch(baseURL, gameID string) (Payloads, error) {
	return fetcher{http: http.DefaultClient, ws: websocket.DefaultDialer}.fetch(baseURL, gameID)
}

// FetchTLS is Fetch for a demo served over TLS at baseURL
// ("https://host:port"). It trusts only the certificates in roots, so a
// self-signed server works without disabling verification. The grpcws
// stream dials wss://.
func FetchTLS(baseURL, gameID string, roots *x509.CertPool) (Payloads, error) {
	// Separate configs: http.Transport adds "h2" to the NextProtos of the
	// config it holds, and a WebSocket dial offering h2 fails.
	f := fetcher{
		http: &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}, ForceAttemptHTTP2: true}},
		ws:   &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: roots}, HandshakeTimeout: 5 * time.Second},
	}
	return f.fetch(baseURL, gameID)
}

type fetcher struct {
	http *http.Client
	ws   *websocket.Dialer
}

func (f fetcher) fetch(baseURL, gameID string) (Payloads, error) {
	var p Payloads
	var err error
	req := fmt.Sprintf(`{"gameId":%q}`, gameID)
	if p.Gateway, err = f.getJSON(baseURL + GatewayPath + gameID); err != nil {
		return p, fmt.Errorf("gateway: %w", err)
	}
	if p.Connect, err = f.postJSON(baseURL+ConnectPath, req); err != nil {
		return p, fmt.Errorf("connect: %w", err)
	}
	if p.GRPCWeb, p.GRPCWebProto, err = f.postGRPCWeb(baseURL+ConnectPath, req); err != nil {
		return p, fmt.Errorf("grpc-web: %w", err)
	}
	if p.GRPCWS, err = f.watchFirst("ws" + strings.TrimPrefix(baseURL, "http") + WatchPath + gameID + "/watch"); err != nil {
		return p, fmt.Errorf("grpcws: %w", err)
	}
	return p, nil
}

func (f fetcher) getJSON(url string) (map[string]any, error) {
	resp, err := f.http.Get(url)
	if err != nil {
		return nil, err
	}
	return decodeBody(resp)
}

func (f fetcher) postJSON(url, body string) (map[string]any, error) {
	resp, err := f.http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	return decodeBody(resp)
}

// postGRPCWeb makes a unary gRPC-Web call with the JSON codec. Each message
// in the body is framed as one flag byte and a 4-byte big-endian length. In
// the response, frames with flag 0x80 carry the trailers, including
// grpc-status.
func (f fetcher) postGRPCWeb(url, body string) (map[string]any, string, error) {
	frame := make([]byte, 5, 5+len(body))
	binary.BigEndian.PutUint32(frame[1:], uint32(len(body)))
	frame = append(frame, body...)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(frame))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/grpc-web+json")
	req.Header.Set("X-Grpc-Web", "1")
	resp, err := f.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%s: %s", resp.Status, data)
	}
	var msg []byte
	status := resp.Header.Get("Grpc-Status")
	for len(data) >= 5 {
		n := int(binary.BigEndian.Uint32(data[1:5]))
		if len(data) < 5+n {
			return nil, "", fmt.Errorf("truncated frame")
		}
		payload := data[5 : 5+n]
		if data[0]&0x80 != 0 {
			for _, line := range strings.Split(string(payload), "\r\n") {
				if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "grpc-status") {
					status = strings.TrimSpace(v)
				}
			}
		} else if msg == nil {
			msg = payload
		}
		data = data[5+n:]
	}
	if status != "0" {
		return nil, "", fmt.Errorf("grpc-status %q", status)
	}
	var out map[string]any
	return out, resp.Proto, json.Unmarshal(msg, &out)
}

func decodeBody(resp *http.Response) (map[string]any, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", resp.Status, data)
	}
	var out map[string]any
	return out, json.Unmarshal(data, &out)
}

func (f fetcher) watchFirst(url string) (map[string]any, error) {
	conn, _, err := f.ws.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var frame struct {
			Type  string         `json:"type"`
			Data  map[string]any `json:"data"`
			Error string         `json:"error"`
		}
		if err := conn.ReadJSON(&frame); err != nil {
			return nil, err
		}
		switch frame.Type {
		case "data":
			return frame.Data, nil
		case "error":
			return nil, fmt.Errorf("stream error: %s", frame.Error)
		case "stream_end":
			return nil, fmt.Errorf("stream ended without data")
		}
	}
}
