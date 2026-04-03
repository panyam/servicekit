"use strict";
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
Object.defineProperty(exports, "__esModule", { value: true });
exports.StreamableClient = void 0;
const sse_parser_1 = require("./sse-parser");
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
class StreamableClient {
    constructor(options = {}) {
        this._abortController = null;
        /** Called when an unnamed data event is received during streaming. */
        this.onMessage = () => { };
        /** Called when a named event is received during streaming. */
        this.onEvent = () => { };
        /** Called when the SSE stream completes (channel closed by server). */
        this.onDone = () => { };
        /** Called when an error occurs. */
        this.onError = () => { };
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
    async post(url, body) {
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
            const result = (await response.json());
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
    close() {
        if (this._abortController) {
            this._abortController.abort();
            this._abortController = null;
        }
    }
    /** Read an SSE stream and dispatch events. */
    async _readStream(body) {
        const reader = body.getReader();
        const decoder = new TextDecoder();
        const parser = (0, sse_parser_1.createParser)({
            onEvent: (event) => {
                let parsed;
                try {
                    parsed = JSON.parse(event.data);
                }
                catch {
                    parsed = event.data;
                }
                if (event.event) {
                    this.onEvent(event.event, parsed);
                }
                else {
                    this.onMessage(parsed);
                }
            },
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
            if (err instanceof Error && err.name === 'AbortError') {
                return;
            }
            this.onError(String(err));
        }
        finally {
            this.onDone();
        }
    }
}
exports.StreamableClient = StreamableClient;
//# sourceMappingURL=streamable-client.js.map