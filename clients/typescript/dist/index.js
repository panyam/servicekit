"use strict";
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
Object.defineProperty(exports, "__esModule", { value: true });
exports.TypedGRPCWSClient = exports.GRPCWSClient = exports.BaseWSClient = exports.ReadyState = void 0;
// Types
var types_1 = require("./types");
Object.defineProperty(exports, "ReadyState", { enumerable: true, get: function () { return types_1.ReadyState; } });
// Clients
var base_client_1 = require("./base-client");
Object.defineProperty(exports, "BaseWSClient", { enumerable: true, get: function () { return base_client_1.BaseWSClient; } });
var grpcws_client_1 = require("./grpcws-client");
Object.defineProperty(exports, "GRPCWSClient", { enumerable: true, get: function () { return grpcws_client_1.GRPCWSClient; } });
var typed_client_1 = require("./typed-client");
Object.defineProperty(exports, "TypedGRPCWSClient", { enumerable: true, get: function () { return typed_client_1.TypedGRPCWSClient; } });
//# sourceMappingURL=index.js.map