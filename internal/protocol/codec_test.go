package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

// TestCheckConnectionRoundTrip covers the handshake message: six Int64 fields,
// encoded then decoded back to the same values, with a byte-stable opcode header.
func TestCheckConnectionRoundTrip(t *testing.T) {
	in := Values{
		"m_socketUID":         int64(5623448),
		"m_connectionKey":     int64(731644093977935779),
		"m_clientSendIV":      int64(-625),
		"m_serverSendIV":      int64(42),
		"m_encryptionKeyLow":  int64(-1),
		"m_encryptionKeyHigh": int64(1234567890123),
	}
	body, err := Encode("S2C_CheckConnection", in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := body[:4]; !bytes.Equal(got, []byte{0x1F, 0x59, 0xE4, 0x32}) {
		t.Fatalf("opcode header = % x, want 1f 59 e4 32", got)
	}

	msg, out, tail, err := Decode(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg == nil || msg.Name != "S2C_CheckConnection" {
		t.Fatalf("decoded wrong message: %v", msg)
	}
	if len(tail) != 0 {
		t.Fatalf("unexpected tail: % x", tail)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip mismatch\n in: %#v\nout: %#v", in, out)
	}
}

// TestStringAndBoolRoundTrip exercises System.String + System.Boolean via the
// login message, which also carries Char[] device fields.
func TestSessionAuthLoginRoundTrip(t *testing.T) {
	msg := LookupName("C2S_SessionAuthLogin")
	if msg == nil {
		t.Skip("C2S_SessionAuthLogin not in catalog")
	}
	in := Values{}
	for _, f := range msg.Fields {
		v := primitiveZero(f.Type)
		if v == nil {
			t.Skipf("field %q has composite type %s", f.Name, f.Type)
		}
		in[f.Name] = v
	}
	body, err := Encode("C2S_SessionAuthLogin", in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, out, tail, err := Decode(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tail) != 0 {
		t.Fatalf("unexpected tail: % x", tail)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip mismatch\n in: %#v\nout: %#v", in, out)
	}
}

// primitiveZero returns a decode-shaped zero for a primitive field type, or nil
// for a composite type (the caller skips messages that have any).
func primitiveZero(ftype string) any {
	switch ftype {
	case "System.UInt32", "System.UInt64", "System.UInt16", "System.Byte":
		return uint64(0)
	case "System.Int32", "System.Int64", "System.Int16", "System.SByte":
		return int64(0)
	case "System.Single", "System.Double":
		return float64(0)
	case "System.Boolean":
		return false
	case "System.String", "System.Char[]":
		return "x"
	default:
		return nil
	}
}
