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
	"regexp"
	"sort"
	"strings"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
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
		tool, err := deriveTool(l.method, l.path, l.op, o, doc.Components.Schemas)
		if err != nil {
			return nil, fmt.Errorf("derive %s: %w", l.op.OperationId, err)
		}
		table.Tools = append(table.Tools, tool)
	}
	table.index()
	return table, nil
}

// deriveTool builds one tool.
func deriveTool(method, path string, op openAPIOperation, o overlay, components map[string]json.RawMessage) (Tool, error) {
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
	refs := newRefCollector(components)

	// parameters
	for _, p := range op.Parameters {
		arg, err := parameterArg(p)
		if err != nil {
			return Tool{}, err
		}
		if arg.Wire == wrapper.IdempotencyKeyHeader && !o.CallerRetryKey {
			// the executor mints the retry key itself; only a tool the
			// overlay names lets the caller choose it
			continue
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
	// The executor mints a retry key for every write and reuses it across
	// its own transport retries, so a model never needs one. The overlay
	// names the tools where the caller must be able to choose it anyway:
	// a call whose blind replay is not harmless (a toggle flips back). It
	// is a header, never a body member.
	if o.CallerRetryKey {
		if method == "get" || method == "head" {
			return Tool{}, fmt.Errorf("overlay gives a caller retry key to a read")
		}
		if _, ok := tool.Arg(idempotencyKeyArg); !ok {
			tool.Args = append(tool.Args, Arg{Name: idempotencyKeyArg, In: ArgHeader, Wire: wrapper.IdempotencyKeyHeader})
		}
		props[idempotencyKeyArg] = idempotencyKeySchema()
	}
	paramNames := make([]string, 0, len(tool.Args))
	for _, a := range tool.Args {
		paramNames = append(paramNames, a.Name)
	}

	// body
	strict := true
	renamesUsed := map[string]bool{}
	if op.RequestBody != nil {
		content, ok := op.RequestBody.Content[contentJSON]
		if ok {
			tool.HasBody = true
			body, err := refs.schema(content.Schema)
			if err != nil {
				return Tool{}, fmt.Errorf("request body: %w", err)
			}
			openOpsItems(body)
			h := hoister{root: root, props: props, required: &required, args: &tool.Args, paramNames: paramNames, renames: o.RenameBodyMember, used: renamesUsed}
			open, err := h.hoist(body)
			if err != nil {
				return Tool{}, fmt.Errorf("request body: %w", err)
			}
			tool.OpenBody = open
			strict = !open
			if desc, _ := body["description"].(string); desc != "" && len(bodyProperties(body)) == 0 && body["anyOf"] == nil {
				// a document body: its shape is named, not listed
				tool.Description += " Body: " + desc + "."
			} else if kinds := documentKinds(body); len(kinds) > 0 {
				tool.Description += " The document form's schema and a worked example come from get_schema with kind " + strings.Join(kinds, " or ") + "; the other forms need no lookup."
			}
			if names := opNames(body); len(names) > 0 {
				tool.Description += " Ops: " + strings.Join(names, ", ") + ". Their members are not in this schema: get_op_schema with an op's name serves them and a worked example, so call it before first using an op."
			}
		} else if len(op.RequestBody.Content) > 0 {
			return Tool{}, fmt.Errorf("request body has no %s form", contentJSON)
		}
	}
	for from := range o.RenameBodyMember {
		if !renamesUsed[from] {
			return Tool{}, fmt.Errorf("overlay renames body member %q, which the body does not have", from)
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

	// assemble
	collected, err := refs.collected()
	if err != nil {
		return Tool{}, err
	}
	for name, raw := range collected {
		if _, taken := defs[name]; taken {
			return Tool{}, fmt.Errorf("$defs name %q is both a component and an op definition", name)
		}
		defs[name] = raw
	}
	root["properties"] = props
	if len(defs) > 0 {
		root["$defs"] = defs
	}
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

// openOpsItems makes an ops envelope's items explicitly open. The document
// types an op by its name alone — the op enum — and points at
// get_op_schema for its members, so an item takes members beyond op; the
// schema says so instead of leaving it to a host's default, the way the
// npm bridge serves it.
func openOpsItems(body map[string]any) {
	for _, holder := range bodyBranches(body) {
		props, _ := holder["properties"].(map[string]any)
		ops, _ := props["ops"].(map[string]any)
		items, _ := ops["items"].(map[string]any)
		if items == nil {
			continue
		}
		if _, set := items["additionalProperties"]; !set {
			items["additionalProperties"] = true
		}
	}
}

// opNames is an ops envelope's op enum, in the document's order, for the
// description: the schema types an item by its op name alone, and a model
// reads the description before it reads a nested item schema.
func opNames(body map[string]any) []string {
	for _, holder := range bodyBranches(body) {
		props, _ := holder["properties"].(map[string]any)
		ops, _ := props["ops"].(map[string]any)
		items, _ := ops["items"].(map[string]any)
		itemProps, _ := items["properties"].(map[string]any)
		op, _ := itemProps["op"].(map[string]any)
		enum, _ := op["enum"].([]any)
		var names []string
		for _, v := range enum {
			if name, ok := v.(string); ok {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	return nil
}

// documentKindPointer is how a body branch names its get_schema lookup.
var documentKindPointer = regexp.MustCompile(`get_schema with kind (\w+)`)

// documentKinds is the get_schema kinds the body's document branches point
// at, lifted into the description for the same reason as opNames.
func documentKinds(body map[string]any) []string {
	var kinds []string
	seen := map[string]bool{}
	for _, holder := range bodyBranches(body)[1:] {
		desc, _ := holder["description"].(string)
		for _, m := range documentKindPointer.FindAllStringSubmatch(desc, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				kinds = append(kinds, m[1])
			}
		}
	}
	return kinds
}

// bodyBranches is the body followed by its anyOf branches.
func bodyBranches(body map[string]any) []map[string]any {
	holders := []map[string]any{body}
	if branches, ok := body["anyOf"].([]any); ok {
		for _, b := range branches {
			if m, ok := b.(map[string]any); ok {
				holders = append(holders, m)
			}
		}
	}
	return holders
}

// idempotencyKeyArg is the caller's retry-key argument, on the tools the
// overlay names (CallerRetryKey).
const idempotencyKeyArg = "idempotency_key"

// idempotencyKeySchema is the retry key's argument schema, bounded the way
// the server bounds the header.
func idempotencyKeySchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"minLength":   1,
		"maxLength":   wrapper.MaxIdempotencyKeyLen,
		"pattern":     "^[!-~]+$",
		"description": "retry key: if this call fails or times out, send it again with the same value so it is not applied twice; one is made for you when omitted",
	}
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

// legalArgName is the argument-name shape every tool host accepts (the
// Claude API's input_schema property keys); a body member spelled outside
// it (a document's "$schema") is not offered as an argument.
var legalArgName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

// hoister lifts a body schema onto the tool root: its members become
// arguments beside the parameters.
type hoister struct {
	root       map[string]any
	props      map[string]any
	required   *[]string
	args       *[]Arg
	paramNames []string
	// renames maps a body member (its wire name) to its argument name; used
	// records which ones applied.
	renames map[string]string
	used    map[string]bool
}

// member is one body member as an argument.
type member struct {
	name, wire string
	schema     any
	required   bool
}

// members lists a body object's members under their argument names,
// refusing one that collides with a parameter.
func (h hoister) members(body map[string]any) ([]member, error) {
	bodyRequired := map[string]bool{}
	if list, ok := body["required"].([]any); ok {
		for _, r := range list {
			if s, ok := r.(string); ok {
				bodyRequired[s] = true
			}
		}
	}
	props := bodyProperties(body)
	out := make([]member, 0, len(props))
	for _, wire := range sortedKeys(props) {
		name := wire
		if to, ok := h.renames[wire]; ok {
			name = to
			h.used[wire] = true
		}
		if !legalArgName.MatchString(name) {
			if bodyRequired[wire] {
				return nil, fmt.Errorf("required body member %q is not a legal argument name — rename it in the overlay (RenameBodyMember)", wire)
			}
			continue
		}
		if a, isArg := findArg(*h.args, name); isArg && a.In != ArgBody {
			return nil, fmt.Errorf("body member %q collides with a parameter of the same name — rename it in the overlay (RenameBodyMember)", wire)
		}
		out = append(out, member{name: name, wire: wire, schema: props[wire], required: bodyRequired[wire]})
	}
	return out, nil
}

// addArg records a body member as an argument once.
func (h hoister) addArg(m member, required bool) {
	if _, ok := findArg(*h.args, m.name); ok {
		return
	}
	*h.args = append(*h.args, Arg{Name: m.name, In: ArgBody, Wire: m.wire, Required: required})
}

// crossMember copies the constraints that name members (allOf, oneOf,
// not) from a body to target. Renames would leave them naming wire names,
// so a body that both renames and carries them is refused.
func (h hoister) crossMember(body, target map[string]any) error {
	for _, key := range []string{"allOf", "oneOf", "not"} {
		v, ok := body[key]
		if !ok {
			continue
		}
		if len(h.renames) > 0 {
			return fmt.Errorf("a body with %s cannot have its members renamed", key)
		}
		if key == "allOf" {
			existing, _ := target["allOf"].([]any)
			list, _ := v.([]any)
			target["allOf"] = append(existing, list...)
		} else {
			target[key] = v
		}
	}
	return nil
}

// atLeastOne turns a body's minProperties into a constraint on the
// flattened arguments: the parameters are members of the flattened object
// too, so a minProperties there would be met by a path argument alone.
func atLeastOne(body map[string]any, ms []member, target map[string]any) error {
	if _, ok := body["maxProperties"]; ok {
		return fmt.Errorf("maxProperties on a body has no flattened equivalent")
	}
	v, ok := body["minProperties"]
	if !ok {
		return nil
	}
	if n, _ := v.(float64); n != 1 {
		return fmt.Errorf("minProperties %v on a body has no flattened equivalent; only 1 is supported", v)
	}
	anyOf := make([]any, 0, len(ms))
	for _, m := range ms {
		anyOf = append(anyOf, map[string]any{"required": []string{m.name}})
	}
	existing, _ := target["allOf"].([]any)
	target["allOf"] = append(existing, map[string]any{"anyOf": anyOf})
	return nil
}

// hoist lifts body onto the root and reports whether it is open — takes
// members beyond the listed ones.
//
// A body with alternatives keeps them as root-level anyOf constraints over
// the same flat arguments. A member every branch spells the same way lives
// once, on the root, and the branch keeps a marker; a member two branches
// spell differently (a shortcut's properties map against a document's)
// gets an unconstrained root entry and its real schema in each branch.
// Each branch keeps its own required list and additionalProperties, widened
// with the parameter names, so it still decides which members belong
// together: a flat type patch may not carry ops, an envelope may not carry
// name.
func (h hoister) hoist(body map[string]any) (bool, error) {
	branches, isAnyOf := body["anyOf"].([]any)
	if !isAnyOf {
		ms, err := h.members(body)
		if err != nil {
			return false, err
		}
		for _, m := range ms {
			h.props[m.name] = m.schema
			h.addArg(m, m.required)
			if m.required {
				*h.required = append(*h.required, m.name)
			}
		}
		if err := h.crossMember(body, h.root); err != nil {
			return false, err
		}
		if err := atLeastOne(body, ms, h.root); err != nil {
			return false, err
		}
		return isOpenObject(body), nil
	}

	perBranch := make([][]member, len(branches))
	spellings := map[string]map[string]bool{}
	for i, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok {
			return false, fmt.Errorf("anyOf branch is not an object schema")
		}
		ms, err := h.members(branch)
		if err != nil {
			return false, err
		}
		perBranch[i] = ms
		for _, m := range ms {
			canonical, err := json.Marshal(m.schema)
			if err != nil {
				return false, fmt.Errorf("encode member %s: %w", m.name, err)
			}
			if spellings[m.name] == nil {
				spellings[m.name] = map[string]bool{}
			}
			spellings[m.name][string(canonical)] = true
		}
	}
	open := false
	reduced := make([]any, 0, len(branches))
	for i, raw := range branches {
		branch := raw.(map[string]any)
		if isOpenObject(branch) {
			open = true
		}
		markers := map[string]any{}
		var branchRequired []string
		for _, m := range perBranch[i] {
			if len(spellings[m.name]) > 1 {
				markers[m.name] = m.schema
				if _, ok := h.props[m.name]; !ok {
					unconstrained := map[string]any{}
					if sm, ok := m.schema.(map[string]any); ok {
						if d, ok := sm["description"]; ok {
							unconstrained["description"] = d
						}
					}
					h.props[m.name] = unconstrained
				}
			} else {
				markers[m.name] = map[string]any{}
				if _, ok := h.props[m.name]; !ok {
					h.props[m.name] = m.schema
				}
			}
			h.addArg(m, false)
			if m.required {
				branchRequired = append(branchRequired, m.name)
			}
		}
		for _, name := range h.paramNames {
			markers[name] = map[string]any{}
		}
		constraint := map[string]any{"properties": markers}
		if len(branchRequired) > 0 {
			constraint["required"] = branchRequired
		}
		for _, key := range []string{"additionalProperties", "description"} {
			if v, ok := branch[key]; ok {
				constraint[key] = v
			}
		}
		if err := h.crossMember(branch, constraint); err != nil {
			return false, err
		}
		if err := atLeastOne(branch, perBranch[i], constraint); err != nil {
			return false, err
		}
		reduced = append(reduced, constraint)
	}
	h.root["anyOf"] = reduced
	if desc, _ := body["description"].(string); desc != "" {
		h.root["description"] = desc
	}
	return open, nil
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
// the same way. A component that does not decode, or that references one
// the document lacks, fails the derivation — a dangling $ref in a served
// tool schema is a host-side failure no test of the tool would see.
func (c *refCollector) collected() (map[string]any, error) {
	out := map[string]any{}
	for {
		progressed := false
		for _, name := range sortedKeys(c.needed) {
			if _, done := out[name]; done {
				continue
			}
			var node any
			if err := json.Unmarshal(c.components[name], &node); err != nil {
				return nil, fmt.Errorf("decode component %s: %w", name, err)
			}
			if err := c.rewrite(node); err != nil {
				return nil, fmt.Errorf("component %s: %w", name, err)
			}
			out[name] = node
			progressed = true
		}
		if !progressed {
			return out, nil
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
