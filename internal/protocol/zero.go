package protocol

import "fmt"

// ZeroValues builds a fully-populated Values for a message with every field set
// to its type's zero: numerics to 0, booleans to false, strings to "", lists to
// empty, fixed arrays to their sized run of zeros, and nested structs recursed.
// Handlers start from this and override the fields they care about, so a reply
// is always structurally complete for Encode.
func ZeroValues(name string) (Values, error) {
	msg := byName[name]
	if msg == nil {
		return nil, fmt.Errorf("protocol: unknown message %q", name)
	}
	return zeroFields(msg.Fields, "Protocol."+msg.Name)
}

func zeroFields(fields []Field, owner string) (Values, error) {
	out := Values{}
	for _, f := range fields {
		v, err := zeroValue(f.Type, owner, f.Name)
		if err != nil {
			return nil, err
		}
		out[f.Name] = v
	}
	return out, nil
}

func zeroValue(ftype, owner, fname string) (any, error) {
	switch ftype {
	case "System.Int32", "System.Int64", "System.Int16", "System.SByte":
		return int64(0), nil
	case "System.UInt32", "System.UInt64", "System.UInt16", "System.Byte":
		return uint64(0), nil
	case "System.Single", "System.Double":
		return float64(0), nil
	case "System.Boolean":
		return false, nil
	case "System.String", "System.Char[]":
		return "", nil
	}
	if len(ftype) >= 2 && ftype[len(ftype)-2:] == "[]" {
		size, ok := fixedArrays[owner+"|"+fname]
		if !ok {
			return nil, fmt.Errorf("protocol: unsized array %s at %s.%s", ftype, owner, fname)
		}
		elem := ftype[:len(ftype)-2]
		out := make([]any, size)
		for i := range out {
			v, err := zeroValue(elem, owner, fname)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	if len(ftype) > len(listPrefix) && ftype[:len(listPrefix)] == listPrefix {
		return []any{}, nil // empty list
	}
	if layout, ok := structs[ftype]; ok {
		return zeroFields(layout, ftype)
	}
	return nil, fmt.Errorf("protocol: cannot zero %s at %s.%s", ftype, owner, fname)
}
