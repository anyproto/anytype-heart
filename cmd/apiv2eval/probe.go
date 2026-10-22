package main

// probe.go — the schema-emission probe: one turn, no live API.
//
// H1 asks whether a model emits a field the schema does not show it. That
// question is answered by the FIRST payload a model writes, and the first
// payload depends on the tool schema alone — not on anything the server
// answers. So the probe hands the model the API's own published op schemas
// (read in-process from the same table the /v2/schemas/ops route serves),
// takes exactly one completion, and records what it wrote.
//
// It is a strictly weaker instrument than a run: it says nothing about
// recovery, about ids echoed from a read, or about whether an edit lands.
// It exists so the one question that does NOT need a live server can be
// answered when there is no live server — and, when there is, as a cheap
// large-n complement to the loop runs.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// probeOps are the two ops the payload-id question is about: insert_blocks
// publishes no id slot since §8.30/§8.31, replace_subtree still does. Same
// model, same prompt shape, one schema with the field and one without.
var probeOps = []string{"insert_blocks", "replace_subtree"}

// probeCase is one authoring intent.
type probeCase struct {
	id     string
	prompt string
	// wantOp is the op the intent calls for; a call to the other one is
	// recorded, not corrected.
	wantOp string
}

func probeCases() []probeCase {
	return []probeCase{
		{
			id:     "add_section",
			wantOp: "insert_blocks",
			prompt: "Add a section at the end of this document: a level-2 heading reading Risks, " +
				"then two bullet points reading Vendor delay and Budget overrun.",
		},
		{
			id:     "add_table",
			wantOp: "insert_blocks",
			prompt: "Add a table at the end of this document with two columns, a header row reading " +
				"Component and Status, and one row reading Beta and Pending.",
		},
		{
			id:     "add_after_block",
			wantOp: "insert_blocks",
			prompt: "The document has a paragraph with id b7. Add a checkbox item reading Follow up directly after it.",
		},
		{
			id:     "replace_subtree",
			wantOp: "replace_subtree",
			prompt: "The document has a bulleted list item with id b7. Replace it and everything under it with " +
				"a single paragraph reading Deferred to Q4.",
		},
		// The matched pair: the SAME temptation put to both ops. A read is
		// quoted, so an id is right there to copy, and the intent is
		// phrased so carrying it over is a plausible reading. One op's
		// schema publishes the slot, the other's does not — which is the
		// whole §8.30 claim, reduced to two prompts that differ only in
		// which tool answers them.
		{
			id:     "copy_block_new",
			wantOp: "insert_blocks",
			prompt: "A read of this document returned this block:\n" +
				`{"id":"c3f1a","type":"bulleted_list_item","text":"Ship the beta"}` + "\n" +
				"Add a second, identical bullet at the end of the document.",
		},
		{
			id:     "echo_block_existing",
			wantOp: "replace_subtree",
			prompt: "A read of this document returned this block:\n" +
				`{"id":"c3f1a","type":"bulleted_list_item","text":"Ship the beta"}` + "\n" +
				"Replace that block with a paragraph reading Shipped, keeping the block's identity.",
		},
	}
}

