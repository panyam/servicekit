// src/types.ts
var ReadyState = {
  CONNECTING: 0,
  OPEN: 1,
  CLOSING: 2,
  CLOSED: 3
};
var JSONCodec = class {
  decode(data) {
    if (typeof data === "string") {
      return JSON.parse(data);
    }
    const text = new TextDecoder().decode(data);
    return JSON.parse(text);
  }
  encode(msg) {
    return JSON.stringify(msg);
  }
};
var BinaryCodec = class {
  constructor(decodeFunc, encodeFunc) {
    this.decodeFunc = decodeFunc;
    this.encodeFunc = encodeFunc;
  }
  decode(data) {
    if (typeof data === "string") {
      throw new Error("BinaryCodec received text data, expected binary");
    }
    return this.decodeFunc(data);
  }
  encode(msg) {
    const encoded = this.encodeFunc(msg);
    return encoded.buffer.slice(encoded.byteOffset, encoded.byteOffset + encoded.byteLength);
  }
};

// src/base-client.ts
var BaseWSClient = class {
  constructor(options = {}) {
    this.ws = null;
    /** Called when a data message is received (decoded by codec) */
    this.onMessage = () => {
    };
    /** Called when a ping is received (after auto-pong if enabled) */
    this.onPing = () => {
    };
    /** Called when the connection closes */
    this.onClose = () => {
    };
    /** Called when a WebSocket error occurs */
    this.onError = () => {
    };
    this._autoPong = options.autoPong ?? true;
    this._WebSocket = options.WebSocket ?? globalThis.WebSocket;
    this._codec = options.codec ?? new JSONCodec();
  }
  /** Get the codec used for encoding/decoding data messages */
  get codec() {
    return this._codec;
  }
  /**
   * Connect to a WebSocket server.
   * @param url The WebSocket URL to connect to
   * @returns Promise that resolves when connected
   */
  connect(url) {
    return new Promise((resolve, reject) => {
      if (this.ws && this.ws.readyState === ReadyState.OPEN) {
        resolve();
        return;
      }
      try {
        this.ws = new this._WebSocket(url);
      } catch (error) {
        reject(error);
        return;
      }
      this.ws.binaryType = "arraybuffer";
      this.ws.onopen = () => {
        resolve();
      };
      this.ws.onerror = (event) => {
        const errorMsg = "WebSocket error";
        this.onError(errorMsg);
        reject(new Error(errorMsg));
      };
      this.ws.onclose = () => {
        this.onClose();
      };
      this.ws.onmessage = (event) => {
        this.handleRawMessage(event.data);
      };
    });
  }
  /**
   * Send a data message to the server using the configured codec.
   * @param data The data to send (will be encoded by codec)
   */
  send(data) {
    if (!this.ws || this.ws.readyState !== ReadyState.OPEN) {
      throw new Error("WebSocket is not connected");
    }
    const encoded = this._codec.encode(data);
    this.ws.send(encoded);
  }
  /**
   * Send a raw message to the server (bypasses codec).
   * Useful for control messages like pong.
   * @param message The raw string or ArrayBuffer to send
   */
  sendRaw(message) {
    if (!this.ws || this.ws.readyState !== ReadyState.OPEN) {
      throw new Error("WebSocket is not connected");
    }
    this.ws.send(message);
  }
  /**
   * Close the WebSocket connection.
   */
  close() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
  /**
   * Check if the client is connected.
   */
  get isConnected() {
    return this.ws !== null && this.ws.readyState === ReadyState.OPEN;
  }
  /**
   * Get the WebSocket ready state.
   */
  get readyState() {
    return this.ws?.readyState ?? ReadyState.CLOSED;
  }
  /**
   * Handle incoming raw message data.
   * - Text frames: Check for control messages (ping), then decode with codec
   * - Binary frames: Decode directly with codec
   *
   * Control messages (ping/pong/error) are always JSON text frames,
   * regardless of what codec is used for data messages.
   */
  handleRawMessage(data) {
    if (data instanceof ArrayBuffer) {
      try {
        const decoded = this._codec.decode(data);
        this.onMessage(decoded);
      } catch (err) {
        this.onError(`Failed to decode binary message: ${err}`);
      }
      return;
    }
    let parsed;
    try {
      parsed = JSON.parse(data);
    } catch {
      try {
        const decoded = this._codec.decode(data);
        this.onMessage(decoded);
      } catch (err) {
        this.onError(`Failed to decode text message: ${err}`);
      }
      return;
    }
    if (this.isPingMessage(parsed)) {
      const pingId = parsed.pingId;
      if (this._autoPong && pingId !== void 0) {
        this.sendPong(pingId);
      }
      this.onPing(pingId ?? 0);
      return;
    }
    try {
      const decoded = this._codec.decode(data);
      this.onMessage(decoded);
    } catch (err) {
      this.onError(`Failed to decode message: ${err}`);
    }
  }
  /**
   * Check if a message is a ping message.
   * Pings are always JSON with type: "ping".
   */
  isPingMessage(msg) {
    return typeof msg === "object" && msg !== null && "type" in msg && msg.type === "ping";
  }
  /**
   * Send a pong response.
   * Pongs are always JSON, bypassing the codec.
   */
  sendPong(pingId) {
    this.sendRaw(JSON.stringify({ type: "pong", pingId }));
  }
};

