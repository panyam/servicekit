package http_test

import (
	"fmt"

	gohttp "github.com/panyam/servicekit/http"
)

// ExampleTypedJSONCodec demonstrates using TypedJSONCodec for strongly-typed
// JSON message encoding/decoding. Use this when your message types are known
// Go structs, for compile-time type safety.
func ExampleTypedJSONCodec() {
	type ChatMessage struct {
		User string `json:"user"`
		Text string `json:"text"`
	}
	type ChatResponse struct {
		Status string `json:"status"`
		ID     int    `json:"id"`
	}

	codec := &gohttp.TypedJSONCodec[ChatMessage, ChatResponse]{}

	// Encode a response
	data, msgType, err := codec.Encode(ChatResponse{Status: "sent", ID: 42})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Type: %d, Data: %s\n", msgType, data)

	// Decode an incoming message
	msg, err := codec.Decode([]byte(`{"user":"alice","text":"hello"}`), gohttp.TextMessage)
	if err != nil {
		panic(err)
	}
	fmt.Printf("User: %s, Text: %s\n", msg.User, msg.Text)

	// Output:
	// Type: 1, Data: {"status":"sent","id":42}
	// User: alice, Text: hello
}

// ExampleJSONCodec demonstrates the default untyped JSON codec.
// Use this for dynamic messages where the structure isn't known at compile time.
// This is what JSONConn uses internally.
func ExampleJSONCodec() {
	codec := &gohttp.JSONCodec{}

	// Encode any value
	data, _, err := codec.Encode(map[string]any{"action": "join", "room": "lobby"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("Encoded: %s\n", data)

	// Decode to any
	msg, err := codec.Decode(data, gohttp.TextMessage)
	if err != nil {
		panic(err)
	}
	m := msg.(map[string]any)
	fmt.Printf("Action: %s\n", m["action"])

	// Output:
	// Encoded: {"action":"join","room":"lobby"}
	// Action: join
}
