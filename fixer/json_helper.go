package fixer

import "encoding/json"

// jsonMarshal wraps json.Marshal for use in fixer functions.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal wraps json.Unmarshal for use in fixer functions.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
