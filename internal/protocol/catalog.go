// Package protocol is the single source of truth for the ROM game wire format:
// the message catalog (opcodes + typed, base-class-first field layouts) and the
// codec that turns messages to and from body bytes. The catalog data itself is
// generated into catalog_gen.go by cmd/gen; this file holds the types and the
// lookup helpers over it.
package protocol

// Field is one serialized member: a name and its IL2CPP type (e.g. System.Int32,
// System.String, System.Char[], a Protocol.* struct, a List<T>, or a T[]).
type Field struct {
	Name string
	Type string
}

// Message is a wire message: its class name, direction (C2S/S2C), opcode, and
// its full field layout with inherited base-class fields already flattened first.
type Message struct {
	Name   string
	Dir    string
	ID     uint32
	Fields []Field
}

// byName indexes the generated messages table by class name. Built lazily-free
// at init since the table is a compile-time constant.
var byName = func() map[string]*Message {
	m := make(map[string]*Message, len(messages))
	for _, msg := range messages {
		m[msg.Name] = msg
	}
	return m
}()

// Lookup returns the message with the given opcode, or nil.
func Lookup(opcode uint32) *Message { return messages[opcode] }

// LookupName returns the message with the given class name, or nil.
func LookupName(name string) *Message { return byName[name] }

// OpcodeOf returns the opcode for a message class name; the second result is
// false if the name is unknown.
func OpcodeOf(name string) (uint32, bool) {
	if m, ok := byName[name]; ok {
		return m.ID, true
	}
	return 0, false
}
