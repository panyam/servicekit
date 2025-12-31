/**
 * @panyam/servicekit-client
 *
 * TypeScript client library for ServiceKit WebSocket protocols.
 *
 * This package provides clients for:
 * - BaseWSClient: Low-level WebSocket with auto ping/pong (for http/JSONConn)
 * - GRPCWSClient: gRPC-style streaming with envelope protocol (for grpcws)
 * - TypedGRPCWSClient: Type-safe wrapper for GRPCWSClient
 *
 * @example Basic WebSocket (http/JSONConn)
 * ```typescript
 * import { BaseWSClient } from '@panyam/servicekit-client';
 *
 * const client = new BaseWSClient();
 * client.onMessage = (data) => console.log('Received:', data);
 * await client.connect('ws://localhost:8080/ws');
 * client.send({ hello: 'world' });
 * ```
 *
 * @example gRPC-WebSocket Streaming
 * ```typescript
 * import { GRPCWSClient } from '@panyam/servicekit-client';
 *
 * const client = new GRPCWSClient();
 * client.onMessage = (data) => console.log('Event:', data);
 * client.onStreamEnd = () => console.log('Done');
 * await client.connect('ws://localhost:8080/ws/v1/subscribe');
 * ```
 *
 * @example Type-Safe with Protobuf Types
 * ```typescript
 * import { TypedGRPCWSClient } from '@panyam/servicekit-client';
 * import { PlayerAction, GameState } from './gen/game_pb';
 *
 * const client = new TypedGRPCWSClient<PlayerAction, GameState>();
 * client.onMessage = (state) => updateUI(state);
 * await client.connect('ws://localhost:8080/ws/v1/sync');
 * client.send({ actionId: '1', move: { x: 10, y: 20 } });
 * ```
 *
 * @packageDocumentation
 */
export { MessageType, ControlMessage, ReadyState, ReadyStateType, ClientOptions, MessageHandler, ErrorHandler, VoidHandler, } from './types';
export { BaseWSClient } from './base-client';
export { GRPCWSClient } from './grpcws-client';
export { TypedGRPCWSClient } from './typed-client';
//# sourceMappingURL=index.d.ts.map