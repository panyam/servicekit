"use strict";
/**
 * Mock SSE utilities for testing.
 *
 * Provides a fake `fetch` implementation that returns a controllable SSE stream.
 * Follows the same DI pattern as `createMockWSPair` (inject via options).
 *
 * @example
 * ```typescript
 * import { SSEClient } from './sse-client';
 * import { createMockSSEPair } from './sse-mock';
 *
 * const { fetch, controller } = createMockSSEPair();
 * const client = new SSEClient({ fetch });
 * await client.connect('http://test/events');
 * controller.simulateMessage({ hello: 'world' });
 * controller.simulateClose();
 * ```
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.createMockSSEPair = createMockSSEPair;
/**
 * Create a mock fetch function + controller pair for testing SSE clients.
 *
 * The returned `fetch` can be passed as `SSEClientOptions.fetch`.
 * The controller lets tests push SSE events into the stream.
 *
 * @param contentType - Content-Type header for the response (default: "text/event-stream")
 */
function createMockSSEPair(contentType = 'text/event-stream') {
    let streamController = null;
    let capturedUrl;
    let capturedHeaders = {};
    let capturedMethod;
    let capturedBody;
    const encoder = new TextEncoder();
    const enqueue = (text) => {
        if (streamController) {
            streamController.enqueue(encoder.encode(text));
        }
    };
    const mockFetch = (async (input, init) => {
        capturedUrl = typeof input === 'string' ? input : input.toString();
        capturedMethod = init?.method ?? 'GET';
        capturedBody =
            typeof init?.body === 'string' ? init.body : undefined;
        // Capture headers
        capturedHeaders = {};
        if (init?.headers) {
            if (init.headers instanceof Headers) {
                init.headers.forEach((v, k) => {
                    capturedHeaders[k] = v;
                });
            }
            else if (Array.isArray(init.headers)) {
                for (const [k, v] of init.headers) {
                    capturedHeaders[k] = v;
                }
            }
            else {
                capturedHeaders = { ...init.headers };
            }
        }
        const body = new ReadableStream({
            start(controller) {
                streamController = controller;
            },
        });
        return new Response(body, {
            status: 200,
            headers: { 'Content-Type': contentType },
        });
    });
    const controller = {
        simulateMessage(data) {
            enqueue(`data: ${JSON.stringify(data)}\n\n`);
        },
        simulateEvent(event, data) {
            enqueue(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
        },
        simulateEventWithId(event, id, data) {
            enqueue(`event: ${event}\nid: ${id}\ndata: ${JSON.stringify(data)}\n\n`);
        },
        simulateComment(text) {
            enqueue(`: ${text}\n\n`);
        },
        simulateClose() {
            if (streamController) {
                streamController.close();
                streamController = null;
            }
        },
        get receivedUrl() {
            return capturedUrl;
        },
        get receivedHeaders() {
            return capturedHeaders;
        },
        get receivedMethod() {
            return capturedMethod;
        },
        get receivedBody() {
            return capturedBody;
        },
    };
    return { fetch: mockFetch, controller };
}
//# sourceMappingURL=sse-mock.js.map