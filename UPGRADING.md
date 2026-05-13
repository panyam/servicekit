# Upgrading Guide

## JSONConn → Typed BaseConn (Optional)

`JSONConn` (alias for `BaseConn[any, any]`) works well for dynamic JSON and is **not deprecated**. But if you want compile-time type safety, you can upgrade to typed messages with minimal changes.

### Before: JSONConn (untyped)

```go
type MyConn struct {
    gohttp.JSONConn
}

func (c *MyConn) HandleMessage(msg any) error {
    // Must type-assert at runtime
    m := msg.(map[string]any)
    action := m["action"].(string)
    // ...
    c.SendOutput(map[string]any{"status": "ok"})
    return nil
}

type MyHandler struct{}

func (h *MyHandler) Validate(w http.ResponseWriter, r *http.Request) (*MyConn, bool) {
    return &MyConn{
        JSONConn: gohttp.JSONConn{Codec: &gohttp.JSONCodec{}, NameStr: "MyConn"},
    }, true
}
```

### After: BaseConn with TypedJSONCodec (typed)

```go
type Command struct {
    Action string `json:"action"`
    Data   any    `json:"data"`
}

type Response struct {
    Status string `json:"status"`
    Error  string `json:"error,omitempty"`
}

type MyConn struct {
    gohttp.BaseConn[Command, Response]
}

func (c *MyConn) HandleMessage(msg Command) error {
    // msg is already typed — no assertion needed
    switch msg.Action {
    case "join":
        c.SendOutput(Response{Status: "ok"})
    default:
        c.SendOutput(Response{Status: "error", Error: "unknown action"})
    }
    return nil
}

type MyHandler struct{}

func (h *MyHandler) Validate(w http.ResponseWriter, r *http.Request) (*MyConn, bool) {
    return &MyConn{
        BaseConn: gohttp.BaseConn[Command, Response]{
            Codec:   &gohttp.TypedJSONCodec[Command, Response]{},
            NameStr: "MyConn",
        },
    }, true
}
```

### What changes

| | JSONConn (untyped) | BaseConn[I, O] (typed) |
|---|---|---|
| **Embed** | `gohttp.JSONConn` | `gohttp.BaseConn[Input, Output]` |
| **Codec** | `&gohttp.JSONCodec{}` | `&gohttp.TypedJSONCodec[I, O]{}` |
| **HandleMessage** | `msg any` — type-assert at runtime | `msg Input` — typed at compile time |
| **SendOutput** | `any` — pass `map[string]any` | `Output` — pass typed struct |
| **Wire format** | JSON (unchanged) | JSON (unchanged) |

### When to upgrade

- **Stay with JSONConn** if: messages have variable structure, you're prototyping, or you handle many different message types in one connection
- **Upgrade to typed** if: you have well-defined request/response types, want IDE autocomplete, or want the compiler to catch type mismatches

The wire format is identical — both use JSON text frames. Clients don't need to change.

## http.Call: generic signature, options, secure TLS default (issue #28)

`http.Call` was reworked to address five gaps: no context propagation, no typed destination, 204 No Content failed, error body/headers were lost, and TLS verification was disabled on the default clients. This is a breaking change.

### Signature change

Before:
```go
func Call(req *http.Request, client *http.Client) (any, error)
```

After:
```go
func Call[T any](ctx context.Context, req *http.Request, opts ...CallOption) (T, error)
func CallVoid(ctx context.Context, req *http.Request, opts ...CallOption) error
```

- `ctx` is explicit — cancel mid-call by canceling the context.
- `T` is the decoded type. No more `any` round-trip-via-map.
- `CallVoid` for endpoints whose response body you don't need (DELETE 204, ack-style POST).
- Empty 2xx bodies (204 No Content) now succeed: `Call[T]` returns the zero value, `CallVoid` returns nil.

### Call-site migration

| Use | Before | After |
|---|---|---|
| Typed JSON response | `r, err := Call(req, c)` then re-decode | `u, err := Call[User](ctx, req, WithClient(c))` |
| Map/dynamic response | `r, err := Call(req, c)` | `m, err := Call[map[string]any](ctx, req, WithClient(c))` |
| 204 / void response | (errored) | `err := CallVoid(ctx, req)` |
| Custom client | second positional arg | `Call[T](ctx, req, WithClient(c))` |

### HTTPError now preserves body and headers

Before:
```go
type HTTPError struct {
    Code    int
    Message string
}
```

After:
```go
type HTTPError struct {
    Code   int
    Body   []byte       // raw response body
    Header http.Header  // response headers (Retry-After, rate-limit, etc.)
}
```

If you previously read `Message`, switch to `string(err.Body)`. `HTTPErrorCode(err)` is unchanged.

### TLS default flipped to secure

`DefaultHttpClient`, `LowQPSHttpClient`, `MediumQPSHttpClient`, and `HighQPSHttpClient` no longer set `InsecureSkipVerify: true`. Calls to public APIs now verify certificates by default.

For internal endpoints with self-signed certificates, use `WithInsecureTLS()`:

```go
data, err := Call[Response](ctx, req, WithInsecureTLS())
```

Or pass a custom `*http.Client` whose transport you control via `WithClient`.
