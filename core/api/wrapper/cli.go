package wrapper

// cli.go — small helpers the CLI delivery shares with tests, kept here so
// cmd/anytype stays a flag-parsing shell over the one tool table.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseObjectFlag decodes an object-shaped CLI flag value (JSON) into the
// args map shape.
func ParseObjectFlag(value string) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		hint := `expected a JSON object, e.g. '{"Status":"Done"}'`
		if !strings.HasPrefix(strings.TrimSpace(value), "{") {
			return nil, fmt.Errorf("%s — got %q", hint, value)
		}
		return nil, fmt.Errorf("%s: %w", hint, err)
	}
	return m, nil
}

// EncodeJSON renders a tool result's machine shape as compact JSON (C3).
func EncodeJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return data, nil
}
