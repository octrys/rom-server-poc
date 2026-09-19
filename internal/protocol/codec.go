package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
)

// errStop signals that a field's type cannot be sized on the wire, so decoding
// must bail here (the caller keeps the remaining bytes as an opaque tail).
var errStop = errors.New("protocol: field type cannot be sized")

const listPrefix = "System.Collections.Generic.List<"

// Values is the dynamic representation of a decoded message body: field name to
// value. Integers decode to int64 (signed) or uint64 (unsigned), floats to
// float64, System.String / System.Char[] to string, List<T>/T[] to []any, and
// nested structs to Values. Encode accepts any Go numeric for a numeric field.
type Values map[string]any

// reader is a little-endian cursor over a body with the opcode already consumed.
type reader struct {
	buf []byte
	pos int
}

func (r *reader) need(n int) error {
	if r.pos+n > len(r.buf) {
		return errStop
	}
	return nil
}

func (r *reader) u8() (uint8, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	v := r.buf[r.pos]
	r.pos++
	return v, nil
}

func (r *reader) u16() (uint16, error) {
	if err := r.need(2); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v, nil
}

func (r *reader) u32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *reader) u64() (uint64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return v, nil
}

// str reads System.String: [u32 charCount][UTF-16LE].
func (r *reader) str() (string, error) {
	count, err := r.u32()
	if err != nil {
		return "", err
	}
	if err := r.need(int(count) * 2); err != nil {
		return "", err
	}
	units := make([]uint16, count)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(r.buf[r.pos:])
		r.pos += 2
	}
	return string(utf16.Decode(units)), nil
}

// charArray reads System.Char[]: NUL-terminated UTF-16LE, no length prefix.
func (r *reader) charArray() (string, error) {
	var units []uint16
	for {
		ch, err := r.u16()
		if err != nil {
			return "", err
		}
		if ch == 0 {
			break
		}
		units = append(units, ch)
	}
	return string(utf16.Decode(units)), nil
}

// writer is a little-endian buffer builder symmetric to reader.
type writer struct{ buf []byte }

func (w *writer) u8(v uint8)   { w.buf = append(w.buf, v) }
func (w *writer) u16(v uint16) { w.buf = binary.LittleEndian.AppendUint16(w.buf, v) }
func (w *writer) u32(v uint32) { w.buf = binary.LittleEndian.AppendUint32(w.buf, v) }
func (w *writer) u64(v uint64) { w.buf = binary.LittleEndian.AppendUint64(w.buf, v) }

func (w *writer) str(s string) {
	units := utf16.Encode([]rune(s))
	w.u32(uint32(len(units)))
	for _, u := range units {
		w.u16(u)
	}
}

func (w *writer) charArray(s string) {
	for _, u := range utf16.Encode([]rune(s)) {
		w.u16(u)
	}
	w.u16(0) // NUL terminator
}

// Encode builds a full message body ([u32 opcode][fields...]) for the named
// message from a Values map. Every declared field must be present.
func Encode(name string, values Values) ([]byte, error) {
	msg := byName[name]
	if msg == nil {
		return nil, fmt.Errorf("protocol: unknown message %q", name)
	}
	w := &writer{}
	w.u32(msg.ID)
	owner := "Protocol." + msg.Name
	for _, f := range msg.Fields {
		v, ok := values[f.Name]
		if !ok {
			return nil, fmt.Errorf("protocol: %s missing field %q (%s)", name, f.Name, f.Type)
		}
		if err := encodeValue(w, f.Type, v, owner, f.Name); err != nil {
			return nil, err
		}
	}
	return w.buf, nil
}

