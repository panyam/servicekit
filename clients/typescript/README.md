# @panyam/servicekit-client

TypeScript client library for [ServiceKit](https://github.com/panyam/servicekit) WebSocket protocols.

## Installation

```bash
npm install @panyam/servicekit-client
```

## Overview

This package provides three client classes in a layered architecture:

| Client | Use Case | Protocol |
|--------|----------|----------|
| `BaseWSClient` | Low-level WebSocket with auto ping/pong | Raw JSON messages |
| `GRPCWSClient` | gRPC-style streaming | Envelope: `{type: "data", data: ...}` |
| `TypedGRPCWSClient<TIn, TOut>` | Type-safe wrapper for GRPCWSClient | Same as GRPCWSClient |

## Quick Start

### Basic WebSocket (http/JSONConn)

Use `BaseWSClient` for plain WebSocket connections with automatic ping/pong handling:

```typescript
import { BaseWSClient } from '@panyam/servicekit-client';

const client = new BaseWSClient();

client.onMessage = (data) => {
  console.log('Received:', data);
};

client.onClose = () => {
  console.log('Disconnected');
};

await client.connect('ws://localhost:8080/ws');
client.send({ hello: 'world' });
```

### gRPC-WebSocket Streaming (grpcws)

Use `GRPCWSClient` for gRPC-style streaming with the envelope protocol:

```typescript
import { GRPCWSClient } from '@panyam/servicekit-client';

const client = new GRPCWSClient();

client.onMessage = (data) => {
  console.log('Event:', data);
};

client.onStreamEnd = () => {
  console.log('Stream completed');
};

client.onError = (error) => {
  console.error('Error:', error);
};

await client.connect('ws://localhost:8080/ws/v1/subscribe?game_id=123');
```

### Type-Safe with Protobuf Types

Use `TypedGRPCWSClient` with your protobuf-generated TypeScript types:

```typescript
import { TypedGRPCWSClient } from '@panyam/servicekit-client';
import { PlayerAction, GameState } from './gen/game_pb';

const client = new TypedGRPCWSClient<PlayerAction, GameState>();

client.onMessage = (state: GameState) => {
  console.log('Players:', state.players);
};

await client.connect('ws://localhost:8080/ws/v1/sync');
client.send({ actionId: '1', move: { x: 10, y: 20 } });
```

## Streaming Patterns

### Server Streaming

Server sends multiple messages; client just listens:

```typescript
const client = new GRPCWSClient();
client.onMessage = (event) => console.log('Event:', event);
client.onStreamEnd = () => console.log('Done');

await client.connect('ws://localhost:8080/ws/v1/subscribe');
// Just listen - server pushes events
```

### Client Streaming

Client sends multiple messages; server responds once at the end:

```typescript
const client = new GRPCWSClient();
client.onMessage = (summary) => console.log('Result:', summary);

await client.connect('ws://localhost:8080/ws/v1/commands');

// Send multiple commands
client.send({ commandId: '1', commandType: 'move' });
client.send({ commandId: '2', commandType: 'attack' });

// Signal done sending - server will respond
client.endSend();
```

### Bidirectional Streaming

Both sides send messages concurrently:

```typescript
const client = new GRPCWSClient();
client.onMessage = (state) => updateUI(state);

await client.connect('ws://localhost:8080/ws/v1/sync');

// Send actions whenever
client.send({ actionId: '1', move: { x: 10, y: 20 } });

// Receive responses concurrently
// When done sending:
client.endSend();
```

## API Reference

### BaseWSClient

Low-level WebSocket client with automatic ping/pong.

```typescript
class BaseWSClient {
  // Connection
  connect(url: string): Promise<void>
  close(): void

  // Sending
  send(data: unknown): void      // JSON-encoded
  sendRaw(message: string): void // Raw string

  // Events
  onMessage: (data: unknown) => void
  onPing: (pingId: number) => void
  onClose: () => void
  onError: (error: string) => void

  // State
  readonly isConnected: boolean
  readonly readyState: number
}
```

### GRPCWSClient

gRPC-WebSocket client with envelope protocol.

```typescript
class GRPCWSClient {
  // Connection
  connect(url: string): Promise<void>
  close(): void

  // Sending (wrapped in {type: "data", data: ...})
  send(data: unknown): void
  endSend(): void  // Half-close
  cancel(): void   // Cancel stream

  // Events
  onMessage: (data: unknown) => void
  onStreamEnd: () => void
  onError: (error: string) => void
  onClose: () => void
  onPing: (pingId: number) => void

  // State
  readonly isConnected: boolean
  readonly readyState: number
}
```

### TypedGRPCWSClient<TIn, TOut>

Type-safe wrapper for GRPCWSClient.

```typescript
class TypedGRPCWSClient<TIn, TOut> {
  // Same API as GRPCWSClient, but with typed send/onMessage
  send(data: TIn): void
  onMessage: (data: TOut) => void
  // ... other methods same as GRPCWSClient
}
```

## Protocol Details

### Ping/Pong Heartbeat

Both clients automatically respond to server pings:
- Server sends: `{type: "ping", pingId: N}`
- Client responds: `{type: "pong", pingId: N}`

### gRPC-WS Envelope Format

GRPCWSClient uses the following message envelope:

**Client → Server:**
```json
{"type": "data", "data": <payload>}
{"type": "end_send"}
{"type": "cancel"}
```

**Server → Client:**
```json
{"type": "data", "data": <payload>}
{"type": "stream_end"}
{"type": "error", "error": "message"}
```

## Configuration

### Custom WebSocket Implementation

For Node.js or testing, provide a custom WebSocket:

```typescript
import WebSocket from 'ws';

const client = new GRPCWSClient({
  WebSocket: WebSocket as any,
});
```

### Disable Auto Pong

```typescript
const client = new BaseWSClient({
  autoPong: false,
});

client.onPing = (pingId) => {
  // Handle ping manually
  client.send({ type: 'pong', pingId });
};
```

## Protobuf Type Generation

This client works with any TypeScript protoc plugin. Popular options:

- **[@bufbuild/protobuf](https://github.com/bufbuild/protobuf-es)** - Modern, tree-shakeable
- **[protobuf-ts](https://github.com/timostamm/protobuf-ts)** - Feature-rich
- **[ts-proto](https://github.com/stephenh/ts-proto)** - Plain TypeScript interfaces

Example with buf:

```bash
buf generate
```

Then use the generated types:

```typescript
import { TypedGRPCWSClient } from '@panyam/servicekit-client';
import { PlayerAction, GameState } from './gen/game_pb';

const client = new TypedGRPCWSClient<PlayerAction, GameState>();
```

## License

MIT
