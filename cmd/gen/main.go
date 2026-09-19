// Command gen turns the exported protocol metadata (messages_typed.json +
// rom_dump.json + rom_struct_bases.json) into a single Go file, catalog_gen.go,
// holding every message's fully-resolved (base-class-first) wire layout, the
// nested struct layouts they reach, and the fixed-array sizes.
//
// It builds the catalog the runtime codec (internal/protocol/codec.go) reads.
//
// Run it via `make gen CATALOG_DIR=<dir>`, or pass the input paths as flags.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"
)

// typedMessage mirrors one entry of messages_typed.json.
type typedMessage struct {
	Name   string  `json:"name"`
	Dir    string  `json:"dir"`
	ID     int64   `json:"id"`
	Fields []field `json:"fields"`
}

// field is a (name, type) pair; type is the IL2CPP full name (e.g. System.Int32).
type field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// dumpType mirrors one entry of rom_dump.json.
type dumpType struct {
	Full   string `json:"full"`
	Fields []struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		IsStatic bool   `json:"isStatic"`
	} `json:"fields"`
}

const listPrefix = "System.Collections.Generic.List<"

// resolved is a message paired with its fully flattened (base-first) field list.
type resolved struct {
	msg    typedMessage
	fields []field
}

// extraBaseFields are base-class field lists the frida dump omits. Network's
// IBSerializableList prepends a leading __state__ byte, serialized base-first.
var extraBaseFields = map[string][]field{
	"Network.IBSerializableList": {{Name: "__state__", Type: "System.Byte"}},
}

// fixedArrays sizes the compile-time-constant C# arrays (T[], no length prefix),
// keyed by "<owner>|<field>". Sizes are compile-time constants read from the client.
var fixedArrays = map[string]int{
	"Protocol.ItemOptions|m_optionIndex":             10,
	"Protocol.AbilityValue|m_data":                   249,
	"Protocol.S2C_ContentsSetting|m_levelLimits":     10,
	"Protocol.S2C_ContentsSetting|m_values":          3,
	"Protocol.S2C_EquipList|m_usePresetSlot":         2,
	"Protocol.S2C_EquipList|m_equipItemList":         2,
	"Protocol.S2C_EquipList|m_equipCostume":          2,
	"Protocol.S2C_EquipList|m_equipPet":              2,
	"Protocol.S2C_EquipList|m_monolithEquipItemList": 2,
}

func main() {
	typedPath := flag.String("typed", "", "path to messages_typed.json (required)")
	dumpPath := flag.String("dump", "", "path to rom_dump.json (required)")
	basesPath := flag.String("bases", "", "path to rom_struct_bases.json (required)")
	outPath := flag.String("out", "internal/protocol/catalog_gen.go", "output Go file")
	flag.Parse()

	if *typedPath == "" || *dumpPath == "" || *basesPath == "" {
		flag.Usage()
		log.Fatal("gen: -typed, -dump and -bases are required")
	}

	messages := loadTyped(*typedPath)
	own := loadOwn(*dumpPath)
	bases := loadBases(*basesPath)

	structs := buildStructs(own, bases)
	messageFull := make(map[string]bool, len(messages))
	for _, m := range messages {
		messageFull["Protocol."+m.Name] = true
	}

	// Resolve each message to its full base-first field list, and collect the
	// struct types those fields (transitively) reach.
	out := make([]resolved, 0, len(messages))
	reachable := map[string]bool{}
	for _, m := range messages {
		full := messageFields(m, bases, own)
		out = append(out, resolved{msg: m, fields: full})
		for _, f := range full {
			collectReachable(f.Type, structs, reachable)
		}
	}

	emit(*outPath, out, structs, reachable, messages, messageFull)
	log.Printf("gen: wrote %s (%d messages, %d structs)", *outPath, len(out), len(reachable))
}

func loadTyped(path string) []typedMessage {
	var messages []typedMessage
	readJSON(path, &messages)
	return messages
}

// loadOwn returns each dumped type's own (non-static, fully typed) fields.
func loadOwn(path string) map[string][]field {
	var types []dumpType
	readJSON(path, &types)
	own := map[string][]field{}
	for _, t := range types {
		if len(t.Fields) == 0 {
			continue
		}
		fields := make([]field, 0, len(t.Fields))
		ok := true
		for _, f := range t.Fields {
			if f.Type == "" {
				ok = false
				break
			}
			if f.IsStatic {
				continue // static fields (e.g. __ID__) are not serialized
			}
			fields = append(fields, field{Name: f.Name, Type: f.Type})
		}
		if ok {
			own[t.Full] = fields
		}
	}
	for name, fields := range extraBaseFields {
		own[name] = fields
	}
	return own
}

func loadBases(path string) map[string]string {
	var bases map[string]string
	readJSON(path, &bases)
	return bases
}

