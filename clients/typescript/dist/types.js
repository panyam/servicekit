"use strict";
/**
 * Protocol types for the gRPC-WebSocket streaming protocol.
 *
 * These types define the JSON envelope format used for communication
 * between the client and server over WebSocket.
 */
Object.defineProperty(exports, "__esModule", { value: true });
exports.ReadyState = void 0;
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
//# sourceMappingURL=types.js.map