func encodeValue(w *writer, ftype string, v any, owner, fname string) error {
	if enc, ok := primitiveEncoders[ftype]; ok {
		return enc(w, v)
	}
	if len(ftype) >= 2 && ftype[len(ftype)-2:] == "[]" {
		size, ok := fixedArrays[owner+"|"+fname]
		if !ok {
			return fmt.Errorf("protocol: unsized array %s at %s.%s", ftype, owner, fname)
		}
		items, ok := v.([]any)
		if !ok {
			return fmt.Errorf("protocol: %s.%s expected []any, got %T", owner, fname, v)
		}
		if len(items) != size {
			return fmt.Errorf("protocol: %s.%s expected %d elements, got %d", owner, fname, size, len(items))
		}
		elem := ftype[:len(ftype)-2]
		for _, item := range items {
			if err := encodeValue(w, elem, item, owner, fname); err != nil {
				return err
			}
		}
		return nil
	}
	if len(ftype) > len(listPrefix) && ftype[:len(listPrefix)] == listPrefix {
		inner := ftype[len(listPrefix) : len(ftype)-1]
		items, ok := v.([]any)
		if !ok {
			return fmt.Errorf("protocol: %s.%s expected []any list, got %T", owner, fname, v)
		}
		w.u32(uint32(len(items)))
		for _, item := range items {
			if err := encodeValue(w, inner, item, owner, fname); err != nil {
				return err
			}
		}
		return nil
	}
	if layout, ok := structs[ftype]; ok {
		if messageFullNames[ftype] { // message used as element: prefix its opcode
			if op, ok := OpcodeOf(ftype[len("Protocol."):]); ok {
				w.u32(op)
			}
		}
		fields, ok := v.(Values)
		if !ok {
			if m, isMap := v.(map[string]any); isMap {
				fields = Values(m)
			} else {
				return fmt.Errorf("protocol: %s expected Values, got %T", ftype, v)
			}
		}
		for _, f := range layout {
			fv, ok := fields[f.Name]
			if !ok {
				return fmt.Errorf("protocol: %s missing field %q (%s)", ftype, f.Name, f.Type)
			}
			if err := encodeValue(w, f.Type, fv, ftype, f.Name); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("protocol: cannot encode %s at %s.%s", ftype, owner, fname)
}

// Decode parses a full plaintext body into (message, values, opaque tail). When
// a field's type cannot be sized, decoding stops cleanly and the unread bytes
// are returned as tail; msg is nil for an unknown opcode.
func Decode(body []byte) (msg *Message, values Values, tail []byte, err error) {
	if len(body) < 4 {
		return nil, nil, body, fmt.Errorf("protocol: body too short (%d bytes)", len(body))
	}
	opcode := binary.LittleEndian.Uint32(body)
	msg = messages[opcode]
	r := &reader{buf: body[4:]}
	values = Values{}
	if msg == nil {
		return nil, values, r.buf[r.pos:], nil
	}
	owner := "Protocol." + msg.Name
	for _, f := range msg.Fields {
		mark := r.pos
		v, derr := decodeValue(r, f.Type, owner, f.Name)
		if derr != nil {
			r.pos = mark // leave the field's bytes in the tail, unconsumed
			break
		}
		values[f.Name] = v
	}
	return msg, values, r.buf[r.pos:], nil
}

func decodeValue(r *reader, ftype, owner, fname string) (any, error) {
	if dec, ok := primitiveDecoders[ftype]; ok {
		return dec(r)
	}
	if len(ftype) >= 2 && ftype[len(ftype)-2:] == "[]" {
		size, ok := fixedArrays[owner+"|"+fname]
		if !ok {
			return nil, errStop
		}
		elem := ftype[:len(ftype)-2]
		out := make([]any, size)
		for i := range out {
			v, err := decodeValue(r, elem, owner, fname)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	if len(ftype) > len(listPrefix) && ftype[:len(listPrefix)] == listPrefix {
		inner := ftype[len(listPrefix) : len(ftype)-1]
		n, err := r.u32()
		if err != nil {
			return nil, err
		}
		out := make([]any, n)
		for i := range out {
			v, err := decodeValue(r, inner, owner, fname)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	if layout, ok := structs[ftype]; ok {
		if messageFullNames[ftype] {
			if _, err := r.u32(); err != nil { // consume the element's opcode prefix
				return nil, err
			}
		}
		out := Values{}
		for _, f := range layout {
			v, err := decodeValue(r, f.Type, ftype, f.Name)
			if err != nil {
				return nil, err
			}
			out[f.Name] = v
		}
		return out, nil
	}
	return nil, errStop
}
