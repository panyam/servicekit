"use strict";
/**
 * Mock WebSocket utilities for testing.
 *
 * Provides a fake WebSocket implementation and controller that can be used
 * to test any client built on BaseWSClient without real connections.
 *
 * @example
 * ```typescript
 * import { BaseWSClient } from './base-client';
 * import { createMockWSPair } from './mock';
 *
 * const { WebSocket, controller } = createMockWSPair();
 * const client = new BaseWSClient({ WebSocket });
 * client.connect('ws://test');
 * controller.simulateOpen();
 * controller.simulateRawMessage('{"hello":"world"}');
 * ```
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.createMockWSPair = createMockWSPair;
const types_1 = require("./types");
/**
 * Create a mock WebSocket constructor + controller pair.
 *
 * The returned `WebSocket` can be passed as `ClientOptions.WebSocket`
 * to any client. The controller lets tests drive the fake connection.
 *
 * A new MockWebSocket instance is captured each time the client calls
 * `connect()`, so the controller always refers to the latest connection.
 */
function createMockWSPair() {
    let ws = null;
    class MockWebSocket {
        constructor(_url) {
            this.readyState = types_1.ReadyState.CONNECTING;
            this.binaryType = '';
            this.onopen = null;
            this.onmessage = null;
            this.onerror = null;
            this.onclose = null;
            this._sent = [];
            ws = this;
        }
        send(data) {
            this._sent.push(data);
        }
        close() {
            this.readyState = types_1.ReadyState.CLOSED;
            this.onclose?.({});
        }
        get sentRaw() {
            return this._sent;
        }
    }
    const assertConnected = (method) => {
        if (!ws)
            throw new Error(`Call client.connect() before ${method}()`);
    };
    const controller = {
        get sentRaw() {
            return ws?.sentRaw ?? [];
        },
        simulateOpen() {
            assertConnected('simulateOpen');
            ws.readyState = types_1.ReadyState.OPEN;
            ws.onopen?.({});
        },
        simulateRawMessage(data) {
            assertConnected('simulateRawMessage');
            ws.onmessage?.({ data });
        },
        simulateWsError() {
            assertConnected('simulateWsError');
            ws.onerror?.({});
        },
        simulateClose(code) {
            assertConnected('simulateClose');
            ws.readyState = types_1.ReadyState.CLOSED;
            ws.onclose?.({});
        },
        get readyState() {
            return ws?.readyState ?? types_1.ReadyState.CLOSED;
        },
    };
    return {
        WebSocket: MockWebSocket,
        controller,
    };
}
//# sourceMappingURL=mock.js.map