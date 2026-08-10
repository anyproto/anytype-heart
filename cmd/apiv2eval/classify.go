package main

// classify.go — the three instrumented questions, computed from an
// attempt's recorded calls and HTTP exchanges. Each is a count over what
// the model actually emitted, never a judgment the model reported about
// itself.
//
//  1. Did an insertBlocks payload carry an `id`? The schema stopped
//     publishing one (§8.30/§8.31) precisely because a decoder emits the
//     fields it is shown. replaceSubtree — which still publishes one — is
//     the control: the same model, the same run, one schema with the field
//     and one without.
//  2. When a block id was echoed back, was it the exact string the read
//     served? The default read serves 5-char labels; the write channels
//     resolve an exact id or a unique suffix, so "close" has grades.
//  3. After a refusal, did the next turn fix the field the error named, or
//     repeat itself? A refusal a model cannot act on is only half a fix.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// idEmission is one `id` a payload carried, with the path it sat at.
type idEmission struct {
	Turn  int    `json:"turn"`
	Tool  string `json:"tool"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

// refUse is one block-reference argument the model wrote.
type refUse struct {
	Turn    int    `json:"turn"`
	Tool    string `json:"tool"`
	Arg     string `json:"arg"`
	Value   string `json:"value"`
	Class   string `json:"class"`
	IsError bool   `json:"is_error"`
}

// reference-echo classes (H2).
const (
	refExact      = "exact_served"        // the string a read served, verbatim
	refStale      = "stale_served"        // served by an EARLIER read, not the latest
	refSuffix     = "suffix_of_served"    // resolvable server-side by unique suffix
	refSubstring  = "substring_of_served" // recognisable but not resolvable
	refCaseFold   = "case_variant"        // right characters, wrong case
	refInvented   = "not_served"          // no read ever served this
	refNoRead     = "no_read_yet"         // written before any read in this attempt
	refHandleLike = "handle_number"       // a find handle used where a block id belongs
)

// repair is one refusal and what the model did on its next call.
type repair struct {
	Turn       int    `json:"turn"`
	Tool       string `json:"tool"`
	Status     int    `json:"status,omitempty"`
	Code       string `json:"code,omitempty"`
	Path       string `json:"path,omitempty"`
	NamedField string `json:"named_field,omitempty"`
	NextTool   string `json:"next_tool,omitempty"`
	Class      string `json:"class"`
	NextOK     bool   `json:"next_ok,omitempty"`
}

// repair classes (H3).
const (
	repairFixedNamed  = "fixed_named_field"
	repairChangedElse = "changed_other_field"
	repairIdentical   = "identical_repeat"
	repairSwitchRead  = "switched_to_read"
	repairSwitchTool  = "switched_tool"
	repairAbandoned   = "abandoned"
)

// signals is the instrumented summary of one attempt.
type signals struct {
	InsertBlocksCalls   int          `json:"insert_blocks_calls"`
	InsertBlocksWithId  int          `json:"insert_blocks_with_id"`
	ReplaceSubtreeCalls int          `json:"replace_subtree_calls"`
	ReplaceSubtreeIds   int          `json:"replace_subtree_with_id"`
	IdEmissions         []idEmission `json:"id_emissions,omitempty"`
	// UnknownArgCalls counts calls refused for naming an argument the tool
	// does not have — the wrapper-arm shadow of the same question.
	UnknownArgCalls int      `json:"unknown_arg_calls"`
	Refs            []refUse `json:"refs,omitempty"`
	Repairs         []repair `json:"repairs,omitempty"`
	MalformedArgs   int      `json:"malformed_args"`
}

// readingTools name the calls whose RESULT is a served document — the
// source of the ids an echo is measured against.
var readingTools = map[string]bool{"read": true, "read_object": true}

// refArgs are the arguments that carry a block reference, per surface. The
// wrapper renames the op vocabulary (inside→under, id→block, tableId→table)
// and the ops arm keeps the raw names; both are listed because one harness
// classifies both arms.
var refArgs = map[string]bool{
	"block": true, "after": true, "under": true, "table": true,
	"row": true, "col": true, "id": true, "before": true,
	"inside": true, "tableId": true,
}

// analyze computes the signals of one attempt from its call list.
func analyze(calls []callRecord) signals {
	var s signals
	var everServed map[string]bool
	var currentServed map[string]bool
	sawRead := false

	for i, call := range calls {
		if call.ArgsError != "" {
			s.MalformedArgs++
			continue
		}
		var args map[string]any
		if err := json.Unmarshal(call.Args, &args); err != nil || args == nil {
			continue
		}

		// H1 — the payload id question, and its control
		switch call.Tool {
		case "insertBlocks":
			s.InsertBlocksCalls++
			found := collectIdPaths(args, "")
			if len(found) > 0 {
				s.InsertBlocksWithId++
			}
			for _, e := range found {
				e.Turn, e.Tool = call.Turn, call.Tool
				s.IdEmissions = append(s.IdEmissions, e)
			}
		case "replaceSubtree":
			s.ReplaceSubtreeCalls++
			// the control counts ids in the PAYLOAD (blocks[…]), not the op's
			// own required `id` — that one names the block being replaced and
			// is not the field under question
			payload := map[string]any{}
			if blocks, ok := args["blocks"]; ok {
				payload["blocks"] = blocks
			}
			if found := collectIdPaths(payload, ""); len(found) > 0 {
				s.ReplaceSubtreeIds++
				for _, e := range found {
					e.Turn, e.Tool = call.Turn, call.Tool
					s.IdEmissions = append(s.IdEmissions, e)
				}
			}
		}
		if call.IsError && strings.Contains(call.ResultText, "does not take") {
			s.UnknownArgCalls++
		}

		// H2 — reference echo fidelity. Argument names are walked in sorted
		// order: map iteration is randomized, and a record that reorders
		// itself between runs of the same input is not a record.
		argNames := make([]string, 0, len(args))
		for arg := range args {
			argNames = append(argNames, arg)
		}
		sort.Strings(argNames)
		for _, arg := range argNames {
			if !refArgs[arg] {
				continue
			}
			value, ok := args[arg].(string)
			if !ok || value == "" {
				continue
			}
			s.Refs = append(s.Refs, refUse{
				Turn: call.Turn, Tool: call.Tool, Arg: arg, Value: value,
				Class:   classifyRef(value, currentServed, everServed, sawRead),
				IsError: call.IsError,
			})
		}

		// a read REFRESHES the served vocabulary for everything after it
		if readingTools[call.Tool] && !call.IsError {
			ids := servedIds(call.ResultText)
			if len(ids) > 0 {
				sawRead = true
				currentServed = ids
				if everServed == nil {
					everServed = map[string]bool{}
				}
				for id := range ids {
					everServed[id] = true
				}
			}
		}

		// H3 — the repair loop
		if call.IsError {
			s.Repairs = append(s.Repairs, classifyRepair(calls, i))
		}
	}
	sort.SliceStable(s.Refs, func(i, j int) bool { return s.Refs[i].Turn < s.Refs[j].Turn })
	return s
}

// collectIdPaths walks a decoded payload and returns every `id` key in it,
// at any depth, with its JSON path.
func collectIdPaths(v any, path string) []idEmission {
	var out []idEmission
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := k
			if path != "" {
				child = path + "." + k
			}
			if k == "id" {
				if s, ok := t[k].(string); ok {
					out = append(out, idEmission{Path: child, Value: s})
					continue
				}
			}
			out = append(out, collectIdPaths(t[k], child)...)
		}
	case []any:
		for i, item := range t {
			out = append(out, collectIdPaths(item, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}

// servedIds extracts every doc-local id from a served document: block ids,
// plus table row and column ids — exactly the strings the write channels
// take as references.
func servedIds(body string) map[string]bool {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	var doc struct {
		Blocks []struct {
			Id      string `json:"id"`
			Columns []struct {
				Id string `json:"id"`
			} `json:"columns"`
			Rows []struct {
				Id string `json:"id"`
			} `json:"rows"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return nil
	}
	ids := map[string]bool{}
	for _, b := range doc.Blocks {
		if b.Id != "" {
			ids[b.Id] = true
		}
		for _, c := range b.Columns {
			if c.Id != "" {
				ids[c.Id] = true
			}
		}
		for _, r := range b.Rows {
			if r.Id != "" {
				ids[r.Id] = true
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// classifyRef grades one echoed reference against what the reads served.
func classifyRef(value string, current, ever map[string]bool, sawRead bool) string {
	if !sawRead {
		if isHandleLike(value) {
			return refHandleLike
		}
		return refNoRead
	}
	if current[value] {
		return refExact
	}
	if ever[value] {
		return refStale
	}
	for id := range ever {
		if strings.EqualFold(id, value) {
			return refCaseFold
		}
	}
	// a bare number is a find handle written where a block reference belongs,
	// even though it is a suffix of half the ids in any document — the suffix
	// tests below would swallow it
	if isHandleLike(value) {
		return refHandleLike
	}
	for id := range ever {
		if len(value) < len(id) && strings.HasSuffix(id, value) {
			return refSuffix
		}
	}
	for id := range ever {
		if len(value) < len(id) && strings.Contains(id, value) {
			return refSubstring
		}
	}
	return refInvented
}

// isHandleLike recognises a find handle (1, 2, …) written where a block
// reference belongs.
func isHandleLike(value string) bool {
	if len(value) == 0 || len(value) > 3 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// classifyRepair looks at what the model did on the call AFTER a refusal.
func classifyRepair(calls []callRecord, i int) repair {
	call := calls[i]
	r := repair{Turn: call.Turn, Tool: call.Tool}
	if len(call.Exchanges) > 0 {
		for _, ex := range call.Exchanges {
			if ex.Status >= 400 {
				r.Status, r.Code = ex.Status, ex.Code
				if len(ex.Issues) > 0 {
					r.Path = ex.Issues[0].Path
				}
			}
		}
	}
	r.NamedField = namedField(r.Path, call.ResultText)
	if i+1 >= len(calls) {
		r.Class = repairAbandoned
		return r
	}
	next := calls[i+1]
	r.NextTool = next.Tool
	r.NextOK = !next.IsError
	if next.Tool != call.Tool {
		if readingTools[next.Tool] || next.Tool == "find" || next.Tool == "describe" || next.Tool == "spaces" {
			r.Class = repairSwitchRead
		} else {
			r.Class = repairSwitchTool
		}
		return r
	}
	if string(next.Args) == string(call.Args) {
		r.Class = repairIdentical
		return r
	}
	if r.NamedField != "" && fieldChanged(call.Args, next.Args, r.NamedField) {
		r.Class = repairFixedNamed
		return r
	}
	r.Class = repairChangedElse
	return r
}

// namedField extracts the argument an error blamed, from the text the MODEL
// saw rather than from the wire. The two differ on purpose: the wrapper
// translates the op vocabulary into the tool vocabulary before the model
// reads it (ops[0].id → block), so scoring a repair against the raw issue
// path would blame the model for not fixing a field it was never shown.
// The wire path is the fallback for errors that carry no rendered issue.
func namedField(path, text string) string {
	if field := fieldFromIssueLines(text); field != "" {
		return field
	}
	if seg := lastPathSegment(path); seg != "" {
		return seg
	}
	// wrapper-side refusals name the argument in quotes: `read: "mode" must
	// be one of full, outline`
	if i := strings.IndexByte(text, '"'); i >= 0 {
		if j := strings.IndexByte(text[i+1:], '"'); j > 0 {
			candidate := text[i+1 : i+1+j]
			if candidate != "" && !strings.ContainsAny(candidate, " \n") {
				return candidate
			}
		}
	}
	return ""
}

// fieldFromIssueLines reads the path off a rendered C6 issue line, which
// both surfaces indent and prefix with "<path>: ".
func fieldFromIssueLines(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line == "" || !(line[0] == ' ' || line[0] == '\t') {
			continue
		}
		trimmed := strings.TrimSpace(line)
		head, _, ok := strings.Cut(trimmed, ": ")
		if !ok || head == "" || strings.ContainsAny(head, " \t\"") {
			continue
		}
		if seg := lastPathSegment(head); seg != "" {
			return seg
		}
	}
	return ""
}

// lastPathSegment reduces a JSON path to the field it addresses:
// ops[0].blocks[0].id → id, block → block.
func lastPathSegment(path string) string {
	if path == "" {
		return ""
	}
	seg := path
	if i := strings.LastIndexByte(seg, '.'); i >= 0 {
		seg = seg[i+1:]
	}
	if i := strings.IndexByte(seg, '['); i >= 0 {
		seg = seg[:i]
	}
	return seg
}

// fieldChanged reports whether the named top-level argument differs between
// two calls (absent on one side counts as a change).
func fieldChanged(before, after json.RawMessage, field string) bool {
	var a, b map[string]any
	if json.Unmarshal(before, &a) != nil || json.Unmarshal(after, &b) != nil {
		return false
	}
	av, aok := a[field]
	bv, bok := b[field]
	if aok != bok {
		return true
	}
	if !aok {
		return false
	}
	return fmt.Sprintf("%v", av) != fmt.Sprintf("%v", bv)
}
