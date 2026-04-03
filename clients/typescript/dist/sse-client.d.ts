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
import { MockSSEController } from './sse-mock';
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
export declare class SSEClient<O = unknown> {
    private _fetch;
    private _headers;
    private _lastEventId;
    private _connected;
    private _abortController;
    /** Called when an unnamed data event is received (no "event:" field). */
    onMessage: MessageHandler<O>;
    /** Called when a named event is received (has "event:" field). */
    onEvent: (event: string, data: O) => void;
    /** Called when the SSE stream closes (server closes or network error). */
    onClose: VoidHandler;
    /** Called when an error occurs (fetch failure, parse error). */
    onError: ErrorHandler;
    constructor(options?: SSEClientOptions);
    /**
     * Connect to an SSE endpoint.
     *
     * @param url The HTTP URL to connect to
     * @returns Promise that resolves when the connection is established (headers received)
     * @throws Error if the response is not 200 or not text/event-stream
     */
    connect(url: string): Promise<void>;
    /**
     * Close the SSE connection.
     * Aborts the underlying fetch request and fires onClose.
     */
    close(): void;
    /** Whether the client is currently connected to an SSE stream. */
    get isConnected(): boolean;
    /** The last event ID received from the server (for reconnection). */
    get lastEventId(): string | undefined;
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
    };
    /** Read the SSE stream and dispatch events via the parser. */
    private _readStream;
}
//# sourceMappingURL=sse-client.d.ts.map