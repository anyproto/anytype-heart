package main

// classify.go — the three instrumented questions, computed from an
// attempt's recorded calls and HTTP exchanges. Each is a count over what
// the model actually emitted, never a judgment the model reported about
// itself.
//
//  1. Did an insert_blocks payload carry an `id`? The schema stopped
//     publishing one (§8.30/§8.31) precisely because a decoder emits the
//     fields it is shown. replace_subtree — which still publishes one — is
//     the control: the same model, the same run, one schema with the field
//     and one without.
//  2. When a block id was echoed back, was it the exact string the read
//     served? The default read serves 5-char labels; the write channels
//     resolve an exact id or a unique suffix, so "close" has grades.
//  3. After a refusal, did the next turn fix the field the error named, or
//     repeat itself? A refusal a model cannot act on is only half a fix.
//
// Two more, added for the edit_text A/B (toolset.go):
//
//  4. Did the model fill edit_text's optional `block`, and what did that
//     cost? EditTextWithBlock is the quantity the three arms vary the
//     published surface to move; SnippetAmbiguous is the trade B1 buys it
//     with — a refusal that names candidate blocks, which under B1 the model
//     has no argument to act on.
//  5. Wasted reads: a read whose result the model did not need. An outline
//     immediately followed by a full read of the same object (outline
//     carries text only on headings, so a snippet cannot be located in it),
//     and a full read that precedes an edit_text supplying no block. The
//     second is only waste where the prompt already carries the text to
//     find — on read-then-edit the model genuinely must read first — so the
//     two kinds are counted apart and never summed into one number.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
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

// wastedRead is one read whose result the model did not need.
type wastedRead struct {
	Turn int    `json:"turn"`
	Tool string `json:"tool"`
	Kind string `json:"kind"`
}

// wasted-read kinds.
const (
	// wasteOutlineThenFull: an outline read immediately followed by a full
	// read of the same object. Outline carries text only on headings, so a
	// model that needs a snippet's block pays for both.
	wasteOutlineThenFull = "outline_then_full"
	// wasteReadBeforeSnippetEdit: a full read whose next write is an
	// edit_text that supplied no block — the snippet located the block, and
	// the document the read served went unused for that purpose.
	wasteReadBeforeSnippetEdit = "full_read_before_snippet_edit"
)

// The locator's two refusals, recognised by the product's own words. They
// are produced by the SERVER (§8.43, v2/service/locator.go) and re-spelled
// into the tool register by the wrapper (opsVocab: "retry with id naming"
// → "retry with block naming"); a harness test drives the real wrapper
// over server-shaped bodies, and the wording pins live where each half is
// produced (v2/service/edit_test.go, wrapper/tools_smallmodel_test.go).
const (
	snippetNoMatchText   = "no block contains"
	snippetAmbiguousText = "retry with block naming one of"
	snippetTooManyText   = "provide more context to make the match unique"
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
	UnknownArgCalls int `json:"unknown_arg_calls"`
	// OpConstAbsent / OpConstWrong count the discriminator the op schemas
	// mark required with a const. The ops arm sets it from the tool name, so
	// these never affect an outcome — they measure how a small model reads a
	// const, which is the same reading skill the rest of the schema needs.
	OpConstAbsent int      `json:"op_const_absent"`
	OpConstWrong  int      `json:"op_const_wrong"`
	Refs          []refUse `json:"refs,omitempty"`
	Repairs       []repair `json:"repairs,omitempty"`
	MalformedArgs int      `json:"malformed_args"`

	// EditTextCalls / EditTextWithBlock are the edit_text A/B's quantity:
	// how often the model filled an argument it did not need. Under the
	// arm that publishes no block, a non-zero WithBlock is a field the
	// model supplied without being shown it (the runner still accepts it —
	// the arms vary the published surface, not the server).
	EditTextCalls     int `json:"edit_text_calls"`
	EditTextWithBlock int `json:"edit_text_with_block"`
	// SnippetAmbiguous / SnippetNoMatch count locateBlock's refusals — the
	// price of snippet-only location, which B1 pays in full.
	SnippetAmbiguous int          `json:"snippet_ambiguous"`
	SnippetNoMatch   int          `json:"snippet_no_match"`
	WastedReads      []wastedRead `json:"wasted_reads,omitempty"`
	// FindCalls / FindMultiMatch watch the fixture isolation: a find that
	// returns more than one object means the run is matching its own
	// leftovers again, which decays a rate for a reason that is not the API.
	FindCalls      int `json:"find_calls"`
	FindMultiMatch int `json:"find_multi_match"`
	MaxFindMatches int `json:"max_find_matches"`
	// ObjectArgIsBlockRef / SpaceIdAsObject count the two ways an id lands
	// in the wrong argument. Both are "the model had an id and one slot to
	// put it in": a block label written as `object` (seen the first time an
	// arm published no block argument), and a space id written as `object`
	// (seen when the harness handed one over naming no argument for it).
	ObjectArgIsBlockRef int `json:"object_arg_is_block_ref"`
	SpaceIdAsObject     int `json:"space_id_as_object"`
}

// opToolNames is the ops arm's tool set — the names that are also op
// discriminator values.
var opToolNames = func() map[string]bool {
	m := make(map[string]bool, len(opsArmOps))
	for _, op := range opsArmOps {
		m[op] = true
	}
	return m
}()

// readingTools name the calls whose RESULT is a served document — the
// source of the ids an echo is measured against.
var readingTools = map[string]bool{"read": true, "read_object": true}

