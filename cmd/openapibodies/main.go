// Command openapibodies prints the request-body schemas the v2 OpenAPI
// document publishes for the operations swag cannot describe, as JSON and as
// the YAML fragment scripts/fix_openapi_v2.py splices into openapi.yaml.
// `make openapi` runs it; nothing at runtime does.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

type fragment struct {
	JSON json.RawMessage `json:"json"`
	YAML string          `json:"yaml"`
}

func main() {
	composed, err := v2service.ComposeOpenAPIBodies()
	if err != nil {
		fmt.Fprintln(os.Stderr, "compose openapi bodies:", err)
		os.Exit(1)
	}
	out := struct {
		Bodies     map[string]fragment `json:"bodies"`
		Components map[string]fragment `json:"components"`
	}{Bodies: map[string]fragment{}, Components: map[string]fragment{}}
	for op, body := range composed.Bodies {
		out.Bodies[op], err = render(body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render %s: %v\n", op, err)
			os.Exit(1)
		}
	}
	for name, schema := range composed.Components {
		out.Components[name], err = render(schema)
		if err != nil {
			fmt.Fprintf(os.Stderr, "render component %s: %v\n", name, err)
			os.Exit(1)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
}

// render pairs a schema with its YAML spelling: two-space indent and sorted
// keys, the style swag writes, so the spliced fragment reads like the rest
// of the document.
func render(schema json.RawMessage) (fragment, error) {
	dec := json.NewDecoder(bytes.NewReader(schema))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return fragment{}, fmt.Errorf("decode: %w", err)
	}
	value = plainNumbers(value)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(value); err != nil {
		return fragment{}, fmt.Errorf("encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fragment{}, fmt.Errorf("close yaml: %w", err)
	}
	return fragment{JSON: schema, YAML: buf.String()}, nil
}

// plainNumbers turns json.Number into int64 where it is integral and float64
// otherwise, so YAML writes 1048576 rather than a quoted string or 1.05e+06.
func plainNumbers(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			v[k] = plainNumbers(child)
		}
		return v
	case []any:
		for i, child := range v {
			v[i] = plainNumbers(child)
		}
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		f, _ := v.Float64()
		return f
	}
	return value
}
