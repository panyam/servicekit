/**
 * SSEClient — Server-Sent Events client for ServiceKit SSEServe endpoints.
 *
 * Uses `fetch` + `ReadableStream` for SSE consumption (not the browser's
 * native `EventSource`, which is GET-only and doesn't support custom headers).
 *
 * Follows the same patterns as BaseWSClient:
 * - Event handler properties (onMessage, onError, onClose)
 * - Dependency injection for fetch (SSEClientOptions.fetch)
 * - Static createMock() for testing
 *
 * @example
 * ```typescript
 * const client = new SSEClient<MyEvent>();
 * client.onMessage = (data) => console.log('Received:', data);
 * client.onEvent = (type, data) => console.log('Event:', type, data);
 * await client.connect('http://localhost:8080/events');
 * ```
 */

import type { MessageHandler, ErrorHandler, VoidHandler } from './types';
import { createParser } from './sse-parser';
import { createMockSSEPair, MockSSEController } from './sse-mock';

/**
 * Options for configuring SSEClient.
 */
export interface SSEClientOptions {
  /**
   * Custom fetch implementation (for Node.js or testing).
   * Default: globalThis.fetch
   */
  fetch?: typeof globalThis.fetch;

  /**
   * HTTP headers to include in the SSE request.
   * Useful for authentication tokens.
   */
  headers?: Record<string, string>;

  /**
   * Initial Last-Event-ID for reconnection.
   * If set, the client sends this as the Last-Event-ID header on connect.
   */
  lastEventId?: string;
}

/**
 * SSE client for long-lived server-sent event streams.
 *
 * Connects to SSEServe endpoints via HTTP GET, parses the SSE stream using
 * a vendored WHATWG-compliant parser, and dispatches events to handlers.
 *
 * Features:
 * - Custom headers (auth tokens) — unlike native EventSource
 * - POST support — unlike native EventSource
 * - Keepalive comments silently consumed
 * - Last-Event-ID tracking for reconnection
 * - Mock utilities for testing (SSEClient.createMock())
 *
 * @example
 * ```typescript
 * const client = new SSEClient<{ status: string }>();
 * client.onMessage = (data) => console.log(data.status);
 * client.onEvent = (type, data) => console.log(type, data);
 * client.onClose = () => console.log('Stream ended');
 * await client.connect('http://localhost:8080/events', {
 *   headers: { Authorization: 'Bearer token' },
 * });
 * ```
 */
export class SSEClient<O = unknown> {
  private _fetch: typeof globalThis.fetch;
  private _headers: Record<string, string>;
  private _lastEventId: string | undefined;
  private _connected = false;
  private _abortController: AbortController | null = null;

  /** Called when an unnamed data event is received (no "event:" field). */
  public onMessage: MessageHandler<O> = () => {};

  /** Called when a named event is received (has "event:" field). */
  public onEvent: (event: string, data: O) => void = () => {};

  /** Called when the SSE stream closes (server closes or network error). */
  public onClose: VoidHandler = () => {};

  /** Called when an error occurs (fetch failure, parse error). */
  public onError: ErrorHandler = () => {};

  constructor(options: SSEClientOptions = {}) {
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
  async connect(url: string): Promise<void> {
    const headers: Record<string, string> = { ...this._headers };
    if (this._lastEventId) {
      headers['Last-Event-ID'] = this._lastEventId;
    }

    this._abortController = new AbortController();

    const response = await this._fetch(url, {
      headers,
      signal: this._abortController.signal,
    });

    if (!response.ok) {
      throw new Error(`SSE connection failed: HTTP ${response.status}`);
    }

    const contentType = response.headers.get('Content-Type') ?? '';
    if (!contentType.includes('text/event-stream')) {
      throw new Error(`Expected text/event-stream, got ${contentType}`);
    }

    if (!response.body) {
      throw new Error('Response has no body');
    }

    this._connected = true;

    // Start reading the stream in the background
    this._readStream(response.body);
  }

  /**
   * Close the SSE connection.
   * Aborts the underlying fetch request and fires onClose.
   */
  close(): void {
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
  get isConnected(): boolean {
    return this._connected;
  }

  /** The last event ID received from the server (for reconnection). */
  get lastEventId(): string | undefined {
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
  static createMock<O = unknown>(): {
    client: SSEClient<O>;
    controller: MockSSEController;
  } {
    const { fetch, controller } = createMockSSEPair();
    const client = new SSEClient<O>({ fetch });
    return { client, controller };
  }

  /** Read the SSE stream and dispatch events via the parser. */
  private async _readStream(body: ReadableStream<Uint8Array>): Promise<void> {
    const reader = body.getReader();
    const decoder = new TextDecoder();

    const parser = createParser({
      onEvent: (event) => {
        // Track last event ID for reconnection
        if (event.id !== undefined) {
          this._lastEventId = event.id;
        }

        // Parse JSON data
        let parsed: O;
        try {
          parsed = JSON.parse(event.data) as O;
        } catch {
          // If not valid JSON, pass raw string as-is
          parsed = event.data as unknown as O;
        }

        // Dispatch to appropriate handler
        if (event.event) {
          this.onEvent(event.event, parsed);
        } else {
          this.onMessage(parsed);
        }
      },
      // Comments (keepalive) are silently consumed — no handler needed
    });

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        parser.feed(decoder.decode(value, { stream: true }));
      }
    } catch (err) {
      // AbortError is expected when close() is called
      if (err instanceof Error && err.name === 'AbortError') {
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
}
