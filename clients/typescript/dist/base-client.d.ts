/**
 * BaseWSClient - Low-level WebSocket client with automatic ping/pong handling.
 *
 * This client handles the connection lifecycle and heartbeat mechanism.
 * It sends and receives raw JSON messages without any envelope wrapping.
 *
 * Use this directly for http/JSONConn servers, or as the base for GRPCWSClient.
 */
import { ClientOptions, Codec, MessageHandler, ErrorHandler, VoidHandler } from './types';
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
export declare class BaseWSClient<I = unknown, O = unknown> {
    private ws;
    private _codec;
    private _autoPong;
    private _WebSocket;
    /** Called when a data message is received (decoded by codec) */
    onMessage: MessageHandler<I>;
    /** Called when a ping is received (after auto-pong if enabled) */
    onPing: (pingId: number) => void;
    /** Called when the connection closes */
    onClose: VoidHandler;
    /** Called when a WebSocket error occurs */
    onError: ErrorHandler;
    constructor(options?: ClientOptions<I, O>);
    /** Get the codec used for encoding/decoding data messages */
    get codec(): Codec<I, O>;
    /**
     * Connect to a WebSocket server.
     * @param url The WebSocket URL to connect to
     * @returns Promise that resolves when connected
     */
    connect(url: string): Promise<void>;
    /**
     * Send a data message to the server using the configured codec.
     * @param data The data to send (will be encoded by codec)
     */
    send(data: O): void;
    /**
     * Send a raw message to the server (bypasses codec).
     * Useful for control messages like pong.
     * @param message The raw string or ArrayBuffer to send
     */
    sendRaw(message: string | ArrayBuffer): void;
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
     * - Text frames: Check for control messages (ping), then decode with codec
     * - Binary frames: Decode directly with codec
     *
     * Control messages (ping/pong/error) are always JSON text frames,
     * regardless of what codec is used for data messages.
     */
    private handleRawMessage;
    /**
     * Check if a message is a ping message.
     * Pings are always JSON with type: "ping".
     */
    private isPingMessage;
    /**
     * Send a pong response.
     * Pongs are always JSON, bypassing the codec.
     */
    private sendPong;
}
//# sourceMappingURL=base-client.d.ts.map