// src/mock.ts
function createMockWSPair() {
  let ws = null;
  class MockWebSocket {
    constructor(_url) {
      this.readyState = ReadyState.CONNECTING;
      this.binaryType = "";
      this.onopen = null;
      this.onmessage = null;
      this.onerror = null;
      this.onclose = null;
      this._sent = [];
      ws = this;
    }
    send(data) {
      this._sent.push(data);
    }
    close() {
      this.readyState = ReadyState.CLOSED;
      this.onclose?.({});
    }
    get sentRaw() {
      return this._sent;
    }
  }
  const assertConnected = (method) => {
    if (!ws) throw new Error(`Call client.connect() before ${method}()`);
  };
  const controller = {
    get sentRaw() {
      return ws?.sentRaw ?? [];
    },
    simulateOpen() {
      assertConnected("simulateOpen");
      ws.readyState = ReadyState.OPEN;
      ws.onopen?.({});
    },
    simulateRawMessage(data) {
      assertConnected("simulateRawMessage");
      ws.onmessage?.({ data });
    },
    simulateWsError() {
      assertConnected("simulateWsError");
      ws.onerror?.({});
    },
    simulateClose(code) {
      assertConnected("simulateClose");
      ws.readyState = ReadyState.CLOSED;
      ws.onclose?.({});
    },
    get readyState() {
      return ws?.readyState ?? ReadyState.CLOSED;
    }
  };
  return {
    WebSocket: MockWebSocket,
    controller
  };
}

