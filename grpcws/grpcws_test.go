package grpcws

import (
	"encoding/json"
	"testing"
	"time"

	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ============================================================================
// GRPCWSCodec Tests
// ============================================================================

func TestGRPCWSCodec_Decode(t *testing.T) {
	codec := &GRPCWSCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{
		NewRequest: func() *timestamppb.Timestamp { return &timestamppb.Timestamp{} },
	}

	tests := []struct {
		name    string
		input   string
		msgType gohttp.MessageType
		want    ControlMessage
		wantErr bool
	}{
		{
			name:    "data message",
			input:   `{"type":"data","data":{"seconds":1234567890}}`,
			msgType: gohttp.TextMessage,
			want: ControlMessage{
				Type: TypeData,
				Data: map[string]any{"seconds": float64(1234567890)},
			},
		},
		{
			name:    "ping message",
			input:   `{"type":"ping","pingId":42}`,
			msgType: gohttp.TextMessage,
			want: ControlMessage{
				Type:   TypePing,
				PingId: 42,
			},
		},
		{
			name:    "pong message",
			input:   `{"type":"pong","pingId":42}`,
			msgType: gohttp.TextMessage,
			want: ControlMessage{
				Type:   TypePong,
				PingId: 42,
			},
		},
		{
			name:    "cancel message",
			input:   `{"type":"cancel"}`,
			msgType: gohttp.TextMessage,
			want: ControlMessage{
				Type: TypeCancel,
			},
		},
		{
			name:    "end_send message",
			input:   `{"type":"end_send"}`,
			msgType: gohttp.TextMessage,
			want: ControlMessage{
				Type: TypeEndSend,
			},
		},
		{
			name:    "error message",
			input:   `{"type":"error","error":"something went wrong"}`,
			msgType: gohttp.TextMessage,
			want: ControlMessage{
				Type:  TypeError,
				Error: "something went wrong",
			},
		},
		{
			name:    "invalid json",
			input:   `{invalid}`,
			msgType: gohttp.TextMessage,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := codec.Decode([]byte(tt.input), tt.msgType)
			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.Type != tt.want.Type {
				t.Errorf("Decode() Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.PingId != tt.want.PingId {
				t.Errorf("Decode() PingId = %v, want %v", got.PingId, tt.want.PingId)
			}
			if got.Error != tt.want.Error {
				t.Errorf("Decode() Error = %v, want %v", got.Error, tt.want.Error)
			}
		})
	}
}

func TestGRPCWSCodec_Encode(t *testing.T) {
	codec := &GRPCWSCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{}

	tests := []struct {
		name    string
		msg     ControlMessage
		wantErr bool
	}{
		{
			name: "data message",
			msg: ControlMessage{
				Type: TypeData,
				Data: map[string]any{"key": "value"},
			},
		},
		{
			name: "error message",
			msg: ControlMessage{
				Type:  TypeError,
				Error: "test error",
			},
		},
		{
			name: "stream_end message",
			msg: ControlMessage{
				Type: TypeStreamEnd,
			},
		},
		{
			name: "ping message",
			msg: ControlMessage{
				Type:   TypePing,
				PingId: 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, msgType, err := codec.Encode(tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Encode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if msgType != gohttp.TextMessage {
				t.Errorf("Encode() msgType = %v, want TextMessage", msgType)
			}

			// Verify it's valid JSON
			var decoded ControlMessage
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Errorf("Encode() produced invalid JSON: %v", err)
			}
			if decoded.Type != tt.msg.Type {
				t.Errorf("Encode() roundtrip Type = %v, want %v", decoded.Type, tt.msg.Type)
			}
		})
	}
}

func TestGRPCWSCodec_EncodePing(t *testing.T) {
	codec := &GRPCWSCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{}

	data, msgType, err := codec.EncodePing(42, "conn-123", "TestConn")
	if err != nil {
		t.Fatalf("EncodePing() error = %v", err)
	}
	if msgType != gohttp.TextMessage {
		t.Errorf("EncodePing() msgType = %v, want TextMessage", msgType)
	}

	var msg ControlMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("EncodePing() produced invalid JSON: %v", err)
	}
	if msg.Type != TypePing {
		t.Errorf("EncodePing() Type = %v, want %v", msg.Type, TypePing)
	}
	if msg.PingId != 42 {
		t.Errorf("EncodePing() PingId = %v, want 42", msg.PingId)
	}
}

