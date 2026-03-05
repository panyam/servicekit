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

import { ReadyState } from './types';

/**
 * Low-level mock WebSocket controller.
 *
 * Operates at the WebSocket transport level — no protocol awareness.
 * Use this directly with BaseWSClient, or wrap with protocol-specific
 * logic for GRPCWSClient.
 */
export interface MockWSController {
  /** Raw messages sent via ws.send() (strings or ArrayBuffers) */
  readonly sentRaw: readonly (string | ArrayBuffer)[];

  /** Simulate WebSocket open — resolves the connect() Promise */
  simulateOpen(): void;

  /** Simulate receiving a raw WebSocket message (no wrapping) */
  simulateRawMessage(data: string | ArrayBuffer): void;

  /** Simulate a WebSocket error event */
  simulateWsError(): void;

  /** Simulate WebSocket close */
  simulateClose(code?: number): void;

  /** Current WebSocket ready state */
  readonly readyState: number;
}

/**
 * Create a mock WebSocket constructor + controller pair.
 *
 * The returned `WebSocket` can be passed as `ClientOptions.WebSocket`
 * to any client. The controller lets tests drive the fake connection.
 *
 * A new MockWebSocket instance is captured each time the client calls
 * `connect()`, so the controller always refers to the latest connection.
 */
export function createMockWSPair(): {
  WebSocket: typeof globalThis.WebSocket;
  controller: MockWSController;
} {
  let ws: MockWebSocket | null = null;

  class MockWebSocket {
    readyState: number = ReadyState.CONNECTING;
    binaryType = '';
    onopen: ((ev: unknown) => void) | null = null;
    onmessage: ((ev: { data: string | ArrayBuffer }) => void) | null = null;
    onerror: ((ev: unknown) => void) | null = null;
    onclose: ((ev: unknown) => void) | null = null;

    private _sent: (string | ArrayBuffer)[] = [];

    constructor(_url: string) {
      ws = this;
    }

    send(data: string | ArrayBuffer) {
      this._sent.push(data);
    }

    close() {
      this.readyState = ReadyState.CLOSED;
      this.onclose?.({});
    }

    get sentRaw(): readonly (string | ArrayBuffer)[] {
      return this._sent;
    }
  }

  const assertConnected = (method: string) => {
    if (!ws) throw new Error(`Call client.connect() before ${method}()`);
  };

  const controller: MockWSController = {
    get sentRaw() {
      return ws?.sentRaw ?? [];
    },

    simulateOpen() {
      assertConnected('simulateOpen');
      ws!.readyState = ReadyState.OPEN;
      ws!.onopen?.({});
    },

    simulateRawMessage(data: string | ArrayBuffer) {
      assertConnected('simulateRawMessage');
      ws!.onmessage?.({ data });
    },

    simulateWsError() {
      assertConnected('simulateWsError');
      ws!.onerror?.({});
    },

    simulateClose(code?: number) {
      assertConnected('simulateClose');
      ws!.readyState = ReadyState.CLOSED;
      ws!.onclose?.({});
    },

    get readyState() {
      return ws?.readyState ?? ReadyState.CLOSED;
    },
  };

  return {
    WebSocket: MockWebSocket as unknown as typeof globalThis.WebSocket,
    controller,
  };
}