// probeRecord is one probe attempt.
type probeRecord struct {
	Run       string    `json:"run"`
	StartedAt time.Time `json:"started_at"`
	Model     string    `json:"model"`
	Case      string    `json:"case"`
	Seq       int       `json:"seq"`
	WantOp    string    `json:"want_op"`
	// ConstAsEnum records whether the discriminator's const was spelled as a
	// single-value enum — see probeToolSpecs.
	ConstAsEnum bool   `json:"const_as_enum,omitempty"`
	CalledOp    string `json:"called_op,omitempty"`
	Args        string `json:"args,omitempty"`
	// Channel is which authoring channel an insert_blocks call used: markdown
	// (no id is expressible at all) or blocks (where the removed slot was).
	Channel     string       `json:"channel,omitempty"`
	IdEmissions []idEmission `json:"id_emissions,omitempty"`
	// RefusalRisks name payload shapes the server refuses, recognised
	// statically (see staticRefusalRisks) — the probe never sends anything.
	RefusalRisks []string `json:"refusal_risks,omitempty"`
	// MissingOpConst records that the payload omitted `op`, which every op
	// schema marks required with a const; OpConstValue is what it wrote
	// instead when it wrote something other than the const. The tool name
	// already determines the value, so this measures schema compliance, not
	// intent — and the wrong-value case is the common one, so it is counted
	// apart from the absent one.
	MissingOpConst bool   `json:"missing_op_const,omitempty"`
	OpConstValue   string `json:"op_const_value,omitempty"`
	NoToolCall     bool   `json:"no_tool_call,omitempty"`
	ArgsError      string `json:"args_error,omitempty"`
	EnvError       string `json:"env_error,omitempty"`
	Usage          usage  `json:"usage"`
}

// runProbe runs the one-turn schema-emission probe over the model list.
func runProbe(ctx context.Context, chat *chatClient, models []string, opt options) error {
	specs, err := probeToolSpecs(opt.constAsEnum)
	if err != nil {
		return err
	}
	tools := make([]toolDef, 0, len(specs))
	for _, spec := range specs {
		tools = append(tools, newToolDef(spec.Name, spec.Description, spec.Parameters))
	}
	system := "You edit one Anytype document through its HTTP API. Each tool is a single PATCH op; " +
		"its parameters are the API's own published schema for that op — follow it exactly. " +
		"Call exactly one tool."

	if err := os.MkdirAll(opt.outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(opt.outDir, "probe.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open probe file: %w", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()

	runId := time.Now().UTC().Format("20060102-150405")
	fmt.Printf("probe %s — %d models × %d cases × n=%d (one turn each, no API needed)\n",
		runId, len(models), len(probeCases()), opt.n)

	var records []probeRecord
	for _, model := range models {
		for seq := 1; seq <= opt.n; seq++ {
			for _, pc := range probeCases() {
				if ctx.Err() != nil {
					return writeProbeSummary(writer, records, runId, opt)
				}
				rec := probeOnce(ctx, chat, model, pc, tools, system, opt, runId, seq)
				rec.ConstAsEnum = opt.constAsEnum
				records = append(records, rec)
				line, err := json.Marshal(rec)
				if err != nil {
					return fmt.Errorf("encode probe record: %w", err)
				}
				if _, err := writer.Write(append(line, '\n')); err != nil {
					return fmt.Errorf("write probe record: %w", err)
				}
				writer.Flush()
				fmt.Printf("  %-12s %-16s #%d → %-15s %-9s ids=%d %s\n",
					model, pc.id, seq, orDash(rec.CalledOp), orDash(rec.Channel),
					len(rec.IdEmissions), firstLine(rec.EnvError))
			}
		}
	}
	return writeProbeSummary(writer, records, runId, opt)
}

func probeOnce(ctx context.Context, chat *chatClient, model string, pc probeCase,
	tools []toolDef, system string, opt options, runId string, seq int) probeRecord {
	rec := probeRecord{
		Run: runId, StartedAt: time.Now(), Model: model, Case: pc.id, Seq: seq, WantOp: pc.wantOp,
	}
	messages := []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: pc.prompt}}
	resp, err := chat.complete(ctx, model, messages, tools, opt.temperature)
	if err != nil {
		rec.EnvError = err.Error()
		return rec
	}
	rec.Usage = resp.Usage
	if len(resp.Message.ToolCalls) == 0 {
		rec.NoToolCall = true
		return rec
	}
	call := resp.Message.ToolCalls[0]
	rec.CalledOp = call.Function.Name
	raw, err := call.argsJSON()
	if err != nil {
		rec.ArgsError = err.Error()
		return rec
	}
	rec.Args = string(raw)
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		rec.ArgsError = err.Error()
		return rec
	}
	switch {
	case args["markdown"] != nil:
		rec.Channel = "markdown"
	case args["blocks"] != nil:
		rec.Channel = "blocks"
	}
	// the op's OWN id (replace_subtree's target) is not the field under
	// question — only ids inside the authored payload are
	payload := map[string]any{}
	if blocks, ok := args["blocks"]; ok {
		payload["blocks"] = blocks
	}
	rec.IdEmissions = collectIdPaths(payload, "")
	for i := range rec.IdEmissions {
		rec.IdEmissions[i].Tool = call.Function.Name
	}
	rec.RefusalRisks = staticRefusalRisks(call.Function.Name, args)
	op, hasOp := args["op"]
	rec.MissingOpConst = !hasOp
	if hasOp {
		if written, _ := json.Marshal(op); string(written) != strconv.Quote(call.Function.Name) {
			rec.OpConstValue = string(written)
		}
	}
	return rec
}

