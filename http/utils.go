package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gorilla/websocket"
	conc "github.com/panyam/gocurrent"
	gut "github.com/panyam/goutils/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// SendJsonResponse writes a JSON response to the http.ResponseWriter.
// If err is nil, resp is marshaled to JSON and written with status 200 OK.
// If err is non-nil, an appropriate HTTP error code is set based on the gRPC
// status code (if present), and an error object is returned in the response body.
//
// The function handles gRPC status errors by extracting the code and message,
// and maps them to appropriate HTTP status codes via ErrorToHttpCode.
func SendJsonResponse(writer http.ResponseWriter, resp any, err error) {
	output := resp
	httpCode := ErrorToHttpCode(err)
	if err != nil {
		if er, ok := status.FromError(err); ok {
			code := er.Code()
			msg := er.Message()
			output = gut.StrMap{
				"error":   code,
				"message": msg,
			}
		} else {
			output = gut.StrMap{
				"error": err.Error(),
			}
		}
	}
	writer.WriteHeader(httpCode)
	writer.Header().Set("Content-Type", "application/json")
	jsonResp, err := json.Marshal(output)
	if err != nil {
		log.Println("Error happened in JSON marshal. Err: ", err)
	}
	writer.Write(jsonResp)
}

// ErrorToHttpCode converts a Go error to an appropriate HTTP status code.
// If err is nil, returns http.StatusOK (200).
// If err contains a gRPC status, maps it to the corresponding HTTP code:
//   - codes.PermissionDenied → 403 Forbidden
//   - codes.NotFound → 404 Not Found
//   - codes.AlreadyExists → 409 Conflict
//   - codes.InvalidArgument → 400 Bad Request
//   - Other errors → 500 Internal Server Error
func ErrorToHttpCode(err error) int {
	httpCode := http.StatusOK
	if err != nil {
		httpCode = http.StatusInternalServerError
		if er, ok := status.FromError(err); ok {
			code := er.Code()
			// msg := er.Message()
			// see if we have a specific client error
			if code == codes.PermissionDenied {
				httpCode = http.StatusForbidden
			} else if code == codes.NotFound {
				httpCode = http.StatusNotFound
			} else if code == codes.AlreadyExists {
				httpCode = http.StatusConflict
			} else if code == codes.InvalidArgument {
				httpCode = http.StatusBadRequest
			}
		}
	}
	return httpCode
}

// WSConnWriteError writes an error message to a WebSocket connection.
// If err is nil or io.EOF, no message is sent and nil is returned.
// For gRPC status errors, the error code is extracted and sent as JSON.
// The message is sent as a text frame containing JSON: {"error": <code>}
func WSConnWriteError(wsConn *websocket.Conn, err error) error {
	if err != nil && err != io.EOF {
		// Some kind of streamer rpc error
		log.Println("Error reading message from streamer: ", err)
		errdata := make(map[string]any)
		if er, ok := status.FromError(err); ok {
			errdata["error"] = er.Code()
		}
		jsonData, outerr := json.Marshal(errdata)
		if outerr != nil {
			outerr = wsConn.WriteMessage(websocket.TextMessage, jsonData)
		}
		if outerr != nil {
			log.Println("Error sending message: ", err)
		}
		return outerr
	}
	return nil
}

// WSConnWriteMessage writes a JSON message to a WebSocket connection.
// The message is marshaled to JSON and sent as a text frame.
// Returns any error from marshaling or writing.
func WSConnWriteMessage(wsConn *websocket.Conn, msg any) error {
	jsonResp, err := json.Marshal(msg)
	if err != nil {
		log.Println("Error happened in JSON marshal. Err: ", err)
	}
	outerr := wsConn.WriteMessage(websocket.TextMessage, jsonResp)
	if err != nil {
		log.Println("Error sending message: ", err)
	}
	return outerr
}

// WSConnJSONReaderWriter creates concurrent reader and writer for JSON messages
// over a WebSocket connection. The reader decodes incoming JSON messages into
// StrMap (map[string]any), and the writer sends outgoing StrMap messages as JSON.
//
// The reader handles connection close errors gracefully by converting them to
// net.ErrClosed. The writer handles io.EOF as a normal stream end and uses
// WSConnWriteError for error messages.
//
// This is useful for creating bidirectional JSON message streams over WebSocket.
func WSConnJSONReaderWriter(conn *websocket.Conn) (reader *conc.Reader[gut.StrMap], writer *conc.Writer[conc.Message[gut.StrMap]]) {
	reader = conc.NewReader(func() (out gut.StrMap, err error) {
		err = conn.ReadJSON(&out)
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
			err = net.ErrClosed
		}
		return
	})
	writer = conc.NewWriter(func(msg conc.Message[gut.StrMap]) error {
		if msg.Error == io.EOF {
			log.Println("Streamer closed...", msg.Error)
			// do nothing
			// SendJsonResponse(rw, nil, msg.Error)
			return msg.Error
		} else if msg.Error != nil {
			return WSConnWriteError(conn, msg.Error)
		} else {
			return WSConnWriteMessage(conn, msg.Value)
		}
	})
	return
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
