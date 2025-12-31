/**
 * @panyam/servicekit-client
 *
 * TypeScript client library for ServiceKit WebSocket protocols.
 *
 * ## Architecture
 *
 * The client mirrors the server-side architecture with two layers:
 * - **Transport layer**: Handles WebSocket connection, ping/pong heartbeats
 * - **Codec layer**: Handles encoding/decoding of data messages
 *
 * Control messages (ping, pong, error) are always JSON at the transport layer,
 * while data messages use the configured codec (JSON, binary protobuf, etc.).
 *
 * ## Clients
 *
 * - `BaseWSClient`: Low-level WebSocket with auto ping/pong (for http/JSONConn)
 * - `GRPCWSClient`: gRPC-style streaming with envelope protocol (for grpcws)
 * - `TypedGRPCWSClient`: Type-safe wrapper for GRPCWSClient
 *
 * ## Codecs
 *
 * - `JSONCodec`: Default, matches server-side JSONCodec
 * - `BinaryCodec`: For binary protobuf, matches server-side BinaryProtoCodec
 *
 * @example Basic WebSocket with JSON (default)
 * ```typescript
 * import { BaseWSClient } from '@panyam/servicekit-client';
 *
 * const client = new BaseWSClient();
 * client.onMessage = (data) => console.log('Received:', data);
 * await client.connect('ws://localhost:8080/ws');
 * client.send({ hello: 'world' });
 * ```
 *
 * @example Binary Protobuf Protocol
 * ```typescript
 * import { BaseWSClient, BinaryCodec } from '@panyam/servicekit-client';
 * import { MyMessage } from './gen/my_pb';
 *
 * const codec = new BinaryCodec<MyMessage, MyMessage>(
 *   (data) => MyMessage.decode(new Uint8Array(data)),
 *   (msg) => MyMessage.encode(msg).finish()
 * );
 * const client = new BaseWSClient({ codec });
 * client.onMessage = (msg) => console.log('Received:', msg);
 * await client.connect('ws://localhost:8080/ws');
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
 * @packageDocumentation
 */

// Types and Codecs
export {
  MessageType,
  ControlMessage,
  ReadyState,
  ReadyStateType,
  ClientOptions,
  Codec,
  JSONCodec,
  BinaryCodec,
  MessageHandler,
  ErrorHandler,
  VoidHandler,
} from './types';

// Clients
export { BaseWSClient } from './base-client';
export { GRPCWSClient } from './grpcws-client';
export { TypedGRPCWSClient } from './typed-client';
