/**
 * StreamableClient — Client for ServiceKit StreamableServe endpoints.
 *
 * Supports the "POST-that-optionally-streams" pattern from MCP 2025-03-26:
 * a single endpoint returns either a JSON response (application/json) or
 * an SSE event stream (text/event-stream) based on the server's decision.
 *
 * @example
 * ```typescript
 * const rpc = new StreamableClient<MyRequest, MyResponse>();
 *
 * // Single response (server returns application/json)
 * const result = await rpc.post('http://localhost:8080/rpc', { method: 'getUser' });
 * console.log(result); // MyResponse
 *
 * // Streaming response (server returns text/event-stream)
 * rpc.onEvent = (type, data) => console.log('progress:', data);
 * rpc.onDone = () => console.log('done');
 * await rpc.post('http://localhost:8080/rpc', { method: 'longOp' });
 * ```
 */

import type { MessageHandler, ErrorHandler, VoidHandler } from './types';
import { createParser } from './sse-parser';

/**
 * Options for configuring StreamableClient.
 */
export interface StreamableClientOptions {
  /**
   * Custom fetch implementation (for Node.js or testing).
   * Default: globalThis.fetch
   */
  fetch?: typeof globalThis.fetch;

  /**
   * HTTP headers to include in requests.
   * Useful for authentication tokens.
   */
  headers?: Record<string, string>;
}

/**
 * Client for StreamableServe endpoints that support both synchronous JSON
 * responses and SSE event streaming from a single POST endpoint.
 *
 * The client POSTs a JSON request body and detects the response Content-Type:
 * - `application/json` → parses body, returns as TOut
 * - `text/event-stream` → streams events via onMessage/onEvent callbacks,
 *   calls onDone when complete, returns undefined
 *
 * @see https://modelcontextprotocol.io/specification/2025-03-26/basic/transports#streamable-http
 */
export class StreamableClient<TReq = unknown, TOut = unknown> {
  private _fetch: typeof globalThis.fetch;
  private _headers: Record<string, string>;
  private _abortController: AbortController | null = null;

  /** Called when an unnamed data event is received during streaming. */
  public onMessage: MessageHandler<TOut> = () => {};

  /** Called when a named event is received during streaming. */
  public onEvent: (event: string, data: TOut) => void = () => {};

  /** Called when the SSE stream completes (channel closed by server). */
  public onDone: VoidHandler = () => {};

  /** Called when an error occurs. */
  public onError: ErrorHandler = () => {};

  constructor(options: StreamableClientOptions = {}) {
    this._fetch = options.fetch ?? globalThis.fetch;
    this._headers = options.headers ?? {};
  }

  /**
   * POST a request to a StreamableServe endpoint.
   *
   * Returns the parsed response for JSON responses, or undefined for
   * streaming responses (events dispatched via callbacks).
   *
   * @param url The endpoint URL
   * @param body The request body (JSON-serialized)
   * @returns Parsed response (JSON path) or undefined (streaming path)
   */
  async post(url: string, body: TReq): Promise<TOut | undefined> {
    this._abortController = new AbortController();

    const response = await this._fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream, application/json',
        ...this._headers,
      },
      body: JSON.stringify(body),
      signal: this._abortController.signal,
    });

    if (!response.ok) {
      throw new Error(`Request failed: HTTP ${response.status}`);
    }

    const contentType = response.headers.get('Content-Type') ?? '';

    // JSON response path (synchronous)
    if (contentType.includes('application/json')) {
      const result = (await response.json()) as TOut;
      return result;
    }

    // SSE streaming path
    if (contentType.includes('text/event-stream')) {
      if (!response.body) {
        throw new Error('Streaming response has no body');
      }
      await this._readStream(response.body);
      return undefined;
    }

    throw new Error(`Unexpected Content-Type: ${contentType}`);
  }

  /**
   * Abort the current request/stream.
   */
  close(): void {
    if (this._abortController) {
      this._abortController.abort();
      this._abortController = null;
    }
  }

  /** Read an SSE stream and dispatch events. */
  private async _readStream(body: ReadableStream<Uint8Array>): Promise<void> {
    const reader = body.getReader();
    const decoder = new TextDecoder();

    const parser = createParser({
      onEvent: (event) => {
        let parsed: TOut;
        try {
          parsed = JSON.parse(event.data) as TOut;
        } catch {
          parsed = event.data as unknown as TOut;
        }

        if (event.event) {
          this.onEvent(event.event, parsed);
        } else {
          this.onMessage(parsed);
        }
      },
    });

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        parser.feed(decoder.decode(value, { stream: true }));
      }
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') {
        return;
      }
      this.onError(String(err));
    } finally {
      this.onDone();
    }
  }
}
