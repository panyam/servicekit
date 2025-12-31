"use strict";
/**
 * GRPCWSClient - WebSocket client for gRPC-style streaming over WebSocket.
 *
 * This client handles the grpcws protocol envelope format:
 * - Wraps outgoing data in {type: "data", data: ...}
 * - Handles incoming stream_end, error, and data messages
 * - Supports client streaming (end_send) and cancellation
 *
 * Uses BaseWSClient for connection and ping/pong handling.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.GRPCWSClient = void 0;
const base_client_1 = require("./base-client");
/**
 * WebSocket client for gRPC-style streaming protocol.
 *
 * Implements the grpcws protocol with message envelope handling:
 * - Server→Client: {type: "data", data: ...}, {type: "error", error: ...}, {type: "stream_end"}
 * - Client→Server: {type: "data", data: ...}, {type: "end_send"}, {type: "cancel"}
 *
 * Supports all three gRPC streaming patterns:
 * - **Server streaming**: connect, then receive multiple messages
 * - **Client streaming**: send multiple messages, call endSend(), receive response
 * - **Bidirectional**: send and receive messages concurrently
 *
 * @example Server Streaming
 * ```typescript
 * const client = new GRPCWSClient();
 * client.onMessage = (data) => console.log('Event:', data);
 * client.onStreamEnd = () => console.log('Stream ended');
 * await client.connect('ws://localhost:8080/ws/v1/subscribe?game_id=123');
 * ```
 *
 * @example Client Streaming
 * ```typescript
 * const client = new GRPCWSClient();
 * client.onMessage = (summary) => console.log('Result:', summary);
 * await client.connect('ws://localhost:8080/ws/v1/commands');
 * client.send({ commandId: '1', commandType: 'move' });
 * client.send({ commandId: '2', commandType: 'attack' });
 * client.endSend(); // Server responds after this
 * ```
 *
 * @example Bidirectional Streaming
 * ```typescript
 * const client = new GRPCWSClient();
 * client.onMessage = (state) => updateUI(state);
 * await client.connect('ws://localhost:8080/ws/v1/sync');
 * client.send({ actionId: '1', move: { x: 10, y: 20 } });
 * // Receives responses concurrently
 * ```
 */
class GRPCWSClient {
    constructor(options = {}) {
        /** Called when a data message is received */
        this.onMessage = () => { };
        /** Called when the stream ends normally */
        this.onStreamEnd = () => { };
        /** Called when the server sends an error */
        this.onError = () => { };
        /** Called when the connection closes */
        this.onClose = () => { };
        /** Called when a ping is received */
        this.onPing = () => { };
        this.base = new base_client_1.BaseWSClient(options);
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
            type: 'data',
            data,
        };
        this.base.send(envelope);
    }
    /**
     * Signal that the client is done sending (half-close).
     * Used in client streaming and bidirectional streaming to indicate
     * the client won't send any more messages.
     */
    endSend() {
        this.base.send({ type: 'end_send' });
    }
    /**
     * Cancel the stream.
     * Signals to the server that the client wants to terminate the stream.
     */
    cancel() {
        this.base.send({ type: 'cancel' });
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
            // Not a valid control message, ignore or pass through
            console.warn('Received non-envelope message:', msg);
            return;
        }
        const controlMsg = msg;
        switch (controlMsg.type) {
            case 'data':
                this.onMessage(controlMsg.data);
                break;
            case 'stream_end':
                this.onStreamEnd();
                break;
            case 'error':
                this.onError(controlMsg.error ?? 'Unknown error');
                break;
            default:
                // Unknown message type, ignore
                console.warn('Unknown message type:', controlMsg.type);
        }
    }
    /**
     * Check if a message is a valid control message.
     */
    isControlMessage(msg) {
        return (typeof msg === 'object' &&
            msg !== null &&
            'type' in msg &&
            typeof msg.type === 'string');
    }
}
exports.GRPCWSClient = GRPCWSClient;
//# sourceMappingURL=grpcws-client.js.map