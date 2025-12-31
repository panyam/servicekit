/**
 * Protocol types for the gRPC-WebSocket streaming protocol.
 *
 * These types define the JSON envelope format used for communication
 * between the client and server over WebSocket.
 */

/**
 * Message types used in the control envelope.
 *
 * - `data`: Contains a protobuf message as JSON payload
 * - `error`: Server-side error notification
 * - `stream_end`: Indicates the stream has completed
 * - `ping`: Server heartbeat request
 * - `pong`: Client heartbeat response
 * - `cancel`: Client requests stream cancellation
 * - `end_send`: Client signals done sending (half-close)
 */
export type MessageType =
  | 'data'
  | 'error'
  | 'stream_end'
  | 'ping'
  | 'pong'
  | 'cancel'
  | 'end_send';

/**
 * Control message envelope used for all WebSocket communication.
 *
 * All messages are JSON objects with a `type` field indicating the message kind.
 * Additional fields depend on the message type:
 * - `data` messages include a `data` field with the payload
 * - `error` messages include an `error` field with the error description
 * - `ping`/`pong` messages include a `pingId` field for correlation
 */
export interface ControlMessage {
  /** The type of control message */
  type: MessageType;
  /** Payload data for 'data' messages (protobuf as JSON) */
  data?: unknown;
  /** Error description for 'error' messages */
  error?: string;
  /** Ping/pong identifier for heartbeat correlation */
  pingId?: number;
}

/**
 * WebSocket ready state constants.
 * Mirrors the WebSocket.readyState values.
 */
export const ReadyState = {
  CONNECTING: 0,
  OPEN: 1,
  CLOSING: 2,
  CLOSED: 3,
} as const;

export type ReadyStateType = (typeof ReadyState)[keyof typeof ReadyState];

/**
 * Options for configuring the BaseWSClient.
 */
export interface ClientOptions<I = unknown, O = unknown> {
  /**
   * Whether to automatically respond to ping messages.
   * Default: true
   */
  autoPong?: boolean;

  /**
   * Custom WebSocket implementation (for Node.js or testing).
   * Default: globalThis.WebSocket
   */
  WebSocket?: typeof WebSocket;

  /**
   * Codec for encoding/decoding data messages.
   * Default: JSONCodec (matches server-side JSONCodec)
   */
  codec?: Codec<I, O>;
}

/**
 * Event handler types for the client.
 */
export type MessageHandler<T = unknown> = (data: T) => void;
export type ErrorHandler = (error: string) => void;
export type VoidHandler = () => void;

/**
 * Codec interface for encoding/decoding data messages.
 * This mirrors the server-side Codec interface.
 *
 * Note: Control messages (ping/pong/error) are handled at the transport layer
 * and always use JSON. The codec only handles business data messages.
 */
export interface Codec<I = unknown, O = unknown> {
  /**
   * Decode incoming data from the server.
   * @param data Raw data from WebSocket (string for text frames, ArrayBuffer for binary)
   * @returns Decoded message of type I
   */
  decode(data: string | ArrayBuffer): I;

  /**
   * Encode outgoing data to send to the server.
   * @param msg Message to encode
   * @returns Encoded data (string for text, ArrayBuffer for binary)
   */
  encode(msg: O): string | ArrayBuffer;
}

/**
 * JSON codec - encodes/decodes messages as JSON.
 * This is the default codec, matching server-side JSONCodec.
 */
export class JSONCodec<I = unknown, O = unknown> implements Codec<I, O> {
  decode(data: string | ArrayBuffer): I {
    if (typeof data === 'string') {
      return JSON.parse(data) as I;
    }
    // ArrayBuffer -> string -> JSON
    const text = new TextDecoder().decode(data);
    return JSON.parse(text) as I;
  }

  encode(msg: O): string {
    return JSON.stringify(msg);
  }
}

/**
 * Binary protobuf codec - for use with server-side BinaryProtoCodec.
 *
 * Users provide their own encode/decode functions from their protobuf library.
 * Works with any TS protoc plugin (@bufbuild/protobuf, ts-proto, protobuf-ts).
 *
 * @example
 * ```typescript
 * import { MyMessage } from './gen/my_pb';
 *
 * const codec = new BinaryCodec<MyMessage, MyMessage>(
 *   (data) => MyMessage.decode(new Uint8Array(data)),
 *   (msg) => MyMessage.encode(msg).finish()
 * );
 * ```
 */
export class BinaryCodec<I, O> implements Codec<I, O> {
  constructor(
    private decodeFunc: (data: ArrayBuffer) => I,
    private encodeFunc: (msg: O) => Uint8Array
  ) {}

  decode(data: string | ArrayBuffer): I {
    if (typeof data === 'string') {
      throw new Error('BinaryCodec received text data, expected binary');
    }
    return this.decodeFunc(data);
  }

  encode(msg: O): ArrayBuffer {
    const encoded = this.encodeFunc(msg);
    // Ensure we return a proper ArrayBuffer (not SharedArrayBuffer)
    return encoded.buffer.slice(encoded.byteOffset, encoded.byteOffset + encoded.byteLength) as ArrayBuffer;
  }
}
