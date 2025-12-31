/**
 * BaseWSClient - Low-level WebSocket client with automatic ping/pong handling.
 *
 * This client handles the connection lifecycle and heartbeat mechanism.
 * It sends and receives raw JSON messages without any envelope wrapping.
 *
 * Use this directly for http/JSONConn servers, or as the base for GRPCWSClient.
 */

import {
  ControlMessage,
  ClientOptions,
  Codec,
  JSONCodec,
  MessageHandler,
  ErrorHandler,
  VoidHandler,
  ReadyState,
} from './types';

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
export class BaseWSClient<I = unknown, O = unknown> {
  private ws: WebSocket | null = null;
  private _codec: Codec<I, O>;
  private _autoPong: boolean;
  private _WebSocket: typeof WebSocket;

  /** Called when a data message is received (decoded by codec) */
  public onMessage: MessageHandler<I> = () => {};

  /** Called when a ping is received (after auto-pong if enabled) */
  public onPing: (pingId: number) => void = () => {};

  /** Called when the connection closes */
  public onClose: VoidHandler = () => {};

  /** Called when a WebSocket error occurs */
  public onError: ErrorHandler = () => {};

  constructor(options: ClientOptions<I, O> = {}) {
    this._autoPong = options.autoPong ?? true;
    this._WebSocket = options.WebSocket ?? globalThis.WebSocket;
    this._codec = options.codec ?? (new JSONCodec() as unknown as Codec<I, O>);
  }

  /** Get the codec used for encoding/decoding data messages */
  get codec(): Codec<I, O> {
    return this._codec;
  }

  /**
   * Connect to a WebSocket server.
   * @param url The WebSocket URL to connect to
   * @returns Promise that resolves when connected
   */
  connect(url: string): Promise<void> {
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
  send(data: O): void {
    if (!this.ws || this.ws.readyState !== ReadyState.OPEN) {
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
  sendRaw(message: string | ArrayBuffer): void {
    if (!this.ws || this.ws.readyState !== ReadyState.OPEN) {
      throw new Error('WebSocket is not connected');
    }
    this.ws.send(message);
  }

  /**
   * Close the WebSocket connection.
   */
  close(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  /**
   * Check if the client is connected.
   */
  get isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === ReadyState.OPEN;
  }

  /**
   * Get the WebSocket ready state.
   */
  get readyState(): number {
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
  private handleRawMessage(data: string | ArrayBuffer): void {
    // Binary frame -> decode with codec directly
    if (data instanceof ArrayBuffer) {
      try {
        const decoded = this._codec.decode(data);
        this.onMessage(decoded);
      } catch (err) {
        this.onError(`Failed to decode binary message: ${err}`);
      }
      return;
    }

    // Text frame -> check for control messages first
    let parsed: unknown;
    try {
      parsed = JSON.parse(data);
    } catch {
      // Not valid JSON, try to decode with codec
      try {
        const decoded = this._codec.decode(data);
        this.onMessage(decoded);
      } catch (err) {
        this.onError(`Failed to decode text message: ${err}`);
      }
      return;
    }

    // Check if it's a ping message (control message)
    if (this.isPingMessage(parsed)) {
      const pingId = (parsed as ControlMessage).pingId;
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
    } catch (err) {
      this.onError(`Failed to decode message: ${err}`);
    }
  }

  /**
   * Check if a message is a ping message.
   * Pings are always JSON with type: "ping".
   */
  private isPingMessage(msg: unknown): boolean {
    return (
      typeof msg === 'object' &&
      msg !== null &&
      'type' in msg &&
      (msg as ControlMessage).type === 'ping'
    );
  }

  /**
   * Send a pong response.
   * Pongs are always JSON, bypassing the codec.
   */
  private sendPong(pingId: number): void {
    this.sendRaw(JSON.stringify({ type: 'pong', pingId }));
  }
}