// src/grpcws-client.ts
var GRPCWSClient = class _GRPCWSClient {
  constructor(options = {}) {
    /** Called when a data message is received */
    this.onMessage = () => {
    };
    /** Called when the stream ends normally */
    this.onStreamEnd = () => {
    };
    /** Called when the server sends an error */
    this.onError = () => {
    };
    /** Called when the connection closes */
    this.onClose = () => {
    };
    /** Called when a ping is received */
    this.onPing = () => {
    };
    this.base = new BaseWSClient(options);
    this.setupBaseHandlers();
  }
  /**
   * Connect to a grpcws WebSocket server.
   * @param url The WebSocket URL to connect to
   * @returns Promise that resolves when connected
   */
  connect(url) {
    return this.base.connect(url);
  }
  /**
   * Send data to the server.
   * The data is wrapped in a {type: "data", data: ...} envelope.
   * @param data The data payload to send
   */
  send(data) {
    const envelope = {
      type: "data",
      data
    };
    this.base.send(envelope);
  }
  /**
   * Signal that the client is done sending (half-close).
   * Used in client streaming and bidirectional streaming to indicate
   * the client won't send any more messages.
   */
  endSend() {
    this.base.send({ type: "end_send" });
  }
  /**
   * Cancel the stream.
   * Signals to the server that the client wants to terminate the stream.
   */
  cancel() {
    this.base.send({ type: "cancel" });
  }
  /**
   * Close the WebSocket connection.
   */
  close() {
    this.base.close();
  }
  /**
   * Check if the client is connected.
   */
  get isConnected() {
    return this.base.isConnected;
  }
  /**
   * Get the WebSocket ready state.
   */
  get readyState() {
    return this.base.readyState;
  }
  /**
   * Create a mock client + controller pair for testing.
   *
   * Returns a pre-wired GRPCWSClient backed by a fake WebSocket, so
   * consumers don't need to mock WebSocket internals or know about the
   * servicekit envelope protocol.
   *
   * @example
   * ```typescript
   * const { client, controller } = GRPCWSClient.createMock();
   *
   * client.onMessage = (data) => { handle(data); };
   * client.connect('ws://test');
   * controller.simulateOpen();
   *
   * controller.simulateMessage({ event: { case: 'roomJoined', value: {} } });
   * expect(controller.sentMessages).toHaveLength(0);
   *
   * client.send({ action: { case: 'join' } });
   * expect(controller.sentMessages[0]).toMatchObject({ action: { case: 'join' } });
   * ```
   */
  static createMock() {
    const { WebSocket, controller: wsCtrl } = createMockWSPair();
    const client = new _GRPCWSClient({ WebSocket });
    const controller = {
      get sentMessages() {
        const messages = [];
        for (const raw of wsCtrl.sentRaw) {
          try {
            const parsed = JSON.parse(raw);
            if (parsed.type === "data") {
              messages.push(parsed.data);
            }
          } catch {
          }
        }
        return messages;
      },
      simulateOpen() {
        wsCtrl.simulateOpen();
      },
      simulateMessage(data) {
        wsCtrl.simulateRawMessage(JSON.stringify({ type: "data", data }));
      },
      simulateError(message) {
        wsCtrl.simulateRawMessage(
          JSON.stringify({ type: "error", error: message ?? "Mock error" })
        );
      },
      simulateClose(code) {
        wsCtrl.simulateClose(code);
      },
      get readyState() {
        return wsCtrl.readyState;
      }
    };
    return { client, controller };
  }
  /**
   * Set up handlers on the base client to process grpcws envelope messages.
   */
  setupBaseHandlers() {
    this.base.onMessage = (msg) => {
      this.handleEnvelopeMessage(msg);
    };
    this.base.onClose = () => {
      this.onClose();
    };
    this.base.onError = (error) => {
      this.onError(error);
    };
    this.base.onPing = (pingId) => {
      this.onPing(pingId);
    };
  }
  /**
   * Handle an incoming envelope message.
   */
  handleEnvelopeMessage(msg) {
    if (!this.isControlMessage(msg)) {
      console.warn("Received non-envelope message:", msg);
      return;
    }
    const controlMsg = msg;
    switch (controlMsg.type) {
      case "data":
        this.onMessage(controlMsg.data);
        break;
      case "stream_end":
        this.onStreamEnd();
        break;
      case "error":
        this.onError(controlMsg.error ?? "Unknown error");
        break;
      default:
        console.warn("Unknown message type:", controlMsg.type);
    }
  }
  /**
   * Check if a message is a valid control message.
   */
  isControlMessage(msg) {
    return typeof msg === "object" && msg !== null && "type" in msg && typeof msg.type === "string";
  }
};

// src/typed-client.ts
var TypedGRPCWSClient = class {
  constructor(options = {}) {
    /** Called when a typed message is received */
    this.onMessage = () => {
    };
    /** Called when the stream ends normally */
    this.onStreamEnd = () => {
    };
    /** Called when the server sends an error */
    this.onError = () => {
    };
    /** Called when the connection closes */
    this.onClose = () => {
    };
    /** Called when a ping is received */
    this.onPing = () => {
    };
    this.client = new GRPCWSClient(options);
    this.setupHandlers();
  }
  /**
   * Connect to a grpcws WebSocket server.
   * @param url The WebSocket URL to connect to
   * @returns Promise that resolves when connected
   */
  connect(url) {
    return this.client.connect(url);
  }
  /**
   * Send a typed message to the server.
   * @param data The typed data payload to send
   */
  send(data) {
    this.client.send(data);
  }
  /**
   * Signal that the client is done sending (half-close).
   */
  endSend() {
    this.client.endSend();
  }
  /**
   * Cancel the stream.
   */
  cancel() {
    this.client.cancel();
  }
  /**
   * Close the WebSocket connection.
   */
  close() {
    this.client.close();
  }
  /**
   * Check if the client is connected.
   */
  get isConnected() {
    return this.client.isConnected;
  }
  /**
   * Get the WebSocket ready state.
   */
  get readyState() {
    return this.client.readyState;
  }
  /**
   * Set up handlers to forward events with proper typing.
   */
  setupHandlers() {
    this.client.onMessage = (data) => {
      this.onMessage(data);
    };
    this.client.onStreamEnd = () => {
      this.onStreamEnd();
    };
    this.client.onError = (error) => {
      this.onError(error);
    };
    this.client.onClose = () => {
      this.onClose();
    };
    this.client.onPing = (pingId) => {
      this.onPing(pingId);
    };
  }
};

