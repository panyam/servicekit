import { describe, it, expect, vi } from 'vitest';
import { createParser, EventSourceMessage } from '../sse-parser';
import { SSEClient } from '../sse-client';
import { StreamableClient } from '../streamable-client';
import { createMockSSEPair } from '../sse-mock';

// ============================================================================
// SSE Parser Tests
// ============================================================================

describe('SSEParser', () => {
  /**
   * Verifies basic SSE data event parsing: a single "data:" line followed
   * by a blank line should fire onEvent with the data content.
   */
  it('parses basic data event', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('data: {"msg":"hello"}\n\n');

    expect(events).toHaveLength(1);
    expect(events[0].data).toBe('{"msg":"hello"}');
    expect(events[0].event).toBeUndefined();
  });

  /**
   * Verifies named event parsing: "event:" field sets the event type,
   * which clients use with EventSource.addEventListener(type, handler).
   */
  it('parses named event', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('event: update\ndata: {"version":3}\n\n');

    expect(events).toHaveLength(1);
    expect(events[0].event).toBe('update');
    expect(events[0].data).toBe('{"version":3}');
  });

  /**
   * Verifies "id:" field tracking: the parser passes the id through to
   * the event, enabling Last-Event-ID reconnection support.
   */
  it('parses event with id', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('id: 42\nevent: msg\ndata: hello\n\n');

    expect(events[0].id).toBe('42');
    expect(events[0].event).toBe('msg');
  });

  /**
   * Verifies multi-line data handling: per WHATWG SSE spec, multiple
   * "data:" lines are joined with "\n" characters.
   */
  it('joins multi-line data with newlines', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('data: line1\ndata: line2\ndata: line3\n\n');

    expect(events[0].data).toBe('line1\nline2\nline3');
  });

  /**
   * Verifies that SSE comments (lines starting with ":") are passed to
   * the onComment callback but do NOT fire onEvent. This is how servers
   * send keepalive signals.
   */
  it('ignores comments, does not fire onEvent', () => {
    const events: EventSourceMessage[] = [];
    const comments: string[] = [];
    const parser = createParser({
      onEvent: (e) => events.push(e),
      onComment: (c) => comments.push(c),
    });

    parser.feed(': keepalive\n\n');
    parser.feed('data: real\n\n');

    expect(events).toHaveLength(1);
    expect(events[0].data).toBe('real');
    expect(comments).toEqual(['keepalive']);
  });

  /**
   * Verifies CRLF line ending handling: per WHATWG spec, SSE must support
   * CR+LF, LF, and CR as line terminators.
   */
  it('handles CRLF line endings', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('data: hello\r\n\r\n');

    expect(events).toHaveLength(1);
    expect(events[0].data).toBe('hello');
  });

  /**
   * Verifies UTF-8 BOM stripping: the parser removes the BOM from the
   * first chunk per WHATWG spec requirement.
   */
  it('strips UTF-8 BOM from first chunk', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('\xEF\xBB\xBFdata: hello\n\n');

    expect(events).toHaveLength(1);
    expect(events[0].data).toBe('hello');
  });

  /**
   * Verifies that data split across multiple feed() calls is correctly
   * buffered and parsed when the terminating blank line arrives.
   */
  it('handles chunked data across feed calls', () => {
    const events: EventSourceMessage[] = [];
    const parser = createParser({ onEvent: (e) => events.push(e) });

    parser.feed('data: hel');
    parser.feed('lo\n\n');

    expect(events).toHaveLength(1);
    expect(events[0].data).toBe('hello');
  });
});

// ============================================================================
// SSEClient Tests
// ============================================================================

