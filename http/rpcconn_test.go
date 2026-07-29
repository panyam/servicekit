package http

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// pipePair wires an RPCConn to a fake peer over two io.Pipes and returns the
// peer's framed reader/writer.
func pipePair(handler RPCHandler) (conn *RPCConn, peerR *bufio.Reader, peerW io.Writer) {
	connR, peerWriter := io.Pipe() // peer writes -> conn reads
	peerReader, connW := io.Pipe() // conn writes -> peer reads
	conn = NewRPCConn(connW, connR, handler)
	return conn, bufio.NewReader(peerReader), peerWriter
}

func TestRPCConnCallCorrelatesResponse(t *testing.T) {
	conn, peerR, peerW := pipePair(nil)
	defer conn.Close()

	// Fake peer: read one request, reply with "pong".
	go func() {
		frame, err := ReadFrame(peerR)
		if err != nil {
			return
		}
		var req rpcMessage
		if json.Unmarshal(frame, &req) != nil || req.Method != "ping" || req.ID == nil {
			return
		}
		data, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`"pong"`)})
		_ = WriteFrame(peerW, data)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var got string
	if err := conn.Call(ctx, "ping", map[string]string{"x": "y"}, &got); err != nil {
		t.Fatal(err)
	}
	if got != "pong" {
		t.Fatalf("Call result = %q, want pong", got)
	}
}

func TestRPCConnAnswersIncomingRequest(t *testing.T) {
	handler := func(ctx context.Context, req *RPCRequest) (any, error) {
		if req.Method == "greet" {
			return "hi", nil
		}
		return nil, &RPCError{Code: rpcCodeMethodNotFound, Message: "nope"}
	}
	conn, peerR, peerW := pipePair(handler)
	defer conn.Close()

	// Peer initiates a request; conn's handler must answer it.
	idRaw := json.RawMessage("7")
	reqData, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: &idRaw, Method: "greet"})
	if err := WriteFrame(peerW, reqData); err != nil {
		t.Fatal(err)
	}

	frame, err := ReadFrame(peerR)
	if err != nil {
		t.Fatal(err)
	}
	var resp rpcMessage
	if err := json.Unmarshal(frame, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID == nil || string(*resp.ID) != "7" {
		t.Fatalf("response id mismatch: %s", string(frameID(resp)))
	}
	if string(resp.Result) != `"hi"` {
		t.Fatalf("response result = %s, want \"hi\"", resp.Result)
	}
}

func TestRPCConnNilHandlerReturnsMethodNotFound(t *testing.T) {
	conn, peerR, peerW := pipePair(nil)
	defer conn.Close()

	idRaw := json.RawMessage("1")
	reqData, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: &idRaw, Method: "whatever"})
	if err := WriteFrame(peerW, reqData); err != nil {
		t.Fatal(err)
	}
	frame, err := ReadFrame(peerR)
	if err != nil {
		t.Fatal(err)
	}
	var resp rpcMessage
	_ = json.Unmarshal(frame, &resp)
	if resp.Error == nil || resp.Error.Code != rpcCodeMethodNotFound {
		t.Fatalf("expected method-not-found error, got %+v", resp.Error)
	}
}

func frameID(m rpcMessage) json.RawMessage {
	if m.ID == nil {
		return json.RawMessage("null")
	}
	return *m.ID
}
