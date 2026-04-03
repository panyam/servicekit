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
export function createMockSSEPair(
  contentType: string = 'text/event-stream',
): {
  fetch: typeof globalThis.fetch;
  controller: MockSSEController;
} {
  let streamController: ReadableStreamDefaultController<Uint8Array> | null = null;
  let capturedUrl: string | undefined;
  let capturedHeaders: Record<string, string> = {};
  let capturedMethod: string | undefined;
  let capturedBody: string | undefined;

  const encoder = new TextEncoder();

  const enqueue = (text: string) => {
    if (streamController) {
      streamController.enqueue(encoder.encode(text));
    }
  };

  const mockFetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
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
      } else if (Array.isArray(init.headers)) {
        for (const [k, v] of init.headers) {
          capturedHeaders[k] = v;
        }
      } else {
        capturedHeaders = { ...init.headers } as Record<string, string>;
      }
    }

    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        streamController = controller;
      },
    });

    return new Response(body, {
      status: 200,
      headers: { 'Content-Type': contentType },
    });
  }) as typeof globalThis.fetch;

  const controller: MockSSEController = {
    simulateMessage(data: unknown) {
      enqueue(`data: ${JSON.stringify(data)}\n\n`);
    },

    simulateEvent(event: string, data: unknown) {
      enqueue(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
    },

    simulateEventWithId(event: string, id: string, data: unknown) {
      enqueue(`event: ${event}\nid: ${id}\ndata: ${JSON.stringify(data)}\n\n`);
    },

    simulateComment(text: string) {
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
