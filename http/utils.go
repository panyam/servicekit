package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"

	"github.com/gorilla/websocket"
)

// JsonToQueryString converts a map to a URL query string.
// Keys are sorted alphabetically for deterministic output.
// Values are converted to strings using fmt.Sprintf("%v", value).
//
// Example:
//
//	params := map[string]any{"name": "John", "age": 30}
//	qs := JsonToQueryString(params) // "age=30&name=John"
func JsonToQueryString(json map[string]any) string {
	// Sort keys for deterministic output
	keys := make([]string, 0, len(json))
	for key := range json {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := ""
	for _, key := range keys {
		if len(out) > 0 {
			out += "&"
		}
		item := fmt.Sprintf("%s=%v", url.PathEscape(key), json[key])
		out += item
	}
	return out
}

// WSConnWriteMessage writes a JSON message to a WebSocket connection.
// The message is marshaled to JSON and sent as a text frame.
// Returns any error from marshaling or writing.
func WSConnWriteMessage(wsConn *websocket.Conn, msg any) error {
	jsonResp, err := json.Marshal(msg)
	if err != nil {
		log.Println("Error happened in JSON marshal. Err: ", err)
		return err
	}
	if err := wsConn.WriteMessage(websocket.TextMessage, jsonResp); err != nil {
		log.Println("Error sending message: ", err)
		return err
	}
	return nil
}

// NormalizeWsUrl converts an HTTP(S) URL to its WebSocket equivalent.
// It performs the following transformations:
//   - Removes trailing slashes
//   - Converts "http:" to "ws:"
//   - Converts "https:" to "wss:"
//
// URLs that are already WebSocket URLs (ws: or wss:) are returned unchanged
// after removing any trailing slash.
//
// Example:
//
//	NormalizeWsUrl("https://example.com/ws/") // "wss://example.com/ws"
func NormalizeWsUrl(httpOrWsUrl string) string {
	if strings.HasSuffix(httpOrWsUrl, "/") {
		httpOrWsUrl = (httpOrWsUrl)[:len(httpOrWsUrl)-1]
	}
	if strings.HasPrefix(httpOrWsUrl, "http:") {
		httpOrWsUrl = "ws:" + (httpOrWsUrl)[len("http:"):]
	}
	if strings.HasPrefix(httpOrWsUrl, "https:") {
		httpOrWsUrl = "wss:" + (httpOrWsUrl)[len("https:"):]
	}
	return httpOrWsUrl
}
