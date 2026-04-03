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
/**
 * Controller for driving a mock SSE stream in tests.
 *
 * Operates at the SSE protocol level — messages are automatically
 * formatted as SSE events before being enqueued to the stream.
 */
export interface MockSSEController {
    /** Simulate receiving an unnamed data event (no "event:" field). */
    simulateMessage(data: unknown): void;
    /** Simulate receiving a named event with "event:" field. */
    simulateEvent(event: string, data: unknown): void;
    /** Simulate receiving a named event with "event:" and "id:" fields. */
    simulateEventWithId(event: string, id: string, data: unknown): void;
    /** Simulate receiving a keepalive comment (": keepalive"). */
    simulateComment(text: string): void;
    /** Close the SSE stream (fires onClose on the client). */
    simulateClose(): void;
    /** The URL that was passed to fetch(). */
    readonly receivedUrl: string | undefined;
    /** The headers that were passed to fetch(). */
    readonly receivedHeaders: Record<string, string>;
    /** The HTTP method used (GET or POST). */
    readonly receivedMethod: string | undefined;
    /** The request body (for POST requests). */
    readonly receivedBody: string | undefined;
}
/**
 * Create a mock fetch function + controller pair for testing SSE clients.
 *
 * The returned `fetch` can be passed as `SSEClientOptions.fetch`.
 * The controller lets tests push SSE events into the stream.
 *
 * @param contentType - Content-Type header for the response (default: "text/event-stream")
 */
export declare function createMockSSEPair(contentType?: string): {
    fetch: typeof globalThis.fetch;
    controller: MockSSEController;
};
//# sourceMappingURL=sse-mock.d.ts.map