// refusal risks recognised without sending anything.
const (
	riskPositionNotIside = "position_with_after_or_before"
)

// staticRefusalRisks names payload shapes the server refuses, recognised
// from the payload alone. Exactly one guard is transcribed here — the
// targeting rule in resolveTarget (stateops.go), which refuses `position`
// alongside `after`/`before`, where the anchor already names the slot.
// Nothing else is duplicated here — a full local validator would be a second
// implementation of the server, and the loop runs against the real one.
func staticRefusalRisks(op string, args map[string]any) []string {
	if op != "insert_blocks" && op != "move_block" {
		return nil
	}
	position, _ := args["position"].(string)
	if position == "" {
		return nil
	}
	targeted := ""
	for _, field := range []string{"after", "before", "inside"} {
		if v, _ := args[field].(string); v != "" {
			targeted = field
		}
	}
	if targeted == "after" || targeted == "before" {
		return []string{riskPositionNotIside}
	}
	return nil
}

// probeToolSpecs reads the published op schemas in-process, from the same
// table GET /v2/schemas/ops/{op} serves — so the probe runs with no server,
// on the bytes the server would have sent. Each published example is one op
// object, matching the schema beside it.
func probeToolSpecs(constAsEnum bool) ([]toolSpec, error) {
	var svc v2service.Service
	specs := make([]toolSpec, 0, len(probeOps))
	for _, op := range probeOps {
		entry, err := svc.SchemaOp(op)
		if err != nil {
			return nil, fmt.Errorf("read published schema for %q: %w", op, err)
		}
		schema := entry.Schema
		if constAsEnum {
			schema = rewriteConstAsEnum(schema, op)
		}
		specs = append(specs, toolSpec{
			Name:        op,
			Description: fmt.Sprintf("PATCH op %q on the document. Example: %s", op, entry.Example),
			Parameters:  schema,
		})
	}
	return specs, nil
}

// rewriteConstAsEnum turns the discriminator's `const` into a single-value
// `enum`. This is the ONE place the harness alters a served schema, and it
// is a diagnostic, never a measurement: when a model writes a positional
// word into a field pinned by `const`, there are two explanations — the
// model ignored the keyword, or the host's tool-schema rendering dropped it
// before the model ever saw it. `enum` is the older, more widely handled
// spelling of the same constraint, so the same run with one keyword swapped
// separates them. The default is always the schema as served.
func rewriteConstAsEnum(schema json.RawMessage, op string) json.RawMessage {
	from := fmt.Sprintf(`"op":{"const":%q}`, op)
	to := fmt.Sprintf(`"op":{"enum":[%q]}`, op)
	return json.RawMessage(strings.Replace(string(schema), from, to, 1))
}

func writeProbeSummary(writer *bufio.Writer, records []probeRecord, runId string, opt options) error {
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush probe file: %w", err)
	}
	summary := buildProbeSummary(runId, records, opt)
	path := filepath.Join(opt.outDir, "probe-summary.txt")
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		return fmt.Errorf("write probe summary: %w", err)
	}
	fmt.Println()
	fmt.Print(summary)
	return nil
}

