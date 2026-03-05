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
import { ClientOptions, MessageHandler, ErrorHandler, VoidHandler } from './types';
/**
 * Mock controller for testing GRPCWSClient without real WebSocket connections.
 *
 * The controller wraps/unwraps the servicekit envelope automatically —
 * consumers only see application-level data.
 *
 * @example
 * ```typescript
 * const { client, controller } = GRPCWSClient.createMock();
 * client.onMessage = (data) => received.push(data);
 * client.connect('ws://test');
 * controller.simulateOpen();
 * controller.simulateMessage({ event: 'joined' });
 * expect(received).toEqual([{ event: 'joined' }]);
 * ```
 */
export interface MockController {
    /** Messages sent by the client (already unwrapped from envelope) */
    readonly sentMessages: unknown[];
    /** Simulate WebSocket open — resolves the connect() Promise */
    simulateOpen(): void;
    /** Simulate receiving a data message (auto-wraps in {type: "data"} envelope) */
    simulateMessage(data: unknown): void;
    /** Simulate a server error (delivers {type: "error"} envelope to onError) */
    simulateError(message?: string): void;
    /** Simulate WebSocket close */
    simulateClose(code?: number): void;
    /** Current WebSocket ready state */
    readonly readyState: number;
}
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
export declare class GRPCWSClient {
    private base;
    /** Called when a data message is received */
    onMessage: MessageHandler;
    /** Called when the stream ends normally */
    onStreamEnd: VoidHandler;
    /** Called when the server sends an error */
    onError: ErrorHandler;
    /** Called when the connection closes */
    onClose: VoidHandler;
    /** Called when a ping is received */
    onPing: (pingId: number) => void;
    constructor(options?: ClientOptions);
    /**
     * Connect to a grpcws WebSocket server.
     * @param url The WebSocket URL to connect to
     * @returns Promise that resolves when connected
     */
    connect(url: string): Promise<void>;
    /**
     * Send data to the server.
     * The data is wrapped in a {type: "data", data: ...} envelope.
     * @param data The data payload to send
     */
    send(data: unknown): void;
    /**
     * Signal that the client is done sending (half-close).
     * Used in client streaming and bidirectional streaming to indicate
     * the client won't send any more messages.
     */
    endSend(): void;
    /**
     * Cancel the stream.
     * Signals to the server that the client wants to terminate the stream.
     */
    cancel(): void;
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
    static createMock(): {
        client: GRPCWSClient;
        controller: MockController;
    };
    /**
     * Set up handlers on the base client to process grpcws envelope messages.
     */
    private setupBaseHandlers;
    /**
     * Handle an incoming envelope message.
     */
    private handleEnvelopeMessage;
    /**
     * Check if a message is a valid control message.
     */
    private isControlMessage;
}
//# sourceMappingURL=grpcws-client.d.ts.map