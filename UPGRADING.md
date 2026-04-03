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
