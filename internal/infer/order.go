package infer

import (
	"encoding/json"
	"errors"
)

// ordObj is a JSON object that remembers the order its keys appeared in.
type ordObj struct {
	keys []string
	vals map[string]any
}

// decodeOrdered reads one JSON value like json.Decoder.Decode into an any,
// except objects come back as *ordObj so key order survives.
func decodeOrdered(dec *json.Decoder) (any, error) {
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return decodeFrom(dec, t)
}

func decodeFrom(dec *json.Decoder, t json.Token) (any, error) {
	d, ok := t.(json.Delim)
	if !ok {
		return t, nil // string, json.Number, bool or nil
	}
	switch d {
	case '{':
		o := &ordObj{vals: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := kt.(string)
			vt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeFrom(dec, vt)
			if err != nil {
				return nil, err
			}
			if _, dup := o.vals[key]; !dup {
				o.keys = append(o.keys, key)
			}
			o.vals[key] = v
		}
		if _, err := dec.Token(); err != nil { // closing }
			return nil, err
		}
		return o, nil
	case '[':
		arr := []any{}
		for dec.More() {
			vt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeFrom(dec, vt)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.Token(); err != nil { // closing ]
			return nil, err
		}
		return arr, nil
	}
	return nil, errors.New("infer: unexpected delimiter")
}