func TestGRPCWSCodec_EncodeData(t *testing.T) {
	codec := &GRPCWSCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{}

	ts := timestamppb.New(time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC))

	msg, err := codec.EncodeData(ts)
	if err != nil {
		t.Fatalf("EncodeData() error = %v", err)
	}
	if msg.Type != TypeData {
		t.Errorf("EncodeData() Type = %v, want %v", msg.Type, TypeData)
	}
	if msg.Data == nil {
		t.Error("EncodeData() Data is nil")
	}

	// protojson encodes Timestamp as a string in RFC 3339 format
	// Verify the data is present (could be string or map depending on message type)
	t.Logf("EncodeData() Data type: %T, value: %v", msg.Data, msg.Data)
}

func TestGRPCWSCodec_DecodeRequest(t *testing.T) {
	codec := &GRPCWSCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{
		NewRequest: func() *timestamppb.Timestamp { return &timestamppb.Timestamp{} },
	}

	// protojson expects Timestamp as RFC 3339 string format
	msg := ControlMessage{
		Type: TypeData,
		Data: "2025-01-15T10:30:00Z",
	}

	req, err := codec.DecodeRequest(msg)
	if err != nil {
		t.Fatalf("DecodeRequest() error = %v", err)
	}

	// Verify the timestamp was parsed correctly
	expected := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	if req.AsTime() != expected {
		t.Errorf("DecodeRequest() time = %v, want %v", req.AsTime(), expected)
	}
}

// ============================================================================
// StreamMetrics Tests
// ============================================================================

func TestStreamMetrics_IncrementSent(t *testing.T) {
	m := &StreamMetrics{}

	for i := int64(1); i <= 5; i++ {
		got := m.IncrementSent()
		if got != i {
			t.Errorf("IncrementSent() = %v, want %v", got, i)
		}
	}

	if m.MsgsSent != 5 {
		t.Errorf("MsgsSent = %v, want 5", m.MsgsSent)
	}
}

func TestStreamMetrics_IncrementReceived(t *testing.T) {
	m := &StreamMetrics{}

	for i := int64(1); i <= 3; i++ {
		got := m.IncrementReceived()
		if got != i {
			t.Errorf("IncrementReceived() = %v, want %v", got, i)
		}
	}

	if m.MsgsReceived != 3 {
		t.Errorf("MsgsReceived = %v, want 3", m.MsgsReceived)
	}
}

// ============================================================================
// ControlMessage Tests
// ============================================================================

func TestControlMessage_Marshaling(t *testing.T) {
	tests := []struct {
		name string
		msg  ControlMessage
	}{
		{
			name: "data with map payload",
			msg: ControlMessage{
				Type: TypeData,
				Data: map[string]any{"key": "value", "num": 42.0},
			},
		},
		{
			name: "error message",
			msg: ControlMessage{
				Type:  TypeError,
				Error: "test error message",
			},
		},
		{
			name: "ping with id",
			msg: ControlMessage{
				Type:   TypePing,
				PingId: 12345,
			},
		},
		{
			name: "stream_end",
			msg: ControlMessage{
				Type: TypeStreamEnd,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal
			var decoded ControlMessage
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// Compare
			if decoded.Type != tt.msg.Type {
				t.Errorf("Type = %v, want %v", decoded.Type, tt.msg.Type)
			}
			if decoded.Error != tt.msg.Error {
				t.Errorf("Error = %v, want %v", decoded.Error, tt.msg.Error)
			}
			if decoded.PingId != tt.msg.PingId {
				t.Errorf("PingId = %v, want %v", decoded.PingId, tt.msg.PingId)
			}
		})
	}
}

// ============================================================================
// Interface Compliance Tests
// ============================================================================

func TestCodecInterfaceCompliance(t *testing.T) {
	// Verify GRPCWSCodec implements the Codec interface
	var _ gohttp.Codec[ControlMessage, ControlMessage] = (*GRPCWSCodec[proto.Message, proto.Message])(nil)
}

// ============================================================================
// Message Type Constants Tests
// ============================================================================

func TestMessageTypeConstants(t *testing.T) {
	// Verify message type constants have expected values
	if TypeData != "data" {
		t.Errorf("TypeData = %v, want 'data'", TypeData)
	}
	if TypeError != "error" {
		t.Errorf("TypeError = %v, want 'error'", TypeError)
	}
	if TypeStreamEnd != "stream_end" {
		t.Errorf("TypeStreamEnd = %v, want 'stream_end'", TypeStreamEnd)
	}
	if TypePing != "ping" {
		t.Errorf("TypePing = %v, want 'ping'", TypePing)
	}
	if TypePong != "pong" {
		t.Errorf("TypePong = %v, want 'pong'", TypePong)
	}
	if TypeCancel != "cancel" {
		t.Errorf("TypeCancel = %v, want 'cancel'", TypeCancel)
	}
	if TypeEndSend != "end_send" {
		t.Errorf("TypeEndSend = %v, want 'end_send'", TypeEndSend)
	}
}
