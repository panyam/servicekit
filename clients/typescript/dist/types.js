"use strict";
/**
 * Protocol types for the gRPC-WebSocket streaming protocol.
 *
 * These types define the JSON envelope format used for communication
 * between the client and server over WebSocket.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.BinaryCodec = exports.JSONCodec = exports.ReadyState = void 0;
/**
 * WebSocket ready state constants.
 * Mirrors the WebSocket.readyState values.
 */
exports.ReadyState = {
    CONNECTING: 0,
    OPEN: 1,
    CLOSING: 2,
    CLOSED: 3,
};
/**
 * JSON codec - encodes/decodes messages as JSON.
 * This is the default codec, matching server-side JSONCodec.
 */
class JSONCodec {
    decode(data) {
        if (typeof data === 'string') {
            return JSON.parse(data);
        }
        // ArrayBuffer -> string -> JSON
        const text = new TextDecoder().decode(data);
        return JSON.parse(text);
    }
    encode(msg) {
        return JSON.stringify(msg);
    }
}
exports.JSONCodec = JSONCodec;
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
class BinaryCodec {
    constructor(decodeFunc, encodeFunc) {
        this.decodeFunc = decodeFunc;
        this.encodeFunc = encodeFunc;
    }
    decode(data) {
        if (typeof data === 'string') {
            throw new Error('BinaryCodec received text data, expected binary');
        }
        return this.decodeFunc(data);
    }
    encode(msg) {
        const encoded = this.encodeFunc(msg);
        // Ensure we return a proper ArrayBuffer (not SharedArrayBuffer)
        return encoded.buffer.slice(encoded.byteOffset, encoded.byteOffset + encoded.byteLength);
    }
}
exports.BinaryCodec = BinaryCodec;
//# sourceMappingURL=types.js.map