package full

// ops.go — the typed ops envelope (spec §2.2). The document's envelope
// publishes the op vocabulary and leaves each op's members to its own served
// schema; here those schemas become the envelope's branches, so a caller
// sees every op's shape in the tool instead of behind a lookup.
//
// Each served op schema carries its own $defs, and the same definition name
// means different shapes on different ops (insert_blocks and
// replace_subtree each define `block`). The definitions are therefore
// namespaced per op on embedding, and those that turn out structurally
// identical across ops are merged back under one name — the measured
// difference is a quarter of the table.

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// opDefSeparator joins an op name and a definition name into a namespaced
// $defs key.
const opDefSeparator = "__"

// opBranchPrefix names an op's own definition under $defs.
const opBranchPrefix = "op" + opDefSeparator

// typeOpsEnvelope replaces the `ops.items` schema of body (at its root or in
// exactly one of its anyOf branches) with a oneOf over the channel's served
// op schemas, adding their definitions to defs under namespaced keys.
// opDef names one namespaced definition awaiting deduplication.
type opDef struct {
	op, base string
}

func typeOpsEnvelope(body map[string]any, channel string, served map[string]OpSchema, defs map[string]any, pending map[string]opDef) error {
	ops := channelOps(channel, served)
	if len(ops) == 0 {
		return fmt.Errorf("no served op schema belongs to channel %q", channel)
	}
	targets := envelopeTargets(body)
	if len(targets) != 1 {
		return fmt.Errorf("expected exactly one ops envelope in the body, found %d", len(targets))
	}
	envelope := targets[0]
	branches := make([]any, 0, len(ops))
	for _, op := range ops {
		schema, err := embedServed(op, served[op].Schema, defs, pending)
		if err != nil {
			return fmt.Errorf("op schema %q: %w", op, err)
		}
		defs[opBranchPrefix+op] = schema
		branches = append(branches, map[string]any{"$ref": defsRefPrefix + opBranchPrefix + op})
	}
	envelope["items"] = map[string]any{"oneOf": branches}
	return nil
}

// embedServed decodes one served schema for embedding in a tool: its own
// $defs move to the tool's under names namespaced by prefix (the same
// definition name means different shapes in different served schemas), its
// references are re-aimed at them, and each is registered for
// deduplication.
func embedServed(prefix string, raw json.RawMessage, defs map[string]any, pending map[string]opDef) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("decode served schema: %w", err)
	}
	rename := func(name string) string { return prefix + opDefSeparator + name }
	if own, ok := schema["$defs"].(map[string]any); ok {
		delete(schema, "$defs")
		for name, def := range own {
			rewriteRefs(def, rename)
			defs[rename(name)] = def
			pending[rename(name)] = opDef{op: prefix, base: name}
		}
	}
	rewriteRefs(schema, rename)
	return schema, nil
}

// embedOpaqueBody replaces the open document form of body — the body
// itself, or its one anyOf branch with no listed members — with the
// served schema of kind.
func embedOpaqueBody(body map[string]any, kind string, kinds map[string]json.RawMessage, defs map[string]any, pending map[string]opDef) (map[string]any, error) {
	raw, ok := kinds[kind]
	if !ok {
		return nil, fmt.Errorf("no served schema for kind %q in the inputs", kind)
	}
	embed := func(pointer map[string]any) (map[string]any, error) {
		schema, err := embedServed(kind, raw, defs, pending)
		if err != nil {
			return nil, fmt.Errorf("kind %q: %w", kind, err)
		}
		if desc, _ := pointer["description"].(string); desc != "" {
			// the pointer named where the shape was; it is here now
			if i := strings.Index(desc, "; its schema"); i >= 0 {
				desc = desc[:i]
			}
			schema["description"] = desc
		}
		delete(schema, "$schema")
		return schema, nil
	}
	if isOpaque(body) {
		return embed(body)
	}
	branches, _ := body["anyOf"].([]any)
	found := -1
	for i, b := range branches {
		if m, ok := b.(map[string]any); ok && isOpaque(m) {
			if found >= 0 {
				return nil, fmt.Errorf("more than one open document form in the body")
			}
			found = i
		}
	}
	if found < 0 {
		return nil, fmt.Errorf("the body has no open document form for kind %q to fill", kind)
	}
	schema, err := embed(branches[found].(map[string]any))
	if err != nil {
		return nil, err
	}
	branches[found] = schema
	return body, nil
}

// isOpaque reports whether a body schema is an open object that lists no
// members: a document form named rather than described.
func isOpaque(schema map[string]any) bool {
	_, hasProps := schema["properties"]
	_, hasAnyOf := schema["anyOf"]
	return !hasProps && !hasAnyOf && isOpenObject(schema)
}

