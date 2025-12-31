"use strict";
var ServiceKit = (() => {
  var __defProp = Object.defineProperty;
  var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
  var __getOwnPropNames = Object.getOwnPropertyNames;
  var __hasOwnProp = Object.prototype.hasOwnProperty;
  var __export = (target, all) => {
    for (var name in all)
      __defProp(target, name, { get: all[name], enumerable: true });
  };
  var __copyProps = (to, from, except, desc) => {
    if (from && typeof from === "object" || typeof from === "function") {
      for (let key of __getOwnPropNames(from))
        if (!__hasOwnProp.call(to, key) && key !== except)
          __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
    }
    return to;
  };
  var __toCommonJS = (mod) => __copyProps(__defProp({}, "__esModule", { value: true }), mod);

  // src/index.ts
  var index_exports = {};
  __export(index_exports, {
    BaseWSClient: () => BaseWSClient,
    BinaryCodec: () => BinaryCodec,
    GRPCWSClient: () => GRPCWSClient,
    JSONCodec: () => JSONCodec,
    ReadyState: () => ReadyState,
    TypedGRPCWSClient: () => TypedGRPCWSClient
  });

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

  // src/grpcws-client.ts
  var GRPCWSClient = class {
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
  return __toCommonJS(index_exports);
})();