describe('SSEClient', () => {
  /**
   * Verifies that SSEClient connects via mock fetch, receives data events,
   * and dispatches them to the onMessage handler with parsed JSON.
   */
  it('connects and receives messages', async () => {
    const { client, controller } = SSEClient.createMock();
    const received: unknown[] = [];
    client.onMessage = (data) => received.push(data);

    await client.connect('http://test/events');
    expect(client.isConnected).toBe(true);

    controller.simulateMessage({ greeting: 'hello' });
    controller.simulateClose();

    // Wait for async stream processing
    await new Promise((r) => setTimeout(r, 10));

    expect(received).toHaveLength(1);
    expect(received[0]).toEqual({ greeting: 'hello' });
  });

  /**
   * Verifies that named events (with "event:" field) are dispatched to
   * the onEvent handler with the event type and parsed data.
   */
  it('dispatches named events to onEvent', async () => {
    const { client, controller } = SSEClient.createMock();
    const events: Array<{ type: string; data: unknown }> = [];
    client.onEvent = (type, data) => events.push({ type, data });

    await client.connect('http://test/events');
    controller.simulateEvent('update', { version: 3 });
    controller.simulateClose();

    await new Promise((r) => setTimeout(r, 10));

    expect(events).toHaveLength(1);
    expect(events[0].type).toBe('update');
    expect(events[0].data).toEqual({ version: 3 });
  });

  /**
   * Verifies that keepalive comments (": keepalive") are silently consumed
   * and do NOT trigger onMessage or onEvent.
   */
  it('filters keepalive comments', async () => {
    const { client, controller } = SSEClient.createMock();
    const received: unknown[] = [];
    client.onMessage = (data) => received.push(data);

    await client.connect('http://test/events');
    controller.simulateComment('keepalive');
    controller.simulateMessage({ real: true });
    controller.simulateClose();

    await new Promise((r) => setTimeout(r, 10));

    expect(received).toHaveLength(1);
    expect(received[0]).toEqual({ real: true });
  });

  /**
   * Verifies that the "id:" field from events is tracked in lastEventId,
   * enabling reconnection via the Last-Event-ID header.
   */
  it('tracks lastEventId from events', async () => {
    const { client, controller } = SSEClient.createMock();

    await client.connect('http://test/events');
    expect(client.lastEventId).toBeUndefined();

    controller.simulateEventWithId('msg', 'evt-99', { data: 'test' });
    controller.simulateClose();

    await new Promise((r) => setTimeout(r, 10));

    expect(client.lastEventId).toBe('evt-99');
  });

  /**
   * Verifies that close() fires the onClose handler.
   */
  it('fires onClose when stream ends', async () => {
    const { client, controller } = SSEClient.createMock();
    let closed = false;
    client.onClose = () => { closed = true; };

    await client.connect('http://test/events');
    controller.simulateClose();

    await new Promise((r) => setTimeout(r, 10));

    expect(closed).toBe(true);
    expect(client.isConnected).toBe(false);
  });

  /**
   * Verifies that createMock captures the request URL and headers
   * for test assertions.
   */
  it('createMock captures request details', async () => {
    const { fetch, controller } = createMockSSEPair();
    const client = new SSEClient({
      fetch,
      headers: { Authorization: 'Bearer token123' },
    });

    await client.connect('http://test/events');
    controller.simulateClose();

    expect(controller.receivedUrl).toBe('http://test/events');
    expect(controller.receivedHeaders['Authorization']).toBe('Bearer token123');

    await new Promise((r) => setTimeout(r, 10));
  });
});

// ============================================================================
// StreamableClient Tests
// ============================================================================

describe('StreamableClient', () => {
  /**
   * Verifies that when the server responds with application/json,
   * StreamableClient parses the JSON body and returns it directly.
   * This is the synchronous (non-streaming) path.
   */
  it('returns parsed JSON for application/json response', async () => {
    const { fetch } = createMockSSEPair('application/json');
    const client = new StreamableClient<{ method: string }, { result: string }>({ fetch });

    // Override the mock to return JSON instead of a stream
    const jsonFetch = (async (_url: RequestInfo | URL, _init?: RequestInit) => {
      return new Response(JSON.stringify({ result: 'ok' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }) as typeof globalThis.fetch;

    const jsonClient = new StreamableClient<{ method: string }, { result: string }>({
      fetch: jsonFetch,
    });

    const result = await jsonClient.post('http://test/rpc', { method: 'getUser' });

    expect(result).toEqual({ result: 'ok' });
  });

  /**
   * Verifies that when the server responds with text/event-stream,
   * StreamableClient streams events via onMessage/onEvent callbacks
   * and calls onDone when the stream completes.
   */
  it('streams SSE events for text/event-stream response', async () => {
    const { fetch, controller } = createMockSSEPair('text/event-stream');
    const client = new StreamableClient<{ method: string }, { progress: number }>({ fetch });

    const events: unknown[] = [];
    let done = false;
    client.onMessage = (data) => events.push(data);
    client.onDone = () => { done = true; };

    const postPromise = client.post('http://test/rpc', { method: 'longOp' });

    // Give the fetch a moment to resolve
    await new Promise((r) => setTimeout(r, 10));

    controller.simulateMessage({ progress: 50 });
    controller.simulateMessage({ progress: 100 });
    controller.simulateClose();

    await postPromise;

    expect(events).toHaveLength(2);
    expect(events[0]).toEqual({ progress: 50 });
    expect(events[1]).toEqual({ progress: 100 });
    expect(done).toBe(true);
  });

  /**
   * Verifies that onDone fires when the SSE stream ends, even if
   * no events were received.
   */
  it('fires onDone when stream completes', async () => {
    const { fetch, controller } = createMockSSEPair('text/event-stream');
    const client = new StreamableClient({ fetch });

    let done = false;
    client.onDone = () => { done = true; };

    const postPromise = client.post('http://test/rpc', {});

    await new Promise((r) => setTimeout(r, 10));
    controller.simulateClose();

    await postPromise;

    expect(done).toBe(true);
  });

  /**
   * Verifies that StreamableClient sends the request body as JSON
   * and includes proper headers (Content-Type, Accept).
   */
  it('sends POST with correct headers and body', async () => {
    const { fetch, controller } = createMockSSEPair('text/event-stream');
    const client = new StreamableClient({ fetch });

    const postPromise = client.post('http://test/rpc', { method: 'test', id: 42 });

    await new Promise((r) => setTimeout(r, 10));
    controller.simulateClose();
    await postPromise;

    expect(controller.receivedMethod).toBe('POST');
    expect(controller.receivedBody).toBe('{"method":"test","id":42}');
    expect(controller.receivedHeaders['Content-Type']).toBe('application/json');
  });
});