// chain returns [full, parent, ..., root] following the class -> base map.
func chain(full string, bases map[string]string) []string {
	var out []string
	cur := full
	for {
		next, ok := bases[cur]
		if !ok {
			break
		}
		out = append(out, cur)
		cur = next
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// fieldsOf concatenates the classes' own fields emitted BASE-FIRST.
func fieldsOf(classes []string, own map[string][]field) []field {
	var fields []field
	for i := len(classes) - 1; i >= 0; i-- {
		fields = append(fields, own[classes[i]]...)
	}
	return fields
}

func buildStructs(own map[string][]field, bases map[string]string) map[string][]field {
	structs := map[string][]field{}
	for name := range own {
		layout := fieldsOf(chain(name, bases), own)
		if len(layout) == 0 {
			layout = own[name]
		}
		structs[name] = layout
	}
	return structs
}

// messageFields returns a message's full field list: inherited base fields
// (classes strictly above the message) first, then its own declared fields.
func messageFields(m typedMessage, bases map[string]string, own map[string][]field) []field {
	full := "Protocol." + m.Name
	ch := chain(full, bases)
	var prefix []field
	if len(ch) > 1 {
		prefix = fieldsOf(ch[1:], own)
	}
	return append(prefix, m.Fields...)
}

// collectReachable walks a field type, adding every struct layout it reaches.
func collectReachable(ftype string, structs map[string][]field, reachable map[string]bool) {
	base := ftype
	if strings.HasSuffix(base, "[]") {
		base = base[:len(base)-2]
	}
	if strings.HasPrefix(base, listPrefix) {
		base = base[len(listPrefix) : len(base)-1]
	}
	if reachable[base] {
		return
	}
	layout, ok := structs[base]
	if !ok {
		return
	}
	reachable[base] = true
	for _, f := range layout {
		collectReachable(f.Type, structs, reachable)
	}
}

func emit(path string, resolved []resolved, structs map[string][]field, reachable map[string]bool, messages []typedMessage, messageFull map[string]bool) {
	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/gen; DO NOT EDIT.\n\n")
	b.WriteString("package protocol\n\n")

	// Opcode constants.
	b.WriteString("// Message opcodes (__ID__). These rotate on every client update.\n")
	b.WriteString("const (\n")
	for _, m := range messages {
		fmt.Fprintf(&b, "\tOp%s uint32 = 0x%08X\n", m.Name, uint32(m.ID))
	}
	b.WriteString(")\n\n")

	// Messages table.
	b.WriteString("var messages = map[uint32]*Message{\n")
	for _, r := range resolved {
		fmt.Fprintf(&b, "\t0x%08X: {Name: %q, Dir: %q, ID: 0x%08X, Fields: []Field{\n",
			uint32(r.msg.ID), r.msg.Name, r.msg.Dir, uint32(r.msg.ID))
		for _, f := range r.fields {
			fmt.Fprintf(&b, "\t\t{Name: %q, Type: %q},\n", f.Name, f.Type)
		}
		b.WriteString("\t}},\n")
	}
	b.WriteString("}\n\n")

	// Struct layouts (only those reachable from messages).
	names := make([]string, 0, len(reachable))
	for name := range reachable {
		names = append(names, name)
	}
	sort.Strings(names)
	b.WriteString("var structs = map[string][]Field{\n")
	for _, name := range names {
		fmt.Fprintf(&b, "\t%q: {\n", name)
		for _, f := range structs[name] {
			fmt.Fprintf(&b, "\t\t{Name: %q, Type: %q},\n", f.Name, f.Type)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")

	// Fixed-array sizes.
	fixedKeys := make([]string, 0, len(fixedArrays))
	for k := range fixedArrays {
		fixedKeys = append(fixedKeys, k)
	}
	sort.Strings(fixedKeys)
	b.WriteString("var fixedArrays = map[string]int{\n")
	for _, k := range fixedKeys {
		fmt.Fprintf(&b, "\t%q: %d,\n", k, fixedArrays[k])
	}
	b.WriteString("}\n\n")

	// Message full names (elements carrying an opcode prefix), reachable only.
	fullNames := make([]string, 0)
	for name := range reachable {
		if messageFull[name] {
			fullNames = append(fullNames, name)
		}
	}
	sort.Strings(fullNames)
	b.WriteString("var messageFullNames = map[string]bool{\n")
	for _, name := range fullNames {
		fmt.Fprintf(&b, "\t%q: true,\n", name)
	}
	b.WriteString("}\n")

	formatted, err := format.Source(b.Bytes())
	if err != nil {
		// Write unformatted output so the error is inspectable.
		_ = os.WriteFile(path, b.Bytes(), 0o644)
		log.Fatalf("gen: gofmt failed (wrote raw): %v", err)
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		log.Fatalf("gen: write %s: %v", path, err)
	}
}

func readJSON(path string, v any) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("gen: read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		log.Fatalf("gen: parse %s: %v", path, err)
	}
}
