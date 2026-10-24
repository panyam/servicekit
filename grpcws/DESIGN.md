---
package: grpcws
purpose: Bridges gRPC streaming RPCs (server/client/bidi) onto WebSocket connections using a JSON control envelope and the http package's BaseConn lifecycle machinery.
last_rebuilt: 04f5d50ab9801bba49c428990700c9aded8b119b
depends_on:
  - folder: http
    entities: [BaseConn, Codec, MessageType, BiDirStreamConfig, WSConnConfig, WSServe]
entities:
  - name: ControlMessage
    kind: struct
    role: The single JSON envelope ({type, data, error, pingId}) carrying every message in both directions.
    why: One envelope for all message types (data/error/ping/pong/cancel/end_send) keeps the transport protocol uniform so control and data flow over the same channel without a side band.
  - name: TypeData / TypeError / TypeStreamEnd / TypePing / TypePong / TypeCancel / TypeEndSend
    kind: const
    role: String discriminators for ControlMessage.Type.
    why: end_send is the explicit half-close signal — gRPC streams need an out-of-band "client done sending" event since WebSocket has no native half-close semantics.
  - name: GRPCWSCodec
    kind: struct
    role: Implements gohttp.Codec[ControlMessage,ControlMessage]; serializes envelopes as JSON and proto payloads via protojson.
    why: Payload is round-tripped proto→protojson→map→envelope rather than embedded as raw bytes, so the wire is human-readable JSON; NewRequest factory is required only for inbound decoding (server-streaming leaves it nil).
  - name: ServerStream
    kind: interface
    role: Constraint for server-streaming gRPC clients (Recv + grpc.ClientStream).
    why: Abstracts over concrete generated stream types so tests can substitute channel-driven doubles without protobuf codegen (per CLAUDE.md convention).
  - name: ClientStream
    kind: interface
    role: Constraint for client-streaming gRPC clients (Send + CloseAndRecv + grpc.ClientStream).
    why: CloseAndRecv (not Recv) reflects the single-response semantics of client streaming, triggered by the client's end_send.
  - name: BidiStream
    kind: interface
    role: Constraint for bidirectional gRPC clients (Send + Recv + CloseSend + grpc.ClientStream).
    why: Has CloseSend (half-close) but no CloseAndRecv, since responses keep flowing after the client stops sending.
  - name: StreamMetrics
    kind: struct
    role: Per-connection counters (ConnectedAt, MsgsSent, MsgsReceived as atomic.Int64) with Sent()/Received() accessors.
    why: The response-forwarding and message-handling goroutines increment the counters while OnClose reads them, so the fields are atomic.Int64 and a plain read does not compile (#41).
  - name: baseGRPCConn
    kind: struct
    role: Embeds gohttp.BaseConn and adds the cancellable gRPC stream context, metrics, and envelope send helpers shared by all three connection types.
    why: Overrides BaseConn.SendPing to emit the ControlMessage ping envelope instead of a raw WS ping — pings must travel as JSON envelopes like every other message; OnClose calls cancel() before the parent to tear down the gRPC stream context.
  - name: ServerStreamConn
    kind: struct
    role: Connection for rpc(Req) returns (stream Resp); spawns forwardResponses goroutine, only accepts pong/cancel inbound.
    why: Inbound data is meaningless for server streaming, so HandleMessage ignores TypeData; the request is parsed once in Validate, not per-message.
  - name: ClientStreamConn
    kind: struct
    role: Connection for rpc(stream Req) returns (Resp); forwards each TypeData to the stream, and on TypeEndSend calls CloseAndRecv and emits the single response + stream_end.
    why: No forwarding goroutine — the lone response only arrives synchronously at half-close, so it's produced inline in HandleMessage.
  - name: BidiStreamConn
    kind: struct
    role: Connection for rpc(stream Req) returns (stream Resp); runs forwardResponses goroutine for outbound while HandleMessage forwards inbound data and handles end_send via CloseSend.
    why: Concurrent read goroutine + write-on-handler is the only mode where both directions are live simultaneously; relies on the Writer being the single writer (no direct conn writes).
  - name: ServerStreamHandler / ClientStreamHandler / BidiStreamHandler
    kind: struct
    role: WSHandler implementations whose Validate creates the gRPC stream and constructs the matching *Conn.
    why: The gRPC stream is created in Validate (before the WS upgrade completes) so an upstream failure returns a clean HTTP error status instead of an opened-then-closed socket; cancel() is called on the creation-failure path to avoid context leaks.
  - name: NewServerStreamHandler / NewClientStreamHandler / NewBidiStreamHandler
    kind: func
    role: Factory constructors wiring CreateStream (and ParseRequest for server streaming) into a handler.
    why: ParseRequest exists only on the server-streaming factory because that mode needs the initial request extracted from the HTTP request (path vars, query); the other modes get their first message over the socket.
---

Lifecycle ordering: Validate (creates gRPC stream + cancellable ctx, fails fast to an HTTP error) → OnStart (calls parent BaseConn.OnStart, then for server/bidi spawns the forwardResponses goroutine) → HandleMessage loop (inbound) running concurrently with forwarding (outbound) → OnClose (calls cancel() to terminate the gRPC stream, then parent OnClose, then logs metrics).

The forwardResponses loop distinguishes io.EOF (clean end → stream_end) from other errors, but on a non-EOF error it checks streamCtx.Done() first and exits silently if the context was already cancelled — this suppresses the spurious error that a Recv returns when the client cancels, so a deliberate cancel doesn't surface as an error frame.

With ConnOptions.CloseOnStreamEnd, endStream replaces sendStreamEnd and hands stream_end to BaseConn.CloseAfterSend, which queues it ahead of a 1000 close frame. The hard-close timer is armed before anything is queued, because the Writer's input channel is unbuffered: if the client has stopped reading, queueing stream_end itself blocks, and only the timer's conn.Close frees it. WriteTimeout covers the other half, a client that stops reading while the stream is still producing (forwardResponses blocks in SendOutput before it ever sees io.EOF).

All writes go through SendOutput / the embedded BaseConn Writer; per the package convention nothing writes to the *websocket.Conn directly, which is what makes the concurrent forwarder + handler safe. Ping is overridden at baseGRPCConn level so the heartbeat rides the same envelope protocol rather than a raw WS control frame.

Half-close (end_send → CloseSend/CloseAndRecv) is the protocol's bridge over WebSocket's lack of native half-close, and is the key behavioral difference between client streaming (responds once, then stream_end) and bidi streaming (keeps receiving).
