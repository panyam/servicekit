package http

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// RPCError is a JSON-RPC 2.0 error object. Return one from an RPCHandler to send
// a specific code/message; RPCConn.Call returns it when the peer replies with an
// error.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message) }

// Standard JSON-RPC 2.0 error codes used by RPCConn.
const (
	rpcCodeInternalError  = -32603
	rpcCodeMethodNotFound = -32601
)

// RPCRequest is a JSON-RPC 2.0 request or notification received from the peer and
// delivered to an RPCHandler. It is a notification when it carries no id.
type RPCRequest struct {
	Method string
	Params json.RawMessage
	id     *json.RawMessage
}

// IsNotification reports whether no response is expected (the request has no id).
func (r *RPCRequest) IsNotification() bool { return r.id == nil }

// RPCHandler answers requests the peer initiates over an RPCConn. For a
// notification (IsNotification), the returned result is ignored and no response
// is sent. Returning an error responds with a JSON-RPC error: an *RPCError is
// sent verbatim, any other error becomes an internal error. A nil handler answers
// requests with method-not-found and drops notifications.
type RPCHandler func(ctx context.Context, req *RPCRequest) (result any, err error)

// RPCConn is a bidirectional JSON-RPC 2.0 connection over a Content-Length framed
// byte stream — the LSP and MCP-stdio wire format (see WriteFrame/ReadFrame). It
// correlates responses to Call by id and dispatches peer-initiated requests and
// notifications to the handler. It is safe for concurrent use.
//
// It is transport-agnostic above the framing: give it any writer/reader pair
// (e.g. a subprocess's stdin and stdout) and it speaks JSON-RPC over them. LSP
// clients, MCP stdio clients, and other language-server drivers can share it
// instead of each re-implementing request/response correlation.
type RPCConn struct {
	w       io.Writer
	r       *bufio.Reader
	handler RPCHandler

	wmu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcResponse
	closed  bool
}

type rpcResponse struct {
	result json.RawMessage
	err    *RPCError
}

type rpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

// NewRPCConn returns a connection that writes framed messages to w and reads them
// from r, dispatching peer-initiated requests to handler (which may be nil). It
// starts a background read loop immediately; call Close to stop it.
func NewRPCConn(w io.Writer, r io.Reader, handler RPCHandler) *RPCConn {
	c := &RPCConn{
		w:       w,
		r:       bufio.NewReader(r),
		handler: handler,
		pending: map[int64]chan rpcResponse{},
	}
	go c.readLoop()
	return c
}

// Call invokes method with params and decodes the response into result (which may
// be nil to discard it). It blocks until the response arrives or ctx is done. A
// nil params sends no params.
func (c *RPCConn) Call(ctx context.Context, method string, params, result any) error {
	p, err := marshalParams(params)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("rpc connection closed")
	}
	c.nextID++
	id := c.nextID
	ch := make(chan rpcResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	idRaw := json.RawMessage(strconv.FormatInt(id, 10))
	if err := c.write(rpcMessage{ID: &idRaw, Method: method, Params: p}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-ch:
		if resp.err != nil {
			return resp.err
		}
		if result != nil && len(resp.result) > 0 {
			return json.Unmarshal(resp.result, result)
		}
		return nil
	}
}

// Notify sends a notification: no id, no response. A nil params sends no params.
func (c *RPCConn) Notify(method string, params any) error {
	p, err := marshalParams(params)
	if err != nil {
		return err
	}
	return c.write(rpcMessage{Method: method, Params: p})
}

// Close stops the connection and fails any in-flight calls. It does not close the
// underlying streams; their owner (e.g. the subprocess) closes them, which also
// unblocks the read loop.
func (c *RPCConn) Close() error {
	c.failAll(fmt.Errorf("rpc connection closed"))
	return nil
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	return json.Marshal(params)
}

func (c *RPCConn) write(msg rpcMessage) error {
	msg.JSONRPC = "2.0"
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return WriteFrame(c.w, data)
}

func (c *RPCConn) readLoop() {
	for {
		frame, err := ReadFrame(c.r)
		if err != nil {
			c.failAll(err)
			return
		}
		var msg rpcMessage
		if json.Unmarshal(frame, &msg) != nil {
			continue
		}
		switch {
		case msg.Method == "" && msg.ID != nil:
			c.deliverResponse(msg)
		case msg.Method != "":
			// Handle in a goroutine so a slow or reentrant handler cannot block
			// response delivery on the read loop.
			go c.dispatch(&RPCRequest{Method: msg.Method, Params: msg.Params, id: msg.ID})
		}
	}
}

func (c *RPCConn) deliverResponse(msg rpcMessage) {
	id, err := strconv.ParseInt(string(*msg.ID), 10, 64)
	if err != nil {
		return
	}
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- rpcResponse{result: msg.Result, err: msg.Error}
	}
}

func (c *RPCConn) dispatch(req *RPCRequest) {
	if req.IsNotification() {
		if c.handler != nil {
			_, _ = c.handler(context.Background(), req)
		}
		return
	}

	var res any
	var herr error
	if c.handler != nil {
		res, herr = c.handler(context.Background(), req)
	} else {
		herr = &RPCError{Code: rpcCodeMethodNotFound, Message: "method not found: " + req.Method}
	}

	resp := rpcMessage{ID: req.id}
	if herr != nil {
		if re, ok := herr.(*RPCError); ok {
			resp.Error = re
		} else {
			resp.Error = &RPCError{Code: rpcCodeInternalError, Message: herr.Error()}
		}
	} else {
		rb, _ := json.Marshal(res)
		resp.Result = rb
	}
	_ = c.write(resp)
}

func (c *RPCConn) failAll(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	pending := c.pending
	c.pending = map[int64]chan rpcResponse{}
	c.mu.Unlock()

	rerr := &RPCError{Code: rpcCodeInternalError, Message: err.Error()}
	for _, ch := range pending {
		ch <- rpcResponse{err: rerr}
	}
}
