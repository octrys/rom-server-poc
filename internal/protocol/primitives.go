package protocol

import (
	"fmt"
	"math"
)

// primitiveDecoders reads one primitive value off the reader. Signed integers
// decode to int64, unsigned to uint64, floats to float64.
var primitiveDecoders = map[string]func(*reader) (any, error){
	"System.Int32":  func(r *reader) (any, error) { v, err := r.u32(); return int64(int32(v)), err },
	"System.UInt32": func(r *reader) (any, error) { v, err := r.u32(); return uint64(v), err },
	"System.Int64":  func(r *reader) (any, error) { v, err := r.u64(); return int64(v), err },
	"System.UInt64": func(r *reader) (any, error) { v, err := r.u64(); return v, err },
	"System.Int16":  func(r *reader) (any, error) { v, err := r.u16(); return int64(int16(v)), err },
	"System.UInt16": func(r *reader) (any, error) { v, err := r.u16(); return uint64(v), err },
	"System.Byte":   func(r *reader) (any, error) { v, err := r.u8(); return uint64(v), err },
	"System.SByte":  func(r *reader) (any, error) { v, err := r.u8(); return int64(int8(v)), err },
	"System.Boolean": func(r *reader) (any, error) {
		v, err := r.u8()
		return v != 0, err
	},
	"System.Single": func(r *reader) (any, error) {
		v, err := r.u32()
		return float64(math.Float32frombits(v)), err
	},
	"System.Double": func(r *reader) (any, error) {
		v, err := r.u64()
		return math.Float64frombits(v), err
	},
	"System.String": func(r *reader) (any, error) { return r.str() },
	"System.Char[]": func(r *reader) (any, error) { return r.charArray() },
}

// primitiveEncoders writes one primitive value, coercing from any Go numeric.
var primitiveEncoders = map[string]func(*writer, any) error{
	"System.Int32":  func(w *writer, v any) error { n, err := toInt64(v); w.u32(uint32(int32(n))); return err },
	"System.UInt32": func(w *writer, v any) error { n, err := toUint64(v); w.u32(uint32(n)); return err },
	"System.Int64":  func(w *writer, v any) error { n, err := toInt64(v); w.u64(uint64(n)); return err },
	"System.UInt64": func(w *writer, v any) error { n, err := toUint64(v); w.u64(n); return err },
	"System.Int16":  func(w *writer, v any) error { n, err := toInt64(v); w.u16(uint16(int16(n))); return err },
	"System.UInt16": func(w *writer, v any) error { n, err := toUint64(v); w.u16(uint16(n)); return err },
	"System.Byte":   func(w *writer, v any) error { n, err := toUint64(v); w.u8(uint8(n)); return err },
	"System.SByte":  func(w *writer, v any) error { n, err := toInt64(v); w.u8(uint8(int8(n))); return err },
	"System.Boolean": func(w *writer, v any) error {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("protocol: expected bool, got %T", v)
		}
		if b {
			w.u8(1)
		} else {
			w.u8(0)
		}
		return nil
	},
	"System.Single": func(w *writer, v any) error {
		f, err := toFloat64(v)
		w.u32(math.Float32bits(float32(f)))
		return err
	},
	"System.Double": func(w *writer, v any) error {
		f, err := toFloat64(v)
		w.u64(math.Float64bits(f))
		return err
	},
	"System.String": func(w *writer, v any) error {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("protocol: expected string, got %T", v)
		}
		w.str(s)
		return nil
	},
	"System.Char[]": func(w *writer, v any) error {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("protocol: expected string, got %T", v)
		}
		w.charArray(s)
		return nil
	},
}

func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int8:
		return int64(n), nil
	case int16:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case uint:
		return int64(n), nil
	case uint8:
		return int64(n), nil
	case uint16:
		return int64(n), nil
	case uint32:
		return int64(n), nil
	case uint64:
		return int64(n), nil
	case float64:
		return int64(n), nil
	case float32:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("protocol: expected integer, got %T", v)
	}
}

func toUint64(v any) (uint64, error) {
	switch n := v.(type) {
	case int:
		return uint64(n), nil
	case int8:
		return uint64(n), nil
	case int16:
		return uint64(n), nil
	case int32:
		return uint64(n), nil
	case int64:
		return uint64(n), nil
	case uint:
		return uint64(n), nil
	case uint8:
		return uint64(n), nil
	case uint16:
		return uint64(n), nil
	case uint32:
		return uint64(n), nil
	case uint64:
		return n, nil
	case float64:
		return uint64(n), nil
	case float32:
		return uint64(n), nil
	default:
		return 0, fmt.Errorf("protocol: expected unsigned integer, got %T", v)
	}
}

func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("protocol: expected float, got %T", v)
	}
}
