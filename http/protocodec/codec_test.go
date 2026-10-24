package protocodec

import (
	"testing"

	gohttp "github.com/panyam/servicekit/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProtoJSONCodec_RoundTripWithFactoryAndExemplar(t *testing.T) {
	in := &timestamppb.Timestamp{Seconds: 1000, Nanos: 5}
	for name, codec := range map[string]*ProtoJSONCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{
		"factory":  {NewInput: func() *timestamppb.Timestamp { return &timestamppb.Timestamp{} }},
		"exemplar": {Input: &timestamppb.Timestamp{}},
	} {
		t.Run(name, func(t *testing.T) {
			data, mt, err := codec.Encode(in)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if mt != gohttp.TextMessage {
				t.Fatalf("got message type %v, want TextMessage", mt)
			}
			out, err := codec.Decode(data, mt)
			if err != nil {
				t.Fatalf("decode %s: %v", data, err)
			}
			if !proto.Equal(in, out) {
				t.Fatalf("round trip changed the message: %v -> %v", in, out)
			}
		})
	}
}

func TestProtoJSONCodec_UsesUnmarshalOptions(t *testing.T) {
	unknownField := []byte(`{"x": 1}`)
	strict := &ProtoJSONCodec[*emptypb.Empty, *emptypb.Empty]{Input: &emptypb.Empty{}}
	if _, err := strict.Decode(unknownField, gohttp.TextMessage); err == nil {
		t.Fatal("default options should reject an unknown field")
	}
	lenient := &ProtoJSONCodec[*emptypb.Empty, *emptypb.Empty]{
		Input:            &emptypb.Empty{},
		UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
	}
	if _, err := lenient.Decode(unknownField, gohttp.TextMessage); err != nil {
		t.Fatalf("DiscardUnknown should accept an unknown field: %v", err)
	}
}

func TestBinaryProtoCodec_RoundTrip(t *testing.T) {
	codec := &BinaryProtoCodec[*timestamppb.Timestamp, *timestamppb.Timestamp]{Input: &timestamppb.Timestamp{}}
	in := &timestamppb.Timestamp{Seconds: 42}
	data, mt, err := codec.Encode(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if mt != gohttp.BinaryMessage {
		t.Fatalf("got message type %v, want BinaryMessage", mt)
	}
	out, err := codec.Decode(data, mt)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !proto.Equal(in, out) {
		t.Fatalf("round trip changed the message: %v -> %v", in, out)
	}
}
