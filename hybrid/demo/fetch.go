package demo

import (
	"bytes"
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
	GRPCWS  map[string]any
}

// Fetch reads game gameID from a running demo at baseURL ("http://host:port")
// over gateway JSON, Connect JSON and the grpcws watch stream.
func Fetch(baseURL, gameID string) (Payloads, error) {
	var p Payloads
	var err error
	if p.Gateway, err = getJSON(baseURL + GatewayPath + gameID); err != nil {
		return p, fmt.Errorf("gateway: %w", err)
	}
	if p.Connect, err = postJSON(baseURL+ConnectPath, fmt.Sprintf(`{"gameId":%q}`, gameID)); err != nil {
		return p, fmt.Errorf("connect: %w", err)
	}
	if p.GRPCWS, err = watchFirst("ws" + strings.TrimPrefix(baseURL, "http") + WatchPath + gameID + "/watch"); err != nil {
		return p, fmt.Errorf("grpcws: %w", err)
	}
	return p, nil
}

func getJSON(url string) (map[string]any, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	return decodeBody(resp)
}

func postJSON(url, body string) (map[string]any, error) {
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	return decodeBody(resp)
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

func watchFirst(url string) (map[string]any, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
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
