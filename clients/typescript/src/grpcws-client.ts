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

import { BaseWSClient } from './base-client';
import { createMockWSPair } from './mock';
import {
  ControlMessage,
  ClientOptions,
  MessageHandler,
  ErrorHandler,
  VoidHandler,
} from './types';

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
export class GRPCWSClient {
  private base: BaseWSClient;

  /** Called when a data message is received */
  public onMessage: MessageHandler = () => {};

  /** Called when the stream ends normally */
  public onStreamEnd: VoidHandler = () => {};

  /** Called when the server sends an error */
  public onError: ErrorHandler = () => {};

  /** Called when the connection closes */
  public onClose: VoidHandler = () => {};

  /** Called when a ping is received */
  public onPing: (pingId: number) => void = () => {};

  constructor(options: ClientOptions = {}) {
    this.base = new BaseWSClient(options);
    this.setupBaseHandlers();
  }

  /**
   * Connect to a grpcws WebSocket server.
   * @param url The WebSocket URL to connect to
   * @returns Promise that resolves when connected
   */
  connect(url: string): Promise<void> {
    return this.base.connect(url);
  }

  /**
   * Send data to the server.
   * The data is wrapped in a {type: "data", data: ...} envelope.
   * @param data The data payload to send
   */
  send(data: unknown): void {
    const envelope: ControlMessage = {
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
  endSend(): void {
    this.base.send({ type: 'end_send' });
  }

  /**
   * Cancel the stream.
   * Signals to the server that the client wants to terminate the stream.
   */
  cancel(): void {
    this.base.send({ type: 'cancel' });
  }

  /**
   * Close the WebSocket connection.
   */
  close(): void {
    this.base.close();
  }

  /**
   * Check if the client is connected.
   */
  get isConnected(): boolean {
    return this.base.isConnected;
  }

  /**
   * Get the WebSocket ready state.
   */
  get readyState(): number {
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
  static createMock(): { client: GRPCWSClient; controller: MockController } {
    const { WebSocket, controller: wsCtrl } = createMockWSPair();
    const client = new GRPCWSClient({ WebSocket });

    const controller: MockController = {
      get sentMessages(): unknown[] {
        const messages: unknown[] = [];
        for (const raw of wsCtrl.sentRaw) {
          try {
            const parsed = JSON.parse(raw as string);
            if (parsed.type === 'data') {
              messages.push(parsed.data);
            }
          } catch {
            // Skip non-JSON messages (e.g. binary)
          }
        }
        return messages;
      },

      simulateOpen() {
        wsCtrl.simulateOpen();
      },

      simulateMessage(data: unknown) {
        wsCtrl.simulateRawMessage(JSON.stringify({ type: 'data', data }));
      },

      simulateError(message?: string) {
        wsCtrl.simulateRawMessage(
          JSON.stringify({ type: 'error', error: message ?? 'Mock error' })
        );
      },

      simulateClose(code?: number) {
        wsCtrl.simulateClose(code);
      },

      get readyState(): number {
        return wsCtrl.readyState;
      },
    };

    return { client, controller };
  }

  /**
   * Set up handlers on the base client to process grpcws envelope messages.
   */
  private setupBaseHandlers(): void {
    this.base.onMessage = (msg: unknown) => {
      this.handleEnvelopeMessage(msg);
    };

    this.base.onClose = () => {
      this.onClose();
    };

    this.base.onError = (error: string) => {
      this.onError(error);
    };

    this.base.onPing = (pingId: number) => {
      this.onPing(pingId);
    };
  }

  /**
   * Handle an incoming envelope message.
   */
  private handleEnvelopeMessage(msg: unknown): void {
    if (!this.isControlMessage(msg)) {
      // Not a valid control message, ignore or pass through
      console.warn('Received non-envelope message:', msg);
      return;
    }

    const controlMsg = msg as ControlMessage;

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
  private isControlMessage(msg: unknown): boolean {
    return (
      typeof msg === 'object' &&
      msg !== null &&
      'type' in msg &&
      typeof (msg as ControlMessage).type === 'string'
    );
  }
}