// buildProbeSummary renders the probe table.
func buildProbeSummary(runId string, records []probeRecord, opt options) string {
	var b []byte
	add := func(format string, args ...any) { b = append(b, fmt.Sprintf(format, args...)...) }
	add("Schema-emission probe — run %s\n", runId)
	add("one turn per attempt, published op schemas, no live API\n")
	add("attempts per (model, case): %d · attempts: %d\n\n", opt.n, len(records))

	type key struct{ model, op string }
	type agg struct {
		calls, withId, markdown, blocks, none, envErr int
	}
	byOp := map[key]*agg{}
	for _, r := range records {
		k := key{r.Model, orDash(r.CalledOp)}
		if byOp[k] == nil {
			byOp[k] = &agg{}
		}
		a := byOp[k]
		a.calls++
		if len(r.IdEmissions) > 0 {
			a.withId++
		}
		switch r.Channel {
		case "markdown":
			a.markdown++
		case "blocks":
			a.blocks++
		}
		if r.NoToolCall {
			a.none++
		}
		if r.EnvError != "" {
			a.envErr++
		}
	}
	keys := make([]key, 0, len(byOp))
	for k := range byOp {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].model != keys[j].model {
			return keys[i].model < keys[j].model
		}
		return keys[i].op < keys[j].op
	})
	add("%-14s %-16s %7s %10s %10s %8s %8s\n", "model", "tool called", "calls", "…with id", "markdown", "blocks", "no call")
	for _, k := range keys {
		a := byOp[k]
		add("%-14s %-16s %7d %10d %10d %8d %8d\n", k.model, k.op, a.calls, a.withId, a.markdown, a.blocks, a.none)
	}

	add("\nid emissions, by payload path\n\n")
	paths := map[string]int{}
	for _, r := range records {
		for _, e := range r.IdEmissions {
			paths[e.Tool+" "+e.Path]++
		}
	}
	if len(paths) == 0 {
		add("(none)\n")
	} else {
		type row struct {
			path string
			n    int
		}
		var rows []row
		for p, n := range paths {
			rows = append(rows, row{p, n})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })
		for _, r := range rows {
			add("%4d× %s\n", r.n, r.path)
		}
	}

	add("\npayloads the server would refuse, by shape\n\n")
	risks := map[string]int{}
	for _, r := range records {
		for _, risk := range r.RefusalRisks {
			risks[r.Model+" "+r.CalledOp+" "+risk]++
		}
	}
	if len(risks) == 0 {
		add("(none)\n")
	} else {
		names := make([]string, 0, len(risks))
		for name := range risks {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool { return risks[names[i]] > risks[names[j]] })
		for _, name := range names {
			add("%4d× %s\n", risks[name], name)
		}
	}

	missingOp := map[string]int{}
	wrongOp := map[string]int{}
	wroteValues := map[string]int{}
	calls := map[string]int{}
	for _, r := range records {
		if r.CalledOp == "" {
			continue
		}
		calls[r.Model]++
		switch {
		case r.MissingOpConst:
			missingOp[r.Model]++
		case r.OpConstValue != "":
			wrongOp[r.Model]++
			wroteValues[r.Model+" wrote "+r.OpConstValue]++
		}
	}
	add("\nthe required `op` const (the tool name determines its one legal value)\n\n")
	models := make([]string, 0, len(calls))
	for m := range calls {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		add("%-14s %d/%d absent, %d/%d wrong\n", m, missingOp[m], calls[m], wrongOp[m], calls[m])
	}
	values := make([]string, 0, len(wroteValues))
	for v := range wroteValues {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool { return wroteValues[values[i]] > wroteValues[values[j]] })
	for _, v := range values {
		add("%4d× %s\n", wroteValues[v], v)
	}

	envErrs := 0
	for _, r := range records {
		if r.EnvError != "" {
			envErrs++
		}
	}
	if envErrs > 0 {
		add("\nenvironment failures (excluded): %d\n", envErrs)
	}
	return string(b)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
