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
 * Options for configuring the BaseGRPCWSClient.
 */
export interface ClientOptions {
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
}

/**
 * Event handler types for the client.
 */
export type MessageHandler<T = unknown> = (data: T) => void;
export type ErrorHandler = (error: string) => void;
export type VoidHandler = () => void;