// src/sse-parser.ts
var ParseError = class extends Error {
  constructor(message, options) {
    super(message);
    this.name = "ParseError";
    this.type = options.type;
    this.field = options.field;
    this.value = options.value;
    this.line = options.line;
  }
};
function noop(_arg) {
}
function createParser(callbacks) {
  if (typeof callbacks === "function") {
    throw new TypeError(
      "`callbacks` must be an object, got a function instead. Did you mean `{onEvent: fn}`?"
    );
  }
  const { onEvent = noop, onError = noop, onRetry = noop, onComment } = callbacks;
  let incompleteLine = "";
  let isFirstChunk = true;
  let id;
  let data = "";
  let eventType = "";
  function feed(newChunk) {
    const chunk = isFirstChunk ? newChunk.replace(/^\xEF\xBB\xBF/, "") : newChunk;
    const [complete, incomplete] = splitLines(`${incompleteLine}${chunk}`);
    for (const line of complete) {
      parseLine(line);
    }
    incompleteLine = incomplete;
    isFirstChunk = false;
  }
  function parseLine(line) {
    if (line === "") {
      dispatchEvent();
      return;
    }
    if (line.startsWith(":")) {
      if (onComment) {
        onComment(line.slice(line.startsWith(": ") ? 2 : 1));
      }
      return;
    }
    const fieldSeparatorIndex = line.indexOf(":");
    if (fieldSeparatorIndex !== -1) {
      const field = line.slice(0, fieldSeparatorIndex);
      const offset = line[fieldSeparatorIndex + 1] === " " ? 2 : 1;
      const value = line.slice(fieldSeparatorIndex + offset);
      processField(field, value, line);
      return;
    }
    processField(line, "", line);
  }
  function processField(field, value, line) {
    switch (field) {
      case "event":
        eventType = value;
        break;
      case "data":
        data = `${data}${value}
`;
        break;
      case "id":
        id = value.includes("\0") ? void 0 : value;
        break;
      case "retry":
        if (/^\d+$/.test(value)) {
          onRetry(parseInt(value, 10));
        } else {
          onError(
            new ParseError(`Invalid \`retry\` value: "${value}"`, {
              type: "invalid-retry",
              value,
              line
            })
          );
        }
        break;
      default:
        onError(
          new ParseError(
            `Unknown field "${field.length > 20 ? `${field.slice(0, 20)}\u2026` : field}"`,
            { type: "unknown-field", field, value, line }
          )
        );
        break;
    }
  }
  function dispatchEvent() {
    const shouldDispatch = data.length > 0;
    if (shouldDispatch) {
      onEvent({
        id,
        event: eventType || void 0,
        data: data.endsWith("\n") ? data.slice(0, -1) : data
      });
    }
    id = void 0;
    data = "";
    eventType = "";
  }
  function reset(options = {}) {
    if (incompleteLine && options.consume) {
      parseLine(incompleteLine);
    }
    isFirstChunk = true;
    id = void 0;
    data = "";
    eventType = "";
    incompleteLine = "";
  }
  return { feed, reset };
}
function splitLines(chunk) {
  const lines = [];
  let incompleteLine = "";
  let searchIndex = 0;
  while (searchIndex < chunk.length) {
    const crIndex = chunk.indexOf("\r", searchIndex);
    const lfIndex = chunk.indexOf("\n", searchIndex);
    let lineEnd = -1;
    if (crIndex !== -1 && lfIndex !== -1) {
      lineEnd = Math.min(crIndex, lfIndex);
    } else if (crIndex !== -1) {
      if (crIndex === chunk.length - 1) {
        lineEnd = -1;
      } else {
        lineEnd = crIndex;
      }
    } else if (lfIndex !== -1) {
      lineEnd = lfIndex;
    }
    if (lineEnd === -1) {
      incompleteLine = chunk.slice(searchIndex);
      break;
    } else {
      const line = chunk.slice(searchIndex, lineEnd);
      lines.push(line);
      searchIndex = lineEnd + 1;
      if (chunk[searchIndex - 1] === "\r" && chunk[searchIndex] === "\n") {
        searchIndex++;
      }
    }
  }
  return [lines, incompleteLine];
}

