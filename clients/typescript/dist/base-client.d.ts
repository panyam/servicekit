/**
 * BaseWSClient - Low-level WebSocket client with automatic ping/pong handling.
 *
 * This client handles the connection lifecycle and heartbeat mechanism.
 * It sends and receives raw JSON messages without any envelope wrapping.
 *
 * Use this directly for http/JSONConn servers, or as the base for GRPCWSClient.
 */
import { ClientOptions, MessageHandler, ErrorHandler, VoidHandler } from './types';
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
export declare class BaseWSClient {
    private ws;
    private options;
    /** Called when a message is received (excluding ping messages) */
    onMessage: MessageHandler;
    /** Called when a ping is received (after auto-pong if enabled) */
    onPing: (pingId: number) => void;
    /** Called when the connection closes */
    onClose: VoidHandler;
    /** Called when a WebSocket error occurs */
    onError: ErrorHandler;
    constructor(options?: ClientOptions);
    /**
     * Connect to a WebSocket server.
     * @param url The WebSocket URL to connect to
     * @returns Promise that resolves when connected
     */
    connect(url: string): Promise<void>;
    /**
     * Send a raw JSON message to the server.
     * @param data The data to send (will be JSON.stringify'd)
     */
    send(data: unknown): void;
    /**
     * Send a raw string message to the server (no JSON encoding).
     * @param message The raw string to send
     */
    sendRaw(message: string): void;
    /**
     * Close the WebSocket connection.
     */
    close(): void;
    /**
     * Check if the client is connected.
     */
    get isConnected(): boolean;
    /**
     * Get the WebSocket ready state.
     */
    get readyState(): number;
    /**
     * Handle incoming raw message data.
     * Parses JSON and handles ping/pong automatically.
     */
    private handleRawMessage;
    /**
     * Check if a message is a ping message.
     */
    private isPingMessage;
    /**
     * Send a pong response.
     */
    private sendPong;
}
//# sourceMappingURL=base-client.d.ts.map