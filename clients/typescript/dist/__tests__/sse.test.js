"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const vitest_1 = require("vitest");
const sse_parser_1 = require("../sse-parser");
const sse_client_1 = require("../sse-client");
const streamable_client_1 = require("../streamable-client");
const sse_mock_1 = require("../sse-mock");
// ============================================================================
// SSE Parser Tests
// ============================================================================
(0, vitest_1.describe)('SSEParser', () => {
    /**
     * Verifies basic SSE data event parsing: a single "data:" line followed
     * by a blank line should fire onEvent with the data content.
     */
    (0, vitest_1.it)('parses basic data event', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('data: {"msg":"hello"}\n\n');
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].data).toBe('{"msg":"hello"}');
        (0, vitest_1.expect)(events[0].event).toBeUndefined();
    });
    /**
     * Verifies named event parsing: "event:" field sets the event type,
     * which clients use with EventSource.addEventListener(type, handler).
     */
    (0, vitest_1.it)('parses named event', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('event: update\ndata: {"version":3}\n\n');
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].event).toBe('update');
        (0, vitest_1.expect)(events[0].data).toBe('{"version":3}');
    });
    /**
     * Verifies "id:" field tracking: the parser passes the id through to
     * the event, enabling Last-Event-ID reconnection support.
     */
    (0, vitest_1.it)('parses event with id', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('id: 42\nevent: msg\ndata: hello\n\n');
        (0, vitest_1.expect)(events[0].id).toBe('42');
        (0, vitest_1.expect)(events[0].event).toBe('msg');
    });
    /**
     * Verifies multi-line data handling: per WHATWG SSE spec, multiple
     * "data:" lines are joined with "\n" characters.
     */
    (0, vitest_1.it)('joins multi-line data with newlines', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('data: line1\ndata: line2\ndata: line3\n\n');
        (0, vitest_1.expect)(events[0].data).toBe('line1\nline2\nline3');
    });
    /**
     * Verifies that SSE comments (lines starting with ":") are passed to
     * the onComment callback but do NOT fire onEvent. This is how servers
     * send keepalive signals.
     */
    (0, vitest_1.it)('ignores comments, does not fire onEvent', () => {
        const events = [];
        const comments = [];
        const parser = (0, sse_parser_1.createParser)({
            onEvent: (e) => events.push(e),
            onComment: (c) => comments.push(c),
        });
        parser.feed(': keepalive\n\n');
        parser.feed('data: real\n\n');
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].data).toBe('real');
        (0, vitest_1.expect)(comments).toEqual(['keepalive']);
    });
    /**
     * Verifies CRLF line ending handling: per WHATWG spec, SSE must support
     * CR+LF, LF, and CR as line terminators.
     */
    (0, vitest_1.it)('handles CRLF line endings', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('data: hello\r\n\r\n');
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].data).toBe('hello');
    });
    /**
     * Verifies UTF-8 BOM stripping: the parser removes the BOM from the
     * first chunk per WHATWG spec requirement.
     */
    (0, vitest_1.it)('strips UTF-8 BOM from first chunk', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('\xEF\xBB\xBFdata: hello\n\n');
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].data).toBe('hello');
    });
    /**
     * Verifies that data split across multiple feed() calls is correctly
     * buffered and parsed when the terminating blank line arrives.
     */
    (0, vitest_1.it)('handles chunked data across feed calls', () => {
        const events = [];
        const parser = (0, sse_parser_1.createParser)({ onEvent: (e) => events.push(e) });
        parser.feed('data: hel');
        parser.feed('lo\n\n');
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].data).toBe('hello');
    });
});
// ============================================================================
// SSEClient Tests
// ============================================================================
(0, vitest_1.describe)('SSEClient', () => {
    /**
     * Verifies that SSEClient connects via mock fetch, receives data events,
     * and dispatches them to the onMessage handler with parsed JSON.
     */
    (0, vitest_1.it)('connects and receives messages', async () => {
        const { client, controller } = sse_client_1.SSEClient.createMock();
        const received = [];
        client.onMessage = (data) => received.push(data);
        await client.connect('http://test/events');
        (0, vitest_1.expect)(client.isConnected).toBe(true);
        controller.simulateMessage({ greeting: 'hello' });
        controller.simulateClose();
        // Wait for async stream processing
        await new Promise((r) => setTimeout(r, 10));
        (0, vitest_1.expect)(received).toHaveLength(1);
        (0, vitest_1.expect)(received[0]).toEqual({ greeting: 'hello' });
    });
    /**
     * Verifies that named events (with "event:" field) are dispatched to
     * the onEvent handler with the event type and parsed data.
     */
    (0, vitest_1.it)('dispatches named events to onEvent', async () => {
        const { client, controller } = sse_client_1.SSEClient.createMock();
        const events = [];
        client.onEvent = (type, data) => events.push({ type, data });
        await client.connect('http://test/events');
        controller.simulateEvent('update', { version: 3 });
        controller.simulateClose();
        await new Promise((r) => setTimeout(r, 10));
        (0, vitest_1.expect)(events).toHaveLength(1);
        (0, vitest_1.expect)(events[0].type).toBe('update');
        (0, vitest_1.expect)(events[0].data).toEqual({ version: 3 });
    });
    /**
     * Verifies that keepalive comments (": keepalive") are silently consumed
     * and do NOT trigger onMessage or onEvent.
     */
    (0, vitest_1.it)('filters keepalive comments', async () => {
        const { client, controller } = sse_client_1.SSEClient.createMock();
        const received = [];
        client.onMessage = (data) => received.push(data);
        await client.connect('http://test/events');
        controller.simulateComment('keepalive');
        controller.simulateMessage({ real: true });
        controller.simulateClose();
        await new Promise((r) => setTimeout(r, 10));
        (0, vitest_1.expect)(received).toHaveLength(1);
        (0, vitest_1.expect)(received[0]).toEqual({ real: true });
    });
    /**
     * Verifies that the "id:" field from events is tracked in lastEventId,
     * enabling reconnection via the Last-Event-ID header.
     */
    (0, vitest_1.it)('tracks lastEventId from events', async () => {
        const { client, controller } = sse_client_1.SSEClient.createMock();
        await client.connect('http://test/events');
        (0, vitest_1.expect)(client.lastEventId).toBeUndefined();
        controller.simulateEventWithId('msg', 'evt-99', { data: 'test' });
        controller.simulateClose();
        await new Promise((r) => setTimeout(r, 10));
        (0, vitest_1.expect)(client.lastEventId).toBe('evt-99');
    });
    /**
     * Verifies that close() fires the onClose handler.
     */
    (0, vitest_1.it)('fires onClose when stream ends', async () => {
        const { client, controller } = sse_client_1.SSEClient.createMock();
        let closed = false;
        client.onClose = () => { closed = true; };
        await client.connect('http://test/events');
        controller.simulateClose();
        await new Promise((r) => setTimeout(r, 10));
        (0, vitest_1.expect)(closed).toBe(true);
        (0, vitest_1.expect)(client.isConnected).toBe(false);
    });
    /**
     * Verifies that createMock captures the request URL and headers
     * for test assertions.
     */
    (0, vitest_1.it)('createMock captures request details', async () => {
        const { fetch, controller } = (0, sse_mock_1.createMockSSEPair)();
        const client = new sse_client_1.SSEClient({
            fetch,
            headers: { Authorization: 'Bearer token123' },
        });
        await client.connect('http://test/events');
        controller.simulateClose();
        (0, vitest_1.expect)(controller.receivedUrl).toBe('http://test/events');
        (0, vitest_1.expect)(controller.receivedHeaders['Authorization']).toBe('Bearer token123');
        await new Promise((r) => setTimeout(r, 10));
    });
});
// ============================================================================
// StreamableClient Tests
// ============================================================================
(0, vitest_1.describe)('StreamableClient', () => {
    /**
     * Verifies that when the server responds with application/json,
     * StreamableClient parses the JSON body and returns it directly.
     * This is the synchronous (non-streaming) path.
     */
    (0, vitest_1.it)('returns parsed JSON for application/json response', async () => {
        const { fetch } = (0, sse_mock_1.createMockSSEPair)('application/json');
        const client = new streamable_client_1.StreamableClient({ fetch });
        // Override the mock to return JSON instead of a stream
        const jsonFetch = (async (_url, _init) => {
            return new Response(JSON.stringify({ result: 'ok' }), {
                status: 200,
                headers: { 'Content-Type': 'application/json' },
            });
        });
        const jsonClient = new streamable_client_1.StreamableClient({
            fetch: jsonFetch,
        });
        const result = await jsonClient.post('http://test/rpc', { method: 'getUser' });
        (0, vitest_1.expect)(result).toEqual({ result: 'ok' });
    });
    /**
     * Verifies that when the server responds with text/event-stream,
     * StreamableClient streams events via onMessage/onEvent callbacks
     * and calls onDone when the stream completes.
     */
    (0, vitest_1.it)('streams SSE events for text/event-stream response', async () => {
        const { fetch, controller } = (0, sse_mock_1.createMockSSEPair)('text/event-stream');
        const client = new streamable_client_1.StreamableClient({ fetch });
        const events = [];
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
        (0, vitest_1.expect)(events).toHaveLength(2);
        (0, vitest_1.expect)(events[0]).toEqual({ progress: 50 });
        (0, vitest_1.expect)(events[1]).toEqual({ progress: 100 });
        (0, vitest_1.expect)(done).toBe(true);
    });
    /**
     * Verifies that onDone fires when the SSE stream ends, even if
     * no events were received.
     */
    (0, vitest_1.it)('fires onDone when stream completes', async () => {
        const { fetch, controller } = (0, sse_mock_1.createMockSSEPair)('text/event-stream');
        const client = new streamable_client_1.StreamableClient({ fetch });
        let done = false;
        client.onDone = () => { done = true; };
        const postPromise = client.post('http://test/rpc', {});
        await new Promise((r) => setTimeout(r, 10));
        controller.simulateClose();
        await postPromise;
        (0, vitest_1.expect)(done).toBe(true);
    });
    /**
     * Verifies that StreamableClient sends the request body as JSON
     * and includes proper headers (Content-Type, Accept).
     */
    (0, vitest_1.it)('sends POST with correct headers and body', async () => {
        const { fetch, controller } = (0, sse_mock_1.createMockSSEPair)('text/event-stream');
        const client = new streamable_client_1.StreamableClient({ fetch });
        const postPromise = client.post('http://test/rpc', { method: 'test', id: 42 });
        await new Promise((r) => setTimeout(r, 10));
        controller.simulateClose();
        await postPromise;
        (0, vitest_1.expect)(controller.receivedMethod).toBe('POST');
        (0, vitest_1.expect)(controller.receivedBody).toBe('{"method":"test","id":42}');
        (0, vitest_1.expect)(controller.receivedHeaders['Content-Type']).toBe('application/json');
    });
});
//# sourceMappingURL=sse.test.js.map