// src/sse-mock.ts
function createMockSSEPair(contentType = "text/event-stream") {
  let streamController = null;
  let capturedUrl;
  let capturedHeaders = {};
  let capturedMethod;
  let capturedBody;
  const encoder = new TextEncoder();
  const enqueue = (text) => {
    if (streamController) {
      streamController.enqueue(encoder.encode(text));
    }
  };
  const mockFetch = async (input, init) => {
    capturedUrl = typeof input === "string" ? input : input.toString();
    capturedMethod = init?.method ?? "GET";
    capturedBody = typeof init?.body === "string" ? init.body : void 0;
    capturedHeaders = {};
    if (init?.headers) {
      if (init.headers instanceof Headers) {
        init.headers.forEach((v, k) => {
          capturedHeaders[k] = v;
        });
      } else if (Array.isArray(init.headers)) {
        for (const [k, v] of init.headers) {
          capturedHeaders[k] = v;
        }
      } else {
        capturedHeaders = { ...init.headers };
      }
    }
    const body = new ReadableStream({
      start(controller2) {
        streamController = controller2;
      }
    });
    return new Response(body, {
      status: 200,
      headers: { "Content-Type": contentType }
    });
  };
  const controller = {
    simulateMessage(data) {
      enqueue(`data: ${JSON.stringify(data)}

`);
    },
    simulateEvent(event, data) {
      enqueue(`event: ${event}
data: ${JSON.stringify(data)}

`);
    },
    simulateEventWithId(event, id, data) {
      enqueue(`event: ${event}
id: ${id}
data: ${JSON.stringify(data)}

`);
    },
    simulateComment(text) {
      enqueue(`: ${text}

`);
    },
    simulateClose() {
      if (streamController) {
        streamController.close();
        streamController = null;
      }
    },
    get receivedUrl() {
      return capturedUrl;
    },
    get receivedHeaders() {
      return capturedHeaders;
    },
    get receivedMethod() {
      return capturedMethod;
    },
    get receivedBody() {
      return capturedBody;
    }
  };
  return { fetch: mockFetch, controller };
}

// src/sse-client.ts
var SSEClient = class _SSEClient {
  constructor(options = {}) {
    this._connected = false;
    this._abortController = null;
    /** Called when an unnamed data event is received (no "event:" field). */
    this.onMessage = () => {
    };
    /** Called when a named event is received (has "event:" field). */
    this.onEvent = () => {
    };
    /** Called when the SSE stream closes (server closes or network error). */
    this.onClose = () => {
    };
    /** Called when an error occurs (fetch failure, parse error). */
    this.onError = () => {
    };
    this._fetch = options.fetch ?? globalThis.fetch;
    this._headers = options.headers ?? {};
    this._lastEventId = options.lastEventId;
  }
  /**
   * Connect to an SSE endpoint.
   *
   * @param url The HTTP URL to connect to
   * @returns Promise that resolves when the connection is established (headers received)
   * @throws Error if the response is not 200 or not text/event-stream
   */
  async connect(url) {
    const headers = { ...this._headers };
    if (this._lastEventId) {
      headers["Last-Event-ID"] = this._lastEventId;
    }
    this._abortController = new AbortController();
    const response = await this._fetch(url, {
      headers,
      signal: this._abortController.signal
    });
    if (!response.ok) {
      throw new Error(`SSE connection failed: HTTP ${response.status}`);
    }
    const contentType = response.headers.get("Content-Type") ?? "";
    if (!contentType.includes("text/event-stream")) {
      throw new Error(`Expected text/event-stream, got ${contentType}`);
    }
    if (!response.body) {
      throw new Error("Response has no body");
    }
    this._connected = true;
    this._readStream(response.body);
  }
  /**
   * Close the SSE connection.
   * Aborts the underlying fetch request and fires onClose.
   */
  close() {
    if (this._abortController) {
      this._abortController.abort();
      this._abortController = null;
    }
    if (this._connected) {
      this._connected = false;
      this.onClose();
    }
  }
  /** Whether the client is currently connected to an SSE stream. */
  get isConnected() {
    return this._connected;
  }
  /** The last event ID received from the server (for reconnection). */
  get lastEventId() {
    return this._lastEventId;
  }
  /**
   * Create a mock SSEClient + controller pair for testing.
   *
   * @example
   * ```typescript
   * const { client, controller } = SSEClient.createMock();
   * client.onMessage = (data) => received.push(data);
   * await client.connect('http://test/events');
   * controller.simulateMessage({ hello: 'world' });
   * controller.simulateClose();
   * ```
   */
  static createMock() {
    const { fetch, controller } = createMockSSEPair();
    const client = new _SSEClient({ fetch });
    return { client, controller };
  }
  /** Read the SSE stream and dispatch events via the parser. */
  async _readStream(body) {
    const reader = body.getReader();
    const decoder = new TextDecoder();
    const parser = createParser({
      onEvent: (event) => {
        if (event.id !== void 0) {
          this._lastEventId = event.id;
        }
        let parsed;
        try {
          parsed = JSON.parse(event.data);
        } catch {
          parsed = event.data;
        }
        if (event.event) {
          this.onEvent(event.event, parsed);
        } else {
          this.onMessage(parsed);
        }
      }
      // Comments (keepalive) are silently consumed — no handler needed
    });
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        parser.feed(decoder.decode(value, { stream: true }));
      }
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") {
        return;
      }
      this.onError(String(err));
    } finally {
      if (this._connected) {
        this._connected = false;
        this.onClose();
      }
    }
  }
};

