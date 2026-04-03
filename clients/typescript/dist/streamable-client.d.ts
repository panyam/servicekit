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
export declare class StreamableClient<TReq = unknown, TOut = unknown> {
    private _fetch;
    private _headers;
    private _abortController;
    /** Called when an unnamed data event is received during streaming. */
    onMessage: MessageHandler<TOut>;
    /** Called when a named event is received during streaming. */
    onEvent: (event: string, data: TOut) => void;
    /** Called when the SSE stream completes (channel closed by server). */
    onDone: VoidHandler;
    /** Called when an error occurs. */
    onError: ErrorHandler;
    constructor(options?: StreamableClientOptions);
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
    post(url: string, body: TReq): Promise<TOut | undefined>;
    /**
     * Abort the current request/stream.
     */
    close(): void;
    /** Read an SSE stream and dispatch events. */
    private _readStream;
}
//# sourceMappingURL=streamable-client.d.ts.map