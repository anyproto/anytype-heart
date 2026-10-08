package full

// derive.go — from the OpenAPI document to the tool table. One tool per
// operation; the operation's parameters and its JSON body's members become
// one flat argument set; component schemas the body references move under
// the tool's $defs; a body with alternatives (a shortcut or a document, a
// flat patch or an ops envelope) keeps them as root-level anyOf constraints
// over the same flat arguments, each branch widened to admit the parameter
// arguments so its own additionalProperties:false still holds.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// The document shapes this file reads. Anything else in the document is
// ignored; a shape this file does not expect is a derivation error, never
// a silently different tool.
type openAPIDoc struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

type openAPIOperation struct {
	OperationId string             `json:"operationId"`
	Summary     string             `json:"summary"`
	Description string             `json:"description"`
	Parameters  []openAPIParameter `json:"parameters"`
	RequestBody *struct {
		Content map[string]struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
}

type openAPIParameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Required    bool            `json:"required"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

const (
	componentRefPrefix = "#/components/schemas/"
	defsRefPrefix      = "#/$defs/"
	contentJSON        = "application/json"
)

// methodRank orders a path's methods the way a reader expects them.
var methodRank = map[string]int{"get": 0, "post": 1, "patch": 2, "put": 3, "delete": 4, "head": 5}

// Derive builds the table.
func Derive(in Inputs) (*Table, error) {
	var doc openAPIDoc
	if err := json.Unmarshal(in.OpenAPI, &doc); err != nil {
		return nil, fmt.Errorf("decode openapi document: %w", err)
	}
	if len(doc.Paths) == 0 {
		return nil, fmt.Errorf("openapi document has no paths")
	}
	type located struct {
		path, method string
		op           openAPIOperation
	}
	var ops []located
	seen := map[string]string{}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			if op.OperationId == "" {
				return nil, fmt.Errorf("%s %s has no operationId", strings.ToUpper(method), path)
			}
			if prev, dup := seen[op.OperationId]; dup {
				return nil, fmt.Errorf("operationId %q appears twice (%s and %s %s)", op.OperationId, prev, strings.ToUpper(method), path)
			}
			seen[op.OperationId] = strings.ToUpper(method) + " " + path
			ops = append(ops, located{path: path, method: method, op: op})
		}
	}
	sort.Slice(ops, func(i, j int) bool {
		if ops[i].path != ops[j].path {
			return ops[i].path < ops[j].path
		}
		return methodRank[ops[i].method] < methodRank[ops[j].method]
	})
	for id := range overlays {
		if _, ok := seen[id]; !ok {
			return nil, fmt.Errorf("overlay names operation %q, which the document does not have", id)
		}
	}

	table := &Table{Excluded: map[string]string{}}
	for _, l := range ops {
		o := overlays[l.op.OperationId]
		if o.Exclude != "" {
			table.Excluded[l.op.OperationId] = o.Exclude
			continue
		}
		tool, err := deriveTool(l.method, l.path, l.op, o, doc.Components.Schemas, in.Ops)
		if err != nil {
			return nil, fmt.Errorf("derive %s: %w", l.op.OperationId, err)
		}
		table.Tools = append(table.Tools, tool)
	}
	table.index()
	return table, nil
}

// deriveTool builds one tool.
func deriveTool(method, path string, op openAPIOperation, o overlay, components map[string]json.RawMessage, served map[string]OpSchema) (Tool, error) {
	tool := Tool{
		Name:        op.OperationId,
		Method:      strings.ToUpper(method),
		Path:        path,
		Description: describe(op, o),
		ReadOnly:    method == "get" || o.ReadOnly,
		Destructive: method == "delete" || o.Destructive,
	}
	root := map[string]any{"type": "object"}
	props := map[string]any{}
	var required []string
	defs := map[string]any{}
	pending := map[string]opDef{}
	refs := newRefCollector(components)

	// parameters
	for _, p := range op.Parameters {
		arg, err := parameterArg(p)
		if err != nil {
			return Tool{}, err
		}
		schema, err := refs.schema(p.Schema)
		if err != nil {
			return Tool{}, fmt.Errorf("parameter %s: %w", p.Name, err)
		}
		if p.Description != "" {
			schema["description"] = p.Description
		}
		if _, dup := props[arg.Name]; dup {
			return Tool{}, fmt.Errorf("argument %q is declared twice", arg.Name)
		}
		props[arg.Name] = schema
		if arg.Required {
			required = append(required, arg.Name)
		}
		tool.Args = append(tool.Args, arg)
	}
	paramNames := make([]string, 0, len(tool.Args))
	for _, a := range tool.Args {
		paramNames = append(paramNames, a.Name)
	}

	// body
	strict := true
	if op.RequestBody != nil {
		content, ok := op.RequestBody.Content[contentJSON]
		if ok {
			tool.HasBody = true
			body, err := refs.schema(content.Schema)
			if err != nil {
				return Tool{}, fmt.Errorf("request body: %w", err)
			}
			if o.OpsChannel != "" {
				if err := typeOpsEnvelope(body, o.OpsChannel, served, defs, pending); err != nil {
					return Tool{}, fmt.Errorf("ops envelope: %w", err)
				}
			}
			open, err := hoistBody(body, root, props, &required, &tool.Args, paramNames)
			if err != nil {
				return Tool{}, fmt.Errorf("request body: %w", err)
			}
			tool.OpenBody = open
			strict = !open
			if desc, _ := body["description"].(string); desc != "" && len(bodyProperties(body)) == 0 {
				// a document body: its shape is named, not listed
				tool.Description += " Body: " + desc + "."
			}
		} else if len(op.RequestBody.Content) > 0 {
			return Tool{}, fmt.Errorf("request body has no %s form", contentJSON)
		}
	}

	// overlay edits
	for _, name := range o.DropArg {
		if _, ok := props[name]; !ok {
			return Tool{}, fmt.Errorf("overlay drops argument %q, which the tool does not have", name)
		}
		delete(props, name)
		required = remove(required, name)
		tool.Args = removeArg(tool.Args, name)
	}
	for from, to := range o.RenameArg {
		schema, ok := props[from]
		if !ok {
			return Tool{}, fmt.Errorf("overlay renames argument %q, which the tool does not have", from)
		}
		if _, taken := props[to]; taken {
			return Tool{}, fmt.Errorf("overlay renames %q to %q, which the tool already has", from, to)
		}
		delete(props, from)
		props[to] = schema
		for i := range required {
			if required[i] == from {
				required[i] = to
			}
		}
		for i := range tool.Args {
			if tool.Args[i].Name == from {
				tool.Args[i].Name = to
			}
		}
	}

	// assemble
	for name, raw := range refs.collected() {
		if _, taken := defs[name]; taken {
			return Tool{}, fmt.Errorf("$defs name %q is both a component and an op definition", name)
		}
		defs[name] = raw
	}
	if len(defs) > 0 {
		if err := dedupeOpDefs(root, defs, pending); err != nil {
			return Tool{}, err
		}
		root["$defs"] = defs
	}
	root["properties"] = props
	if len(required) > 0 {
		sort.Strings(required)
		root["required"] = required
	}
	if strict {
		root["additionalProperties"] = false
	}
	schema, err := marshalDefsLast(root)
	if err != nil {
		return Tool{}, fmt.Errorf("encode input schema: %w", err)
	}
	tool.InputSchema = schema
	return tool, nil
}

// describe renders the tool description: the overlay's, else the
// operation's summary and description.
func describe(op openAPIOperation, o overlay) string {
	if o.Description != "" {
		return o.Description
	}
	summary := strings.TrimSpace(op.Summary)
	desc := strings.TrimSpace(op.Description)
	switch {
	case summary == "":
		return desc
	case desc == "":
		return summary
	}
	if !strings.HasSuffix(summary, ".") {
		summary += "."
	}
	return summary + " " + desc
}

// parameterArg maps one document parameter to an argument. Header names
// are spelled snake_case; path and query names are kept.
func parameterArg(p openAPIParameter) (Arg, error) {
	switch p.In {
	case "path":
		return Arg{Name: p.Name, In: ArgPath, Wire: p.Name, Required: true}, nil
	case "query":
		return Arg{Name: p.Name, In: ArgQuery, Wire: p.Name, Required: p.Required}, nil
	case "header":
		return Arg{Name: snakeCase(p.Name), In: ArgHeader, Wire: p.Name, Required: p.Required}, nil
	default:
		return Arg{}, fmt.Errorf("parameter %s is in %q, which this table does not place", p.Name, p.In)
	}
}

// snakeCase spells an HTTP header name as an argument: Idempotency-Key →
// idempotency_key.
func snakeCase(header string) string {
	return strings.ToLower(strings.ReplaceAll(header, "-", "_"))
}

// hoistBody lifts a body schema onto the tool root: its members become
// arguments beside the parameters. Returns whether the body is open —
// takes members beyond the listed ones.
//
// A body with alternatives keeps them as root-level anyOf constraints over
// the same flat arguments. The member schemas live once, on the root; each
// branch keeps only what makes it an alternative — its membership
// (properties reduced to markers, widened with the parameter arguments so
// its own additionalProperties:false still decides which members belong
// together: a flat type patch may not carry ops, an envelope may not carry
// name), its required list and its description. A member two branches both
// declare takes the first branch's schema on the root.
func hoistBody(body, root map[string]any, props map[string]any, required *[]string, args *[]Arg, paramNames []string) (bool, error) {
	if branches, ok := body["anyOf"].([]any); ok {
		open := false
		reduced := make([]any, 0, len(branches))
		for _, raw := range branches {
			branch, ok := raw.(map[string]any)
			if !ok {
				return false, fmt.Errorf("anyOf branch is not an object schema")
			}
			if err := addBodyMembers(branch, props, nil, args); err != nil {
				return false, err
			}
			if isOpenObject(branch) {
				open = true
			}
			markers := map[string]any{}
			for name := range bodyProperties(branch) {
				markers[name] = map[string]any{}
			}
			for _, name := range paramNames {
				if _, collides := markers[name]; collides {
					return false, fmt.Errorf("body member %q collides with a parameter of the same name — rename one in the overlay (RenameArg)", name)
				}
				markers[name] = map[string]any{}
			}
			constraint := map[string]any{"properties": markers}
			for _, key := range []string{"required", "additionalProperties", "description", "allOf", "minProperties"} {
				if v, ok := branch[key]; ok {
					constraint[key] = v
				}
			}
			reduced = append(reduced, constraint)
		}
		root["anyOf"] = reduced
		if desc, _ := body["description"].(string); desc != "" {
			root["description"] = desc
		}
		return open, nil
	}
	if err := addBodyMembers(body, props, required, args); err != nil {
		return false, err
	}
	for _, key := range []string{"allOf", "oneOf", "not", "minProperties", "maxProperties"} {
		if v, ok := body[key]; ok {
			root[key] = v
		}
	}
	return isOpenObject(body), nil
}

// addBodyMembers adds a body object's properties as arguments. When
// requiredOut is nil the branch's required list stays on the branch (an
// alternative's requirement is not the tool's).
func addBodyMembers(body map[string]any, props map[string]any, requiredOut *[]string, args *[]Arg) error {
	bodyProps := bodyProperties(body)
	bodyRequired := map[string]bool{}
	if list, ok := body["required"].([]any); ok {
		for _, r := range list {
			if s, ok := r.(string); ok {
				bodyRequired[s] = true
			}
		}
	}
	for _, name := range sortedKeys(bodyProps) {
		if _, dup := props[name]; dup {
			if a, isArg := findArg(*args, name); isArg && a.In != ArgBody {
				return fmt.Errorf("body member %q collides with a parameter of the same name — rename one in the overlay (RenameArg)", name)
			}
			// the same member on two branches: keep the first branch's
			// schema, the branch constraint carries the difference
			continue
		}
		props[name] = bodyProps[name]
		*args = append(*args, Arg{Name: name, In: ArgBody, Wire: name, Required: requiredOut != nil && bodyRequired[name]})
		if requiredOut != nil && bodyRequired[name] {
			*requiredOut = append(*requiredOut, name)
		}
	}
	return nil
}

// bodyProperties returns a body schema's properties map, or nil.
func bodyProperties(body map[string]any) map[string]any {
	props, _ := body["properties"].(map[string]any)
	return props
}

// isOpenObject reports whether an object schema admits members beyond its
// properties. A schema that lists properties and says nothing about
// additional ones is closed: the document's generated request structs are
// decoded strictly by the server, and a body that lists no properties at
// all is a document form, open by design.
func isOpenObject(schema map[string]any) bool {
	ap, present := schema["additionalProperties"]
	if !present {
		return len(bodyProperties(schema)) == 0
	}
	if b, ok := ap.(bool); ok {
		return b
	}
	return true
}

func findArg(args []Arg, name string) (Arg, bool) {
	for _, a := range args {
		if a.Name == name {
			return a, true
		}
	}
	return Arg{}, false
}

func remove(list []string, name string) []string {
	out := list[:0]
	for _, s := range list {
		if s != name {
			out = append(out, s)
		}
	}
	return out
}

func removeArg(args []Arg, name string) []Arg {
	out := args[:0]
	for _, a := range args {
		if a.Name != name {
			out = append(out, a)
		}
	}
	return out
}

//
// ---- component references ----
//

// refCollector resolves a schema's top-level component reference and
// rewrites nested ones to $defs, collecting the referenced components
// (transitively) for the tool's $defs.
type refCollector struct {
	components map[string]json.RawMessage
	needed     map[string]bool
}

func newRefCollector(components map[string]json.RawMessage) *refCollector {
	return &refCollector{components: components, needed: map[string]bool{}}
}

// schema decodes raw into a map, resolving a top-level $ref to its
// component and rewriting nested component refs to $defs.
func (c *refCollector) schema(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	m, ok := node.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema is not an object")
	}
	if ref, ok := m["$ref"].(string); ok && strings.HasPrefix(ref, componentRefPrefix) {
		name := strings.TrimPrefix(ref, componentRefPrefix)
		component, ok := c.components[name]
		if !ok {
			return nil, fmt.Errorf("unknown component %q", name)
		}
		resolved, err := c.schema(component)
		if err != nil {
			return nil, fmt.Errorf("component %s: %w", name, err)
		}
		// a sibling description on the ref wins over the component's
		if desc, ok := m["description"].(string); ok && desc != "" {
			resolved["description"] = desc
		}
		return resolved, nil
	}
	if err := c.rewrite(m); err != nil {
		return nil, err
	}
	return m, nil
}

// rewrite re-aims component refs under node at $defs and records them.
func (c *refCollector) rewrite(node any) error {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && strings.HasPrefix(ref, componentRefPrefix) {
			name := strings.TrimPrefix(ref, componentRefPrefix)
			if _, ok := c.components[name]; !ok {
				return fmt.Errorf("unknown component %q", name)
			}
			n["$ref"] = defsRefPrefix + name
			c.needed[name] = true
		}
		for _, child := range n {
			if err := c.rewrite(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range n {
			if err := c.rewrite(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// collected returns every referenced component, transitively, rewritten
// the same way.
func (c *refCollector) collected() map[string]any {
	out := map[string]any{}
	for {
		progressed := false
		for name := range c.needed {
			if _, done := out[name]; done {
				continue
			}
			var node any
			if err := json.Unmarshal(c.components[name], &node); err != nil {
				// the document is valid JSON by the time it is embedded;
				// an unreadable component is a test-time failure elsewhere
				out[name] = map[string]any{}
				continue
			}
			_ = c.rewrite(node)
			out[name] = node
			progressed = true
		}
		if !progressed {
			return out
		}
	}
}

// marshalDefsLast serializes a schema with its $defs member last, so a
// reader meets the arguments before the shared shapes (the same order the
// discovery surface uses).
func marshalDefsLast(root map[string]any) ([]byte, error) {
	defs, ok := root["$defs"]
	if !ok {
		return json.Marshal(root)
	}
	delete(root, "$defs")
	defer func() { root["$defs"] = defs }()
	body, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	tail, err := json.Marshal(defs)
	if err != nil {
		return nil, err
	}
	if string(body) == "{}" {
		return append([]byte(`{"$defs":`), append(tail, '}')...), nil
	}
	out := make([]byte, 0, len(body)+len(tail)+12)
	out = append(out, body[:len(body)-1]...)
	out = append(out, `,"$defs":`...)
	out = append(out, tail...)
	out = append(out, '}')
	return out, nil
}
