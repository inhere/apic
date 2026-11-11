package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Object is a JSON object that remembers the order its keys appeared in.
type Object struct {
	Keys []string
	Vals map[string]any
}

// Decode parses one JSON document keeping key order and number literals.
// The result is Object, []any, json.Number, string, bool or nil.
func Decode(body []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("invalid JSON: unexpected trailing content")
		}
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("invalid JSON: empty or truncated body")
		}
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // string, json.Number, bool or nil
	}
	switch delim {
	case '{':
		obj := Object{Vals: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("invalid JSON: %w", err)
			}
			key, _ := kt.(string)
			val, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			if _, dup := obj.Vals[key]; !dup {
				obj.Keys = append(obj.Keys, key)
			}
			obj.Vals[key] = val
		}
		if _, err := dec.Token(); err != nil { // closing }
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return obj, nil
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil { // closing ]
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return arr, nil
	}
	return nil, fmt.Errorf("invalid JSON: unexpected delimiter %v", delim)
}

// EncodeJSON writes v as JSON followed by no extra newline. Numbers keep their
// original literal, HTML characters are not escaped, object keys follow Keys.
func EncodeJSON(w io.Writer, v any, indent bool) error {
	var buf bytes.Buffer
	if err := encodeValue(&buf, v, indent, 0); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func encodeScalar(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	buf.Truncate(buf.Len() - 1) // Encode appends a newline
	return nil
}

func writeIndent(buf *bytes.Buffer, indent bool, depth int) {
	if !indent {
		return
	}
	buf.WriteByte('\n')
	for i := 0; i < depth; i++ {
		buf.WriteString("  ")
	}
}

func encodeValue(buf *bytes.Buffer, v any, indent bool, depth int) error {
	switch t := v.(type) {
	case Object:
		if len(t.Keys) == 0 {
			buf.WriteString("{}")
			return nil
		}
		buf.WriteByte('{')
		for i, k := range t.Keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeIndent(buf, indent, depth+1)
			if err := encodeScalar(buf, k); err != nil {
				return err
			}
			buf.WriteByte(':')
			if indent {
				buf.WriteByte(' ')
			}
			if err := encodeValue(buf, t.Vals[k], indent, depth+1); err != nil {
				return err
			}
		}
		writeIndent(buf, indent, depth)
		buf.WriteByte('}')
		return nil
	case []any:
		if len(t) == 0 {
			buf.WriteString("[]")
			return nil
		}
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeIndent(buf, indent, depth+1)
			if err := encodeValue(buf, e, indent, depth+1); err != nil {
				return err
			}
		}
		writeIndent(buf, indent, depth)
		buf.WriteByte(']')
		return nil
	default:
		return encodeScalar(buf, v) // json.Number encodes as its literal
	}
}