// channelOps lists the ops a channel accepts, in served order.
func channelOps(channel string, served map[string]OpSchema) []string {
	var ops []string
	for op, s := range served {
		if slices.Contains(s.Channels, channel) {
			ops = append(ops, op)
		}
	}
	sort.Strings(ops)
	return ops
}

// envelopeTargets finds every `ops` array schema at the body root or in its
// anyOf branches.
func envelopeTargets(body map[string]any) []map[string]any {
	var out []map[string]any
	candidates := []map[string]any{body}
	if branches, ok := body["anyOf"].([]any); ok {
		for _, b := range branches {
			if m, ok := b.(map[string]any); ok {
				candidates = append(candidates, m)
			}
		}
	}
	for _, c := range candidates {
		props, _ := c["properties"].(map[string]any)
		ops, _ := props["ops"].(map[string]any)
		if ops == nil {
			continue
		}
		if _, ok := ops["items"]; ok {
			out = append(out, ops)
		}
	}
	return out
}

// rewriteRefs re-aims every `#/$defs/<name>` under node at rename(name).
func rewriteRefs(node any, rename func(string) string) {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && strings.HasPrefix(ref, defsRefPrefix) {
			name := strings.TrimPrefix(ref, defsRefPrefix)
			if !strings.Contains(name, opDefSeparator) {
				n["$ref"] = defsRefPrefix + rename(name)
			}
		}
		for _, child := range n {
			rewriteRefs(child, rename)
		}
	case []any:
		for _, child := range n {
			rewriteRefs(child, rename)
		}
	}
}

// dedupeOpDefs merges namespaced op definitions that are structurally
// identical: a definition name every op agrees on goes back to its plain
// name; one that differs between ops keeps a namespaced name per distinct
// shape, spelled after the first op (in name order) that has it. It runs to
// a fixpoint: a definition that references others compares equal only once
// those have been merged and its references re-aimed, so leaves merge
// first and their parents on the next pass.
func dedupeOpDefs(root map[string]any, defs map[string]any, pending map[string]opDef) error {
	for len(pending) > 0 {
		rename, next, err := opDefRenames(defs, pending)
		if err != nil {
			return err
		}
		changed := false
		for from, to := range rename {
			if from == to {
				continue
			}
			changed = true
			if _, exists := defs[to]; !exists {
				defs[to] = defs[from]
			}
			delete(defs, from)
		}
		pending = next
		if !changed {
			return nil
		}
		renameExact := func(name string) string {
			if to, ok := rename[name]; ok {
				return to
			}
			return name
		}
		rewriteRefsExact(root, renameExact)
		for _, def := range defs {
			rewriteRefsExact(def, renameExact)
		}
	}
	return nil
}

// opDefRenames computes one pass of merges over the pending definitions:
// key → final name, and the definitions still pending afterwards (variants
// that may yet merge once what they reference has).
func opDefRenames(defs map[string]any, pending map[string]opDef) (map[string]string, map[string]opDef, error) {
	type variant struct {
		canonical string
		keys      []string
		ops       []string
	}
	byBase := map[string][]*variant{}
	for _, key := range sortedKeys(pending) {
		canonical, err := json.Marshal(defs[key])
		if err != nil {
			return nil, nil, fmt.Errorf("encode definition %s: %w", key, err)
		}
		base := pending[key].base
		var found *variant
		for _, v := range byBase[base] {
			if v.canonical == string(canonical) {
				found = v
				break
			}
		}
		if found == nil {
			found = &variant{canonical: string(canonical)}
			byBase[base] = append(byBase[base], found)
		}
		found.keys = append(found.keys, key)
		found.ops = append(found.ops, pending[key].op)
	}
	rename := map[string]string{}
	next := map[string]opDef{}
	for base, variants := range byBase {
		_, plainTaken := defs[base]
		if plainTaken {
			if _, isPending := pending[base]; isPending {
				plainTaken = false
			}
		}
		for _, v := range variants {
			sort.Strings(v.ops)
			final := base
			if len(variants) > 1 || plainTaken {
				final = base + opDefSeparator + v.ops[0]
				next[final] = opDef{op: v.ops[0], base: base}
			}
			for _, key := range v.keys {
				rename[key] = final
			}
		}
	}
	return rename, next, nil
}

// rewriteRefsExact re-aims every `#/$defs/<name>` under node at
// rename(name), namespaced or not.
func rewriteRefsExact(node any, rename func(string) string) {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && strings.HasPrefix(ref, defsRefPrefix) {
			n["$ref"] = defsRefPrefix + rename(strings.TrimPrefix(ref, defsRefPrefix))
		}
		for _, child := range n {
			rewriteRefsExact(child, rename)
		}
	case []any:
		for _, child := range n {
			rewriteRefsExact(child, rename)
		}
	}
}
