---
package: http
purpose: Production-grade HTTP/WebSocket/SSE building blocks — typed bidirectional connections, SSE streaming with resumption, and request/auth client helpers — all funneling writes through a single serialized Writer.
last_rebuilt: 04f5d50ab9801bba49c428990700c9aded8b119b
depends_on:
  - folder: middleware
    entities: [CORS, OriginChecker, NewOriginChecker]
entities:
  - name: Codec
    kind: interface
    role: Encode/Decode contract mapping typed messages I/O to WebSocket frame bytes + MessageType.
    why: Decouples wire encoding from transport so pings/errors stay JSON-at-transport while data uses any codec; codecs deliberately do NOT touch pings.
  - name: JSONCodec
    kind: struct
    role: Untyped any↔JSON codec for dynamic messages.
    why: Backs JSONConn/JSONSSEConn; the default everywhere a concrete type isn't known at compile time.
  - name: TypedJSONCodec
    kind: struct
    role: Strongly-typed JSON codec over Go structs.
    why: Decodes via zero-value of I — no factory needed because encoding/json allocates into the addressable target.
  - name: ProtoJSONCodec
    kind: struct
    role: protobuf-message codec serialized as protojson (human-readable wire).
    why: Needs a NewInput factory (or reflection fallback) because proto messages can't be zero-allocated generically; reflection path is the documented slow path.
  - name: BinaryProtoCodec
    kind: struct
    role: protobuf codec in binary wire format (BinaryMessage frames).
    why: Same factory/reflection split as ProtoJSONCodec; chosen for high-throughput where readability is irrelevant.
  - name: MessageType
    kind: type
    role: Int alias over gorilla TextMessage/BinaryMessage frame types.
    why: Lets codecs decide text-vs-binary framing while keeping the gorilla constant values.
  - name: BiDirStreamConn
    kind: interface
    role: Lifecycle+message contract for any bidirectional stream (SendPing/HandleMessage/OnError/OnClose/OnTimeout/Name/ConnId).
    why: Transport-agnostic core so WSConn extends it; OnTimeout returning false lets long-silent connections stay alive.
  - name: BiDirStreamConfig
    kind: struct
    role: PingPeriod / PongPeriod timing for heartbeat + timeout detection.
    why: PongPeriod doubles as the read deadline; default 30s/300s tuned for production, not dev.
  - name: WSConn
    kind: interface
    role: WebSocket connection — BiDirStreamConn plus ReadMessage + OnStart(*websocket.Conn).
    why: Separates the read loop (ReadMessage) from dispatch so WSHandleConn can drive any implementation generically.
  - name: WSHandler
    kind: interface
    role: Factory that Validates an HTTP request and returns a WSConn (auth happens here).
    why: Validate owns writing its own rejection response; returning false means the handler already responded.
  - name: WSConnConfig
    kind: struct
    role: BiDirStreamConfig + gorilla Upgrader for WS endpoints.
    why: Default CheckOrigin allows ALL origins — explicitly a dev default that must be overridden in production.
  - name: WSServe
    kind: func
    role: Builds an http.HandlerFunc: Validate → Upgrade → WSHandleConn.
    why: Primary WS entry point; closes conn via defer so handlers never manage the socket directly.
  - name: WSHandleConn
    kind: func
    role: Runs the connection event loop — ping ticker, pong/timeout checker, reader dispatch, OnClose.
    why: Wraps reads in a conc.Reader and maps gorilla unexpected-close errors to net.ErrClosed; resets read deadline on every received frame, not just pongs. Also selects on reader.ClosedChan, because conc.Reader reports net.ErrClosed only there (a clean client close and a server-side conn.Close both land there). Any close ends the loop even if OnError would swallow it.
  - name: BaseConn
    kind: struct
    role: Embeddable generic WS connection [I,O] wiring Codec + serialized Writer + ping/error/data dispatch.
    why: All outgoing traffic (data, ping, error, close) goes through one OutgoingMessage union on one Writer goroutine — the package's core defense against concurrent socket writes. A write error closes the socket so the read loop exits too; WriteTimeout turns a stuck client into such an error. CloseAfterSend arms a hard-close timer before queueing its final frames, since the unbuffered Writer can block behind a stuck client.
  - name: OutgoingMessage
    kind: struct
    role: Tagged union (Data | Ping | Error | Close) carried on BaseConn's Writer.
    why: Forces every write type onto a single channel so gorilla's no-concurrent-write rule is never violated.
  - name: PingData
    kind: struct
    role: Ping metadata (PingId/ConnId/Name) sent as JSON text.
    why: Pings are always JSON-at-transport regardless of data codec — debuggable and codec-independent.
  - name: BaseHandler
    kind: struct
    role: Trivial WSHandler that hands out BaseConn instances with a fixed codec.
    why: For prototyping/no-auth cases; accepts every connection.
  - name: JSONConn
    kind: type
    role: Alias BaseConn[any,any] for dynamic JSON WS.
    why: Convenience type referenced throughout docs as the "simplest" path.
  - name: JSONHandler
    kind: struct
    role: WSHandler accepting all connections, producing JSONConn.
    why: Zero-config WS endpoint helper.
  - name: SSEConn
    kind: interface
    role: Write-only server→client SSE connection contract (OnStart/OnClose/SendKeepalive/Done).
    why: SSE counterpart to WSConn; Done() lets app code programmatically terminate the SSEServe loop.
  - name: BaseSSEConn
    kind: struct
    role: Embeddable generic SSE connection [O] formatting events to the SSE wire protocol via a serialized Writer.
    why: Header flush MUST happen inside OnStart before the Writer goroutine exists, else a concurrent ResponseWriter race (the v0.0.10 fix); ready/done guarded by sync.Once.
  - name: SSEOutgoingMessage
    kind: struct
    role: Union of Data event (with Event/ID/Retry) or keepalive Comment.
    why: Same single-Writer discipline as WS; bare Retry (no Data) supports server-issued reconnect-delay hints.
  - name: SSEConnConfig
    kind: struct
    role: KeepalivePeriod for SSE comment heartbeats.
    why: Keepalives are SSE comments (": ...") ignored by clients but defeat proxy idle timeouts (nginx/ALB 60s); 0 disables.
  - name: SSEHandler
    kind: interface
    role: Factory validating a request into an SSEConn.
    why: Mirrors WSHandler; rejection response is the handler's responsibility.
  - name: SSEServe
    kind: func
    role: http.HandlerFunc that sets SSE headers, runs OnStart, drives keepalive, and exits on client disconnect or Done().
    why: Requires http.Server.WriteTimeout=0 or long-lived streams get killed; sets X-Accel-Buffering:no to defeat proxy buffering.
  - name: JSONSSEConn
    kind: type
    role: Alias BaseSSEConn[any] for dynamic JSON SSE.
    why: Convenience parallel to JSONConn.
  - name: JSONSSEHandler
    kind: struct
    role: SSEHandler accepting all connections.
    why: Zero-config SSE endpoint helper.
  - name: SSEHub
    kind: struct
    role: Instance-based registry of BaseSSEConn keyed by ConnId with Send/Broadcast/CloseAll.
    why: Instance (not global) for testability and multi-hub processes; Broadcast queues per-connection so a slow client can't block others; Register/Unregister deliberately do NOT manage connection lifecycle.
  - name: SSEEventReader
    kind: struct
    role: Client-side parser turning an SSE byte stream into discrete SSEReadEvent values.
    why: Full WHATWG parse algorithm — BOM strip, CRLF/CR/LF handling, multi-line data join, retry digit-validation; returns partial event + io.EOF when server omits the trailing blank line.
  - name: SSEReadEvent
    kind: struct
    role: One parsed SSE event (Event/Data/ID/Retry/Comment).
    why: Comment is captured (not discarded) so tests can assert on keepalives.
  - name: EventStore
    kind: interface
    role: Persist/Replay/Trim SSE events for Last-Event-ID resumption.
    why: Replay falls back to returning ALL events when the anchor ID is missing (conservative — assume the client missed everything rather than nothing).
  - name: MemoryEventStore
    kind: struct
    role: Bounded in-memory EventStore with FIFO eviction per stream.
    why: Single-process only; copies on eviction to drop references to the old backing array (avoid leaking evicted events).
  - name: StoredEvent
    kind: struct
    role: Persisted event (ID/Event/Data bytes).
    why: Stores pre-serialized Data to avoid re-encoding on replay.
  - name: StreamableResponse
    kind: interface
    role: Sealed union (SingleResponse | StreamResponse) for the POST-that-optionally-streams pattern.
    why: Sealed via unexported isStreamable() so only the two in-package types are valid — implements MCP 2025-03-26 Streamable HTTP.
  - name: SingleResponse
    kind: struct
    role: Synchronous JSON response branch of StreamableResponse.
    why: StatusCode 0 means 200; the simple path.
  - name: StreamResponse
    kind: struct
    role: SSE-streaming branch carrying an event channel.
    why: Handler owns the goroutine + channel close; StreamableServe only formats/flushes/watches disconnect.
  - name: SSEEvent
    kind: struct
    role: One event for StreamResponse (Event/Data/ID).
    why: Distinct from SSEReadEvent (read side) and SSEOutgoingMessage (BaseSSEConn) — this is the streamable-handler's value type.
  - name: StreamableHandlerFunc
    kind: type
    role: ctx+request → StreamableResponse decision function.
    why: Lets one endpoint choose sync vs stream per request based on Accept/content.
  - name: StreamableServe
    kind: func
    role: http.HandlerFunc dispatching on the returned StreamableResponse type.
    why: Falls back to json.Marshal if the codec fails on an event's Data (best-effort delivery).
  - name: Call
    kind: func
    role: Generic request→JSON-decode-into-T client helper.
    why: For in-memory request/response bodies only (not streaming); empty 2xx body returns zero T (handles 204), non-2xx returns *HTTPError preserving raw body+headers.
  - name: CallVoid
    kind: func
    role: Perform request, discard body, surface errors.
    why: For DELETE/ack POSTs where the body is irrelevant.
  - name: CallOption / WithClient / WithInsecureTLS
    kind: func
    role: Functional options selecting the http.Client.
    why: WithInsecureTLS lazily shares one InsecureSkipVerify client; mutually exclusive with WithClient (last option wins).
  - name: HTTPError
    kind: struct
    role: Error for non-2xx Call/CallVoid responses (Code/Body/Header).
    why: Keeps raw body+cloned headers so callers can parse structured errors or read Retry-After.
  - name: NewRequest / NewJsonRequest / NewBytesRequest / MakeUrl / JsonGet
    kind: func
    role: JSON-request builders and a raw GET helper.
    why: NewRequest defaults Content-Type to application/json; JsonGet kept for callers wanting the raw *http.Response alongside decoded body.
  - name: DefaultHttpClient (+Low/Medium/High QPS)
    kind: const
    role: Package-level pre-tuned http.Client pool sizes.
    why: TLS verification ON by default; tiered idle-conn limits per expected QPS.
  - name: DoWithAuthRetry
    kind: func
    role: Executes a request with at-most-one 401 (refresh) and one 403 (scope step-up) retry.
    why: buildReq must produce a fresh request each attempt (prior body may be consumed); reads only MaxErrorBodySize of error bodies.
  - name: AuthRetryConfig
    kind: struct
    role: SetAuth / OnUnauthorized / OnForbidden callbacks for the OAuth Bearer retry pattern.
    why: Nil config (or nil callbacks) means 401/403 surface immediately as AuthRetryError.
  - name: AuthRetryError
    kind: struct
    role: Error after auth retry exhaustion or callback failure.
    why: Auto-parses RequiredScopes from WWW-Authenticate per RFC 6750 so callers don't; Unwrap exposes the callback Cause.
  - name: ParseWWWAuthenticate
    kind: func
    role: Extract resource_metadata URL + scopes from a Bearer WWW-Authenticate header.
    why: Param matching guards against suffix false-positives (e.g. "noscope" vs "scope"); handles quoted+unquoted values.
  - name: HTTPStatusError
    kind: struct
    role: Generic non-2xx error (StatusCode/Header/Body string).
    why: Companion to IsHTTPTransient for retry classification; separate from HTTPError (Call's []byte-body variant).
  - name: IsHTTPTransient
    kind: func
    role: Reports whether a status code (>=500) is retriable.
    why: 4xx treated as terminal — won't change on retry.
  - name: MaxErrorBodySize
    kind: const
    role: 16KB cap on error-body reads.
    why: Prevents memory exhaustion from oversized error payloads; shared across error-body readers.
  - name: ParseAcceptTypes
    kind: func
    role: Parse Accept header into acceptsJSON / acceptsSSE flags.
    why: Drives Streamable-HTTP content negotiation; */* matches both per RFC 7231.
  - name: WriteFrame / ReadFrame
    kind: func
    role: LSP/MCP-stdio Content-Length message framing over byte streams.
    why: Tolerates and ignores non-Content-Length headers (e.g. LSP's optional Content-Type); io.EOF on empty reader = clean shutdown.
  - name: RPCConn / NewRPCConn
    kind: type
    role: Bidirectional JSON-RPC 2.0 connection over Content-Length framed streams; Call/Notify + a peer-request handler, with id-based response correlation.
    why: The layer above WriteFrame/ReadFrame. LSP clients and MCP stdio clients each hand-roll request/response correlation; this lets them share one conn.
  - name: ResolveURL
    kind: func
    role: Resolve a URL reference against a base per RFC 3986.
    why: Used by SSE clients to resolve endpoint-event URLs against the connection URL.
  - name: GenerateSessionID
    kind: func
    role: 128-bit crypto-random hex session ID.
    why: crypto/rand-backed and panics on RNG failure — session/CSRF-token safe, not a counter.
  - name: IDGen / AtomicIDGen
    kind: interface
    role: Concurrent-safe ID generator; AtomicIDGen yields decimal counter strings.
    why: IDs are explicitly opaque — callers must not assume ordering/format; AtomicIDGen is per-stream, NOT globally unique (contrast GenerateSessionID).
  - name: DetectBatch / SplitBatch
    kind: func
    role: JSON-RPC 2.0 batch detection ('[' first byte) and splitting into RawMessages.
    why: Defers per-element parsing to the caller so each request unmarshals independently.
  - name: DefaultBiDirStreamConfig / DefaultWSConnConfig / DefaultSSEConnConfig / DefaultStreamableConfig
    kind: func
    role: Constructors returning sensible defaults for each config struct.
    why: Centralize the production-vs-dev default story (allow-all origins, 30s ping, etc.).
  - name: JsonToQueryString / SendJsonResponse / ErrorToHttpCode / WSConnWriteError / WSConnWriteMessage / WSConnJSONReaderWriter / NormalizeWsUrl
    kind: func
    role: Misc server-side HTTP/WS helpers (query encoding, gRPC-status→HTTP mapping, raw WS writes, ws/wss URL normalization).
    why: ErrorToHttpCode/SendJsonResponse map google.golang.org/grpc status codes to HTTP — the package's gRPC-aware error bridge.
  - name: URLWaiter
    kind: struct
    role: Polls a URL until it returns success (readiness gate for dependencies).
    why: Uses an insecure-TLS ping client (urlPingHttpClient, InsecureSkipVerify) — readiness checks tolerate self-signed certs.
  - name: CORS
    kind: func
    role: Allow-all CORS wrapper for local dev.
    why: Deprecated — delegates to middleware.CORS; production should use an OriginChecker.
---

Purpose: this package is the HTTP-layer toolkit for ServiceKit — it provides three transport families (typed WebSocket via BaseConn, server-push SSE via BaseSSEConn/SSEHub, and the MCP-style "POST-that-optionally-streams" via StreamableServe) plus a set of client-side HTTP helpers (Call, DoWithAuthRetry, header/URL parsers, framing).

Core invariant — single serialized Writer. Both BaseConn and BaseSSEConn route every outgoing byte (data, ping/keepalive, error) through one conc.Writer goroutine via a tagged union (OutgoingMessage / SSEOutgoingMessage). This is the package's central concurrency discipline: it satisfies gorilla's "no concurrent writes" rule and the equivalent constraint on http.ResponseWriter. The CLAUDE.md convention "all writes through Writer" is enforced here. Pings and errors are always emitted as JSON text at the transport layer regardless of the data Codec, so they stay debuggable and codec-independent.

WS lifecycle ordering (WSHandleConn): defer OnClose registered first, then OnStart; the loop selects over a ping ticker (SendPing), a pong/timeout checker (compares wall-clock delta against PongPeriod, calls OnTimeout), and the conc.Reader output channel. The read deadline is reset on every received frame — any inbound traffic counts as liveness, not just pongs. gorilla unexpected-close errors are normalized to net.ErrClosed before reaching application code.

SSE flush-race gotcha (v0.0.10): BaseSSEConn.OnStart flushes headers *before* constructing the Writer goroutine; SSEServe relies on this and never flushes from the handler goroutine afterward. ready/done channels use sync.Once so Ready()/Close() are safe to call before OnStart completes (e.g. when a handler grabs the conn reference in Validate). SSE endpoints additionally require http.Server.WriteTimeout=0 and emit X-Accel-Buffering:no to survive proxies.

SSE resumption: the read side (SSEEventReader) implements the full WHATWG parse algorithm; the write side persists via EventStore (Last-Event-ID replay). MemoryEventStore is single-process with FIFO eviction and a conservative replay fallback (anchor-not-found ⇒ replay everything). For multi-process resumption a Redis/DB-backed EventStore is expected.

Three distinct SSE "event" shapes coexist deliberately: SSEOutgoingMessage (BaseSSEConn write side), SSEReadEvent (SSEEventReader read side), and SSEEvent (StreamableServe handler value). They are not interchangeable — pick by which subsystem you're in.

Client helpers: Call[T]/CallVoid are for in-memory request/response bodies only — streaming/large bodies should use client.Do directly and SSE should use SSEEventReader. Two parallel error types exist by design: HTTPError (Call's []byte body) and HTTPStatusError (string body, paired with IsHTTPTransient for retry classification). DoWithAuthRetry implements the OAuth Bearer pattern with a strict budget (one 401 refresh + one 403 step-up) and requires a fresh request per attempt. ID generation has two non-interchangeable tools: GenerateSessionID (crypto-random, unguessable) vs AtomicIDGen (counter, per-stream, not globally unique).
