---
package: servicekit-typescript-client
purpose: TypeScript client library for ServiceKit's WebSocket, gRPC-over-WebSocket, SSE, and streamable-HTTP transports, mirroring the server's transport/codec split.
last_rebuilt: 04f5d50ab9801bba49c428990700c9aded8b119b
depends_on:
entities:
  - name: BaseWSClient
    kind: class
    role: Low-level WebSocket client with automatic ping/pong heartbeat and codec-driven data framing.
    why: Control messages (ping/pong/error) are always JSON at the transport layer even when data uses a binary codec; pings are auto-answered without surfacing pong handling to callers.
  - name: GRPCWSClient
    kind: class
    role: Wraps BaseWSClient to speak the grpcws envelope protocol (data/error/stream_end inbound; data/end_send/cancel outbound).
    why: Adds half-close (endSend) and cancel semantics for the three gRPC streaming patterns; envelope wrap/unwrap is hidden so consumers only see application payloads.
  - name: TypedGRPCWSClient
    kind: class
    role: Generic compile-time-typed facade over GRPCWSClient parameterized by TIn/TOut.
    why: Pure type safety — it casts unknown to TOut on receive (protobuf JSON is assumed to match the TS type), adding no runtime behavior.
  - name: SSEClient
    kind: class
    role: Server-Sent Events client for SSEServe endpoints, built on fetch + ReadableStream.
    why: Deliberately avoids native EventSource because that is GET-only and cannot send custom headers; this enables auth tokens, POST, and Last-Event-ID reconnection tracking.
  - name: StreamableClient
    kind: class
    role: Client for StreamableServe endpoints implementing MCP's POST-that-optionally-streams pattern.
    why: A single POST may return application/json (returns parsed TOut) or text/event-stream (dispatches via callbacks, returns undefined) — branch is decided by response Content-Type, not the caller.
  - name: Codec
    kind: interface
    role: Encode/decode contract for business data messages (string or ArrayBuffer).
    why: Only covers data messages; control frames bypass the codec entirely, keeping the transport protocol codec-agnostic.
  - name: JSONCodec
    kind: class
    role: Default codec; JSON-stringify/parse, handling both text and ArrayBuffer inbound.
    why: Matches the server's JSONCodec so the default client works out of the box with no configuration.
  - name: BinaryCodec
    kind: class
    role: Codec wrapping user-supplied protobuf encode/decode functions for binary frames.
    why: Library-agnostic (bufbuild, ts-proto, protobuf-ts); slices the returned buffer to a real ArrayBuffer to avoid SharedArrayBuffer/byteOffset issues.
  - name: ControlMessage
    kind: interface
    role: The JSON envelope shape ({type, data?, error?, pingId?}) for all WS protocol frames.
    why: Single optional-field struct covers every MessageType, so parsing is one type guard on the `type` field.
  - name: MessageType
    kind: type
    role: Union of envelope type tags (data | error | stream_end | ping | pong | cancel | end_send).
    why: Documents the full grpcws wire vocabulary in one place; the source of truth for both directions.
  - name: ReadyState
    kind: const
    role: Frozen object of WebSocket readyState constants (CONNECTING/OPEN/CLOSING/CLOSED).
    why: Avoids depending on the global WebSocket constructor's static constants, which are absent when a mock WebSocket is injected.
  - name: ClientOptions
    kind: interface
    role: Constructor options for the WS clients (autoPong, WebSocket, codec).
    why: WebSocket is injectable specifically so tests (and Node) can supply a fake constructor — the basis of createMock().
  - name: ReadyStateType
    kind: type
    role: Value-union type derived from the ReadyState const.
    why: Lets readyState be typed without re-listing the numeric literals.
  - name: MessageHandler
    kind: type
    role: Generic data-message callback type `(data: T) => void`.
    why: Shared handler signature across all clients for consistent event wiring.
  - name: ErrorHandler
    kind: type
    role: Error callback type `(error: string) => void`.
    why: Errors are surfaced as strings (not Error objects) uniformly across transports.
  - name: VoidHandler
    kind: type
    role: No-arg callback type for close/streamEnd/done events.
    why: One alias for all lifecycle-completion callbacks.
  - name: SSEClientOptions
    kind: interface
    role: Options for SSEClient (fetch, headers, lastEventId).
    why: fetch is injectable for Node/testing; lastEventId seeds the Last-Event-ID reconnect header.
  - name: StreamableClientOptions
    kind: interface
    role: Options for StreamableClient (fetch, headers).
    why: Same fetch DI pattern as SSEClient for testability.
  - name: createParser
    kind: func
    role: Factory for a WHATWG-compliant SSE line parser with feed()/reset().
    why: Vendored from eventsource-parser v3.0.6 (MIT) rather than depended on — the SSE spec is frozen (2015) so there is no maintenance risk and one fewer runtime dependency.
  - name: EventSourceParser
    kind: interface
    role: Parser instance shape returned by createParser.
    why: reset() must be called between reconnections to clear partial-line and event state.
  - name: EventSourceMessage
    kind: interface
    role: A parsed SSE event ({event?, id?, data}).
    why: Multi-line data fields are joined with "\n"; absent event/id are undefined (browsers would default event to "message").
  - name: ParserCallbacks
    kind: interface
    role: Callback bag for createParser (onEvent/onRetry/onComment/onError).
    why: Comments (keepalives) only fire if onComment is provided, letting consumers silently drop them.
  - name: ParseError
    kind: class
    role: Error type for SSE parse failures (invalid-retry, unknown-field).
    why: Carries field/value/line context for diagnostics rather than a bare message.
  - name: ErrorType
    kind: type
    role: Union of SSE parse error categories.
    why: Distinguishes recoverable unknown-field from invalid-retry without string matching.
  - name: createMockWSPair
    kind: func
    role: Factory returning a fake WebSocket constructor + MockWSController for testing any BaseWSClient-based client.
    why: Operates at the raw transport level (no protocol awareness) and re-captures the socket on each connect() so the controller always targets the latest connection.
  - name: MockWSController
    kind: interface
    role: Test harness to drive a mock WebSocket (open/message/error/close, inspect sentRaw).
    why: Raw-frame level by design; protocol-aware mocking is layered on top in GRPCWSClient.createMock.
  - name: createMockSSEPair
    kind: func
    role: Factory returning a fake fetch + MockSSEController that serves a controllable SSE stream.
    why: Formats payloads as proper SSE wire text and captures request url/method/headers/body for assertions.
  - name: MockSSEController
    kind: interface
    role: Test harness to push SSE events (message/event/event-with-id/comment/close) and inspect the captured request.
    why: Protocol-level (auto-formats SSE frames) so tests assert on application data, not wire syntax.
  - name: MockController
    kind: interface
    role: Protocol-aware mock controller for GRPCWSClient (envelope wrap/unwrap built in).
    why: sentMessages returns already-unwrapped data payloads so tests never touch the {type:"data"} envelope.
---

Architecture: the package deliberately mirrors the server's two-layer split — a transport layer (connection + always-JSON control frames) under a codec layer (business data). Three WebSocket clients form a stack: `BaseWSClient` (raw, for JSONConn servers) → `GRPCWSClient` (envelope protocol) → `TypedGRPCWSClient` (compile-time types only). Two HTTP-stream clients (`SSEClient`, `StreamableClient`) share the vendored `sse-parser` and the injectable-`fetch` DI pattern.

Key non-obvious decisions:
- Native `EventSource` is intentionally not used (GET-only, no custom headers); SSE/streamable clients use `fetch` + `ReadableStream`.
- The SSE parser is vendored, not depended upon, because the SSE spec is frozen.
- All clients take an injectable transport (`WebSocket` or `fetch`) purely to enable the static `createMock()` helpers; mock controllers come in two flavors — raw-transport (`MockWSController`, `MockSSEController`) and protocol-aware (`MockController`, and SSE's auto-formatting).
- `AbortError` is swallowed in stream readers because it is the expected signal from `close()`.
- `ReadyState` is a local const rather than the WebSocket static so injected mocks need not reimplement those statics.