// refArgs are the arguments that carry a block reference, per surface. The
// wrapper renames the op vocabulary (inside→under, id→block, table_id→table)
// and the ops arm keeps the raw names; both are listed because one harness
// classifies both arms.
var refArgs = map[string]bool{
	"block": true, "after": true, "under": true, "table": true,
	"row": true, "col": true, "id": true, "before": true,
	"inside": true, "table_id": true,
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
		case "insert_blocks":
			s.InsertBlocksCalls++
			found := collectIdPaths(args, "")
			if len(found) > 0 {
				s.InsertBlocksWithId++
			}
			for _, e := range found {
				e.Turn, e.Tool = call.Turn, call.Tool
				s.IdEmissions = append(s.IdEmissions, e)
			}
		case "replace_subtree":
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

		// H4 — the edit_text A/B: the optional field, and what refusing to
		// guess costs when it is absent
		if call.Tool == "edit_text" {
			s.EditTextCalls++
			block, _ := args["block"].(string)
			if block != "" {
				s.EditTextWithBlock++
			}
			// only a call that named NO block can earn a snippet-location
			// refusal. The server's own multiple-matches text is nearly the
			// wrapper's, and it fires on the explicit-block path too — where
			// it says something else entirely about the same words.
			if block == "" {
				switch {
				case strings.Contains(call.ResultText, snippetAmbiguousText),
					strings.Contains(call.ResultText, snippetTooManyText):
					s.SnippetAmbiguous++
				case strings.Contains(call.ResultText, snippetNoMatchText):
					s.SnippetNoMatch++
				}
			}
		}
		if call.Tool == "find" && !call.IsError {
			s.FindCalls++
			if n, ok := findMatchCount(call.ResultText); ok {
				if n > s.MaxFindMatches {
					s.MaxFindMatches = n
				}
				if n > 1 {
					s.FindMultiMatch++
				}
			}
		}
		if opToolNames[call.Tool] {
			switch op, present := args["op"]; {
			case !present:
				s.OpConstAbsent++
			case op != call.Tool:
				s.OpConstWrong++
			}
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

		// an `object` argument that is neither a handle nor an object id, but
		// a block reference a read served: the id the model had, in the only
		// id-shaped slot its tool offered
		if target, ok := args["object"].(string); ok && target != "" && !isHandleLike(target) && everServed[target] {
			s.ObjectArgIsBlockRef++
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
	s.WastedReads = wastedReads(calls)
	return s
}

// wastedReads finds the reads whose result the model did not need. Both
// kinds are structural — an outline superseded by a full read of the same
// object, and a full read followed by an edit that located its block from
// the snippet — so neither depends on judging what the model "meant".
func wastedReads(calls []callRecord) []wastedRead {
	var out []wastedRead
	counted := map[int]bool{}
	for i, call := range calls {
		if !readingTools[call.Tool] || call.IsError || i+1 >= len(calls) {
			continue
		}
		next := calls[i+1]
		if !readingTools[next.Tool] || next.IsError {
			continue
		}
		if readIsOutline(call) && !readIsOutline(next) && readTarget(call) == readTarget(next) {
			counted[i] = true
			out = append(out, wastedRead{Turn: call.Turn, Tool: call.Tool, Kind: wasteOutlineThenFull})
		}
	}
	for i, call := range calls {
		// the edit's OUTCOME is deliberately not a condition: whether the
		// model needed the read to compose the call is answered by the call's
		// arguments. Requiring success would undercount exactly on the arm
		// whose edits fail most, which is the arm the experiment is about.
		if call.Tool != "edit_text" {
			continue
		}
		var args map[string]any
		if json.Unmarshal(call.Args, &args) != nil {
			continue
		}
		if block, _ := args["block"].(string); block != "" {
			continue
		}
		// walk back to the read that fed this edit, past nothing but reads:
		// a read separated from the edit by another write served that write
		for j := i - 1; j >= 0; j-- {
			prior := calls[j]
			if !readingTools[prior.Tool] {
				break
			}
			if prior.IsError || readIsOutline(prior) || counted[j] {
				continue
			}
			counted[j] = true
			out = append(out, wastedRead{Turn: prior.Turn, Tool: prior.Tool, Kind: wasteReadBeforeSnippetEdit})
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Turn < out[j].Turn })
	return out
}

// countSpaceIdAsObject counts calls that passed the SPACE id where an object
// belongs. The harness's own preamble caused this once — it named the space
// id without naming the argument it belongs to — so the count has to survive
// the fix that was supposed to end it, or nobody learns whether it did.
func countSpaceIdAsObject(calls []callRecord, spaceId string) int {
	n := 0
	for _, call := range calls {
		var args map[string]any
		if json.Unmarshal(call.Args, &args) != nil {
			continue
		}
		if target, _ := args["object"].(string); target == spaceId {
			n++
		}
	}
	return n
}

// readIsOutline reports whether a read asked for the outline shape — the
// wrapper spells it mode=outline, the ops arm outline=true.
func readIsOutline(call callRecord) bool {
	var args map[string]any
	if json.Unmarshal(call.Args, &args) != nil {
		return false
	}
	if mode, _ := args["mode"].(string); mode == "outline" {
		return true
	}
	outline, _ := args["outline"].(bool)
	return outline
}

// readTarget names the object a read addressed; the ops arm's read_object is
// bound to one object and names none.
func readTarget(call callRecord) string {
	var args map[string]any
	if json.Unmarshal(call.Args, &args) != nil {
		return ""
	}
	target, _ := args["object"].(string)
	return target
}

// findMatchCountRe reads the count off find's own summary line ("3 matches",
// "12 matches — showing 10; narrow with …").
var findMatchCountRe = regexp.MustCompile(`(?m)^(\d+) match`)

// findMatchCount returns how many objects a find reported.
func findMatchCount(text string) (int, bool) {
	if strings.Contains(text, "no matches") {
		return 0, true
	}
	m := findMatchCountRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
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
