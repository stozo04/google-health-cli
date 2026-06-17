// Package dailylog ports google_health/daily_log.py: read / merge / upsert /
// write DAILY_LOG.json. The write is the #1 fidelity surface (GOAL.md §2, §11):
// it must be byte-identical to Python's json.dump(ensure_ascii=False, indent=2)
// + trailing newline, preserving the key order of every existing key and the
// "source": "ghealth" marker.
package dailylog

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Object is an order-preserving JSON object. It remembers key order and stores
// each value as raw JSON, so unknown keys and nested structures pass through
// untouched and we only mutate what we intend (days[] and a day's training).
// Re-serialization through the standard Encoder + Indent reproduces Python's
// pretty-printing exactly (GOAL.md §11): 2-space indent, ": " after keys, items
// separated by ",\n", with HTML escaping OFF (the zone2 label carries '<'/'>').
type Object struct {
	keys []string
	vals map[string]json.RawMessage
}

// NewObject returns an empty ordered object ready for Set.
func NewObject() *Object {
	return &Object{vals: map[string]json.RawMessage{}}
}

// Get returns the raw value for key and whether it is present.
func (o *Object) Get(key string) (json.RawMessage, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Has reports whether key is present.
func (o *Object) Has(key string) bool {
	_, ok := o.vals[key]
	return ok
}

// SetRaw stores a raw JSON value, preserving an existing key's position or
// appending a new key at the end — Python dict-assignment semantics (GOAL.md
// §11 key-position rule).
func (o *Object) SetRaw(key string, raw json.RawMessage) {
	if o.vals == nil {
		o.vals = map[string]json.RawMessage{}
	}
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = raw
}

// Set marshals v (HTML escaping OFF) and stores it. v may be a string, number,
// bool, nil, *Object, or any json.Marshaler (e.g. a pyFloat carried through).
func (o *Object) Set(key string, v any) error {
	raw, err := rawJSON(v)
	if err != nil {
		return err
	}
	o.SetRaw(key, raw)
	return nil
}

// Keys returns the ordered key slice (do not mutate).
func (o *Object) Keys() []string { return o.keys }

// UnmarshalJSON decodes a JSON object via a token stream, capturing each value
// as raw bytes in source order.
func (o *Object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("dailylog: expected JSON object, got %v", tok)
	}
	o.keys = nil
	o.vals = map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("dailylog: non-string object key %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		o.SetRaw(key, raw)
	}
	if _, err := dec.Token(); err != nil { // consume closing '}'
		return err
	}
	return nil
}

// MarshalJSON emits the object compactly in key order. The enclosing Encoder
// (SetEscapeHTML(false) + SetIndent) re-indents the whole document, so this
// returns compact JSON and lets json.Indent normalize spacing to match Python.
func (o *Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k) // keys here never contain HTML chars.
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		v := o.vals[k]
		if len(v) == 0 {
			v = json.RawMessage("null")
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// rawJSON compactly marshals v with HTML escaping OFF so characters like the
// zone2 label's '<' and '>' survive byte-for-byte (GOAL.md §11). The trailing
// newline the Encoder appends is trimmed.
func rawJSON(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}
