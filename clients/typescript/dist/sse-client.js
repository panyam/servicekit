"use strict";
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
Object.defineProperty(exports, "__esModule", { value: true });
exports.SSEClient = void 0;
const sse_parser_1 = require("./sse-parser");
const sse_mock_1 = require("./sse-mock");
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
class SSEClient {
    constructor(options = {}) {
        this._connected = false;
        this._abortController = null;
        /** Called when an unnamed data event is received (no "event:" field). */
        this.onMessage = () => { };
        /** Called when a named event is received (has "event:" field). */
        this.onEvent = () => { };
        /** Called when the SSE stream closes (server closes or network error). */
        this.onClose = () => { };
        /** Called when an error occurs (fetch failure, parse error). */
        this.onError = () => { };
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
    async connect(url) {
        const headers = { ...this._headers };
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
    close() {
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
    get isConnected() {
        return this._connected;
    }
    /** The last event ID received from the server (for reconnection). */
    get lastEventId() {
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
    static createMock() {
        const { fetch, controller } = (0, sse_mock_1.createMockSSEPair)();
        const client = new SSEClient({ fetch });
        return { client, controller };
    }
    /** Read the SSE stream and dispatch events via the parser. */
    async _readStream(body) {
        const reader = body.getReader();
        const decoder = new TextDecoder();
        const parser = (0, sse_parser_1.createParser)({
            onEvent: (event) => {
                // Track last event ID for reconnection
                if (event.id !== undefined) {
                    this._lastEventId = event.id;
                }
                // Parse JSON data
                let parsed;
                try {
                    parsed = JSON.parse(event.data);
                }
                catch {
                    // If not valid JSON, pass raw string as-is
                    parsed = event.data;
                }
                // Dispatch to appropriate handler
                if (event.event) {
                    this.onEvent(event.event, parsed);
                }
                else {
                    this.onMessage(parsed);
                }
            },
            // Comments (keepalive) are silently consumed — no handler needed
        });
        try {
            while (true) {
                const { done, value } = await reader.read();
                if (done)
                    break;
                parser.feed(decoder.decode(value, { stream: true }));
            }
        }
        catch (err) {
            // AbortError is expected when close() is called
            if (err instanceof Error && err.name === 'AbortError') {
                return;
            }
            this.onError(String(err));
        }
        finally {
            if (this._connected) {
                this._connected = false;
                this.onClose();
            }
        }
    }
}
exports.SSEClient = SSEClient;
//# sourceMappingURL=sse-client.js.map