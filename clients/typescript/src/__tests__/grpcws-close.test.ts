import { describe, it, expect } from 'vitest';
import { BaseWSClient } from '../base-client';
import { GRPCWSClient, StreamCloseInfo } from '../grpcws-client';
import { TypedGRPCWSClient } from '../typed-client';
import { createMockWSPair } from '../mock';
import { CloseInfo } from '../types';

describe('BaseWSClient close', () => {
  it('passes the close code and reason to onClose', () => {
    const { WebSocket, controller } = createMockWSPair();
    const client = new BaseWSClient({ WebSocket });
    const closes: CloseInfo[] = [];
    client.onClose = (info) => closes.push(info);

    client.connect('ws://test');
    controller.simulateOpen();
    controller.simulateClose(1001, 'going away');

    expect(closes).toEqual([{ code: 1001, reason: 'going away', wasClean: true }]);
  });
});

describe('GRPCWSClient close', () => {
  it('reports a server close after stream_end as a normal end', () => {
    const { client, controller } = GRPCWSClient.createMock();
    const events: string[] = [];
    const closes: StreamCloseInfo[] = [];
    client.onStreamEnd = () => events.push('stream_end');
    client.onError = (e) => events.push(`error:${e}`);
    client.onClose = (info) => {
      events.push('close');
      closes.push(info);
    };

    client.connect('ws://test');
    controller.simulateOpen();
    controller.simulateStreamEnd();
    controller.simulateClose(1000, 'stream_end');

    expect(events).toEqual(['stream_end', 'close']);
    expect(closes).toEqual([
      { code: 1000, reason: 'stream_end', wasClean: true, streamEnded: true },
    ]);
  });

  it('reports a close without stream_end as not ended', () => {
    const { client, controller } = GRPCWSClient.createMock();
    const closes: StreamCloseInfo[] = [];
    client.onClose = (info) => closes.push(info);

    client.connect('ws://test');
    controller.simulateOpen();
    controller.simulateClose(1006);

    expect(closes).toEqual([{ code: 1006, reason: '', wasClean: false, streamEnded: false }]);
  });

  it('resets streamEnded on reconnect', () => {
    const { client, controller } = GRPCWSClient.createMock();
    const closes: StreamCloseInfo[] = [];
    client.onClose = (info) => closes.push(info);

    client.connect('ws://test');
    controller.simulateOpen();
    controller.simulateStreamEnd();
    controller.simulateClose(1000, 'stream_end');

    client.connect('ws://test');
    controller.simulateOpen();
    controller.simulateClose(1006);

    expect(closes.map((c) => c.streamEnded)).toEqual([true, false]);
  });
});

describe('TypedGRPCWSClient close', () => {
  it('forwards close info', () => {
    const { WebSocket, controller } = createMockWSPair();
    const client = new TypedGRPCWSClient<unknown, unknown>({ WebSocket });
    const closes: StreamCloseInfo[] = [];
    client.onClose = (info) => closes.push(info);

    client.connect('ws://test');
    controller.simulateOpen();
    controller.simulateRawMessage(JSON.stringify({ type: 'stream_end' }));
    controller.simulateClose(1000, 'stream_end');

    expect(closes).toEqual([
      { code: 1000, reason: 'stream_end', wasClean: true, streamEnded: true },
    ]);
  });
});