// src/streamable-client.ts
var StreamableClient = class {
  constructor(options = {}) {
    this._abortController = null;
    /** Called when an unnamed data event is received during streaming. */
    this.onMessage = () => {
    };
    /** Called when a named event is received during streaming. */
    this.onEvent = () => {
    };
    /** Called when the SSE stream completes (channel closed by server). */
    this.onDone = () => {
    };
    /** Called when an error occurs. */
    this.onError = () => {
    };
    this._fetch = options.fetch ?? globalThis.fetch;
    this._headers = options.headers ?? {};
  }
  /**
   * POST a request to a StreamableServe endpoint.
   *
   * Returns the parsed response for JSON responses, or undefined for
   * streaming responses (events dispatched via callbacks).
   *
   * @param url The endpoint URL
   * @param body The request body (JSON-serialized)
   * @returns Parsed response (JSON path) or undefined (streaming path)
   */
  async post(url, body) {
    this._abortController = new AbortController();
    const response = await this._fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "text/event-stream, application/json",
        ...this._headers
      },
      body: JSON.stringify(body),
      signal: this._abortController.signal
    });
    if (!response.ok) {
      throw new Error(`Request failed: HTTP ${response.status}`);
    }
    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      const result = await response.json();
      return result;
    }
    if (contentType.includes("text/event-stream")) {
      if (!response.body) {
        throw new Error("Streaming response has no body");
      }
      await this._readStream(response.body);
      return void 0;
    }
    throw new Error(`Unexpected Content-Type: ${contentType}`);
  }
  /**
   * Abort the current request/stream.
   */
  close() {
    if (this._abortController) {
      this._abortController.abort();
      this._abortController = null;
    }
  }
  /** Read an SSE stream and dispatch events. */
  async _readStream(body) {
    const reader = body.getReader();
    const decoder = new TextDecoder();
    const parser = createParser({
      onEvent: (event) => {
        let parsed;
        try {
          parsed = JSON.parse(event.data);
        } catch {
          parsed = event.data;
        }
        if (event.event) {
          this.onEvent(event.event, parsed);
        } else {
          this.onMessage(parsed);
        }
      }
    });
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        parser.feed(decoder.decode(value, { stream: true }));
      }
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") {
        return;
      }
      this.onError(String(err));
    } finally {
      this.onDone();
    }
  }
};
export {
  BaseWSClient,
  BinaryCodec,
  GRPCWSClient,
  JSONCodec,
  ParseError,
  ReadyState,
  SSEClient,
  StreamableClient,
  TypedGRPCWSClient,
  createMockSSEPair,
  createMockWSPair,
  createParser
};
