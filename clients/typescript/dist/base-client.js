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
        /** Called when a message is received (excluding ping messages) */
        this.onMessage = () => { };
        /** Called when a ping is received (after auto-pong if enabled) */
        this.onPing = () => { };
        /** Called when the connection closes */
        this.onClose = () => { };
        /** Called when a WebSocket error occurs */
        this.onError = () => { };
        this.options = {
            autoPong: options.autoPong ?? true,
            WebSocket: options.WebSocket ?? globalThis.WebSocket,
        };
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
                this.ws = new this.options.WebSocket(url);
            }
            catch (error) {
                reject(error);
                return;
            }
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
     * Send a raw JSON message to the server.
     * @param data The data to send (will be JSON.stringify'd)
     */
    send(data) {
        if (!this.ws || this.ws.readyState !== types_1.ReadyState.OPEN) {
            throw new Error('WebSocket is not connected');
        }
        this.ws.send(JSON.stringify(data));
    }
    /**
     * Send a raw string message to the server (no JSON encoding).
     * @param message The raw string to send
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
     * Parses JSON and handles ping/pong automatically.
     */
    handleRawMessage(data) {
        let msg;
        try {
            msg = JSON.parse(data);
        }
        catch {
            // Not valid JSON, pass through as-is
            this.onMessage(data);
            return;
        }
        // Check if it's a ping message
        if (this.isPingMessage(msg)) {
            const pingId = msg.pingId;
            if (this.options.autoPong && pingId !== undefined) {
                this.sendPong(pingId);
            }
            this.onPing(pingId ?? 0);
            return;
        }
        // Pass through all other messages
        this.onMessage(msg);
    }
    /**
     * Check if a message is a ping message.
     */
    isPingMessage(msg) {
        return (typeof msg === 'object' &&
            msg !== null &&
            'type' in msg &&
            msg.type === 'ping');
    }
    /**
     * Send a pong response.
     */
    sendPong(pingId) {
        this.send({ type: 'pong', pingId });
    }
}
exports.BaseWSClient = BaseWSClient;
//# sourceMappingURL=base-client.js.map