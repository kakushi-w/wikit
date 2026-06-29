// Package jsonx produces JSON that is byte-for-byte identical to JavaScript's
// JSON.stringify(value, null, 4), which is what the original WikiComma (Node.js)
// uses for every metadata file it writes. Reproducing that exact byte layout is
// what lets a wikit backup be a drop-in continuation of an existing one.
//
// The differences from Go's encoding/json that matter here:
//
//   - 4-space indentation, ": " after keys, no trailing newline.
//   - HTML characters (< > &) are NOT escaped.
//   - Non-ASCII is emitted as raw UTF-8 (never \uXXXX).
//   - U+2028 / U+2029 are NOT escaped (encoding/json escapes them even with
//     SetEscapeHTML(false)).
//   - Object key order follows the ECMAScript rule: integer-index keys first in
//     ascending numeric order, then string keys in insertion order.
//   - An undefined field is omitted entirely; a null field is written as null.
//     The caller models "undefined" by simply not Set-ing the key.
package jsonx

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Object is an insertion-order-preserving JSON object. Marshal re-applies the
// ECMAScript key ordering at encode time, so callers add keys in whatever order
// the original code assigned the properties.
type Object struct {
	keys []string
	vals map[string]any
}

func NewObject() *Object {
	return &Object{vals: map[string]any{}}
}

// Set adds or updates a key. Like a JS property assignment, updating an existing
// key keeps its original position.
func (o *Object) Set(key string, val any) *Object {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = val
	return o
}

func (o *Object) Has(key string) bool { _, ok := o.vals[key]; return ok }

func (o *Object) Get(key string) (any, bool) { v, ok := o.vals[key]; return v, ok }

func (o *Object) Len() int { return len(o.keys) }

// Keys returns the keys in the order Marshal would emit them.
func (o *Object) Keys() []string {
	return orderKeys(o.keys)
}

// Number holds a verbatim numeric literal. It is emitted exactly as stored,
// which guarantees lossless round-tripping of numbers read from existing files.
type Number string

// Marshal renders v the way JSON.stringify(v, null, 4) would.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := encode(&buf, v, 0); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const indentUnit = "    "

func encode(buf *bytes.Buffer, v any, depth int) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case *Object:
		return encodeObject(buf, val, depth)
	case []any:
		return encodeArray(buf, val, depth)
	case string:
		encodeString(buf, val)
	case Number:
		buf.WriteString(string(val))
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case float64:
		buf.WriteString(formatFloat(val))
	default:
		return fmt.Errorf("jsonx: unsupported type %T", v)
	}
	return nil
}

func encodeObject(buf *bytes.Buffer, o *Object, depth int) error {
	if o == nil || len(o.keys) == 0 {
		buf.WriteString("{}")
		return nil
	}
	keys := orderKeys(o.keys)
	inner := strings.Repeat(indentUnit, depth+1)
	buf.WriteString("{\n")
	for i, k := range keys {
		buf.WriteString(inner)
		encodeString(buf, k)
		buf.WriteString(": ")
		if err := encode(buf, o.vals[k], depth+1); err != nil {
			return err
		}
		if i != len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(strings.Repeat(indentUnit, depth))
	buf.WriteByte('}')
	return nil
}

func encodeArray(buf *bytes.Buffer, arr []any, depth int) error {
	if len(arr) == 0 {
		buf.WriteString("[]")
		return nil
	}
	inner := strings.Repeat(indentUnit, depth+1)
	buf.WriteString("[\n")
	for i, el := range arr {
		buf.WriteString(inner)
		if err := encode(buf, el, depth+1); err != nil {
			return err
		}
		if i != len(arr)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(strings.Repeat(indentUnit, depth))
	buf.WriteByte(']')
	return nil
}

// orderKeys applies the ECMAScript own-property enumeration order: array-index
// keys ascending, then the remaining keys in insertion order.
func orderKeys(keys []string) []string {
	var idx []string
	var rest []string
	for _, k := range keys {
		if isArrayIndex(k) {
			idx = append(idx, k)
		} else {
			rest = append(rest, k)
		}
	}
	if len(idx) > 1 {
		sort.Slice(idx, func(i, j int) bool {
			a, _ := strconv.ParseUint(idx[i], 10, 64)
			b, _ := strconv.ParseUint(idx[j], 10, 64)
			return a < b
		})
	}
	if len(idx) == 0 {
		return keys
	}
	return append(idx, rest...)
}

// isArrayIndex reports whether k is a canonical integer index in the JS sense:
// the string form of a uint32 in [0, 2^32-2]. "0" is valid; "01" is not.
func isArrayIndex(k string) bool {
	if k == "" || len(k) > 10 {
		return false
	}
	if k == "0" {
		return true
	}
	if k[0] < '1' || k[0] > '9' {
		return false
	}
	for i := 1; i < len(k); i++ {
		if k[i] < '0' || k[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(k, 10, 64)
	if err != nil {
		return false
	}
	return n <= 4294967294
}

var shortEscapes = map[byte]string{
	'\b': `\b`,
	'\t': `\t`,
	'\n': `\n`,
	'\f': `\f`,
	'\r': `\r`,
	'"':  `\"`,
	'\\': `\\`,
}

// encodeString writes a JSON string literal using JS JSON.stringify's escaping:
// only ", \, and control chars below 0x20 are escaped; everything else (HTML
// chars, non-ASCII, U+2028/U+2029) is emitted verbatim as UTF-8.
func encodeString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if esc, ok := shortEscapes[c]; ok {
			buf.WriteString(esc)
		} else if c < 0x20 {
			buf.WriteString(`\u00`)
			const hex = "0123456789abcdef"
			buf.WriteByte(hex[c>>4])
			buf.WriteByte(hex[c&0xf])
		} else {
			buf.WriteByte(c)
		}
	}
	buf.WriteByte('"')
}

// formatFloat mirrors ECMAScript Number::toString for the finite values that
// occur in WikiComma data (integers and small-magnitude decimals).
func formatFloat(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "null"
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	abs := math.Abs(f)
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		return strconv.FormatFloat(f, 'e', -1, 64)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
