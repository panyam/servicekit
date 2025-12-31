"use strict";
/**
 * BaseWSClient - Low-level WebSocket client with automatic ping/pong handling.
 *
 * This client handles the connection lifecycle and heartbeat mechanism.
 * It sends and receives raw JSON messages without any envelope wrapping.
 *
 * Use this directly for http/JSONConn servers, or as the base for GRPCWSClient.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.BaseWSClient = void 0;
const types_1 = require("./types");
/**
 * Base WebSocket client with automatic ping/pong heartbeat handling.
 *
 * Features:
 * - Automatic ping/pong response (keeps connection alive)
 * - Promise-based connect()
 * - Event handlers for messages, errors, and close
 * - Connection state tracking
 *
 * @example
 * ```typescript
 * const client = new BaseWSClient();
 * client.onMessage = (data) => console.log('Received:', data);
 * client.onClose = () => console.log('Disconnected');
 * await client.connect('ws://localhost:8080/ws');
 * client.send({ hello: 'world' });
 * ```
 */
class BaseWSClient {
    constructor(options = {}) {
        this.ws = null;
        /** Called when a data message is received (decoded by codec) */
        this.onMessage = () => { };
        /** Called when a ping is received (after auto-pong if enabled) */
        this.onPing = () => { };
        /** Called when the connection closes */
        this.onClose = () => { };
        /** Called when a WebSocket error occurs */
        this.onError = () => { };
        this._autoPong = options.autoPong ?? true;
        this._WebSocket = options.WebSocket ?? globalThis.WebSocket;
        this._codec = options.codec ?? new types_1.JSONCodec();
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
            if (this.ws && this.ws.readyState === types_1.ReadyState.OPEN) {
                resolve();
                return;
            }
            try {
                this.ws = new this._WebSocket(url);
            }
            catch (error) {
                reject(error);
                return;
            }
            // Set binary type for proper ArrayBuffer handling
            this.ws.binaryType = 'arraybuffer';
            this.ws.onopen = () => {
                resolve();
            };
            this.ws.onerror = (event) => {
                const errorMsg = 'WebSocket error';
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
        if (!this.ws || this.ws.readyState !== types_1.ReadyState.OPEN) {
            throw new Error('WebSocket is not connected');
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
        if (!this.ws || this.ws.readyState !== types_1.ReadyState.OPEN) {
            throw new Error('WebSocket is not connected');
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
        return this.ws !== null && this.ws.readyState === types_1.ReadyState.OPEN;
    }
    /**
     * Get the WebSocket ready state.
     */
    get readyState() {
        return this.ws?.readyState ?? types_1.ReadyState.CLOSED;
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
        // Binary frame -> decode with codec directly
        if (data instanceof ArrayBuffer) {
            try {
                const decoded = this._codec.decode(data);
                this.onMessage(decoded);
            }
            catch (err) {
                this.onError(`Failed to decode binary message: ${err}`);
            }
            return;
        }
        // Text frame -> check for control messages first
        let parsed;
        try {
            parsed = JSON.parse(data);
        }
        catch {
            // Not valid JSON, try to decode with codec
            try {
                const decoded = this._codec.decode(data);
                this.onMessage(decoded);
            }
            catch (err) {
                this.onError(`Failed to decode text message: ${err}`);
            }
            return;
        }
        // Check if it's a ping message (control message)
        if (this.isPingMessage(parsed)) {
            const pingId = parsed.pingId;
            if (this._autoPong && pingId !== undefined) {
                this.sendPong(pingId);
            }
            this.onPing(pingId ?? 0);
            return;
        }
        // Not a control message -> decode with codec
        // For JSON codec, the parsed object is already decoded
        // For other codecs, we pass the raw string
        try {
            const decoded = this._codec.decode(data);
            this.onMessage(decoded);
        }
        catch (err) {
            this.onError(`Failed to decode message: ${err}`);
        }
    }
    /**
     * Check if a message is a ping message.
     * Pings are always JSON with type: "ping".
     */
    isPingMessage(msg) {
        return (typeof msg === 'object' &&
            msg !== null &&
            'type' in msg &&
            msg.type === 'ping');
    }
    /**
     * Send a pong response.
     * Pongs are always JSON, bypassing the codec.
     */
    sendPong(pingId) {
        this.sendRaw(JSON.stringify({ type: 'pong', pingId }));
    }
}
exports.BaseWSClient = BaseWSClient;
//# sourceMappingURL=base-client.js.map