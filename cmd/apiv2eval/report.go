package main

// report.go — the readable summary beside the JSONL. Rates are printed as
// k/n with n always visible: with three or five attempts per cell a bare
// percentage would be a lie of precision, and small models vary enough that
// the spread between cells matters more than any single number.

import (
	"fmt"
	"sort"
	"strings"
)

// cellKey identifies one (model, arm, task) cell.
type cellKey struct {
	model string
	arm   string
	task  string
}

type cellStats struct {
	attempts int
	success  int
	env      int
	turns    int
	tokens   int
	calls    int
	failed   int
}

func (c cellStats) counted() int { return c.attempts - c.env }

// buildSummary renders the whole report.
func buildSummary(runId string, attempts []attemptRecord, opt options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "API v2 small-model evaluation — run %s\n", runId)
	fmt.Fprintf(&b, "attempts per cell (n): %d · turn budget: %d · temperature: %.2f\n",
		opt.n, opt.maxTurns, opt.temperature)
	fmt.Fprintf(&b, "attempts: %d\n\n", len(attempts))

	byCell := map[cellKey]*cellStats{}
	byArm := map[cellKey]*cellStats{}
	for _, a := range attempts {
		ck := cellKey{a.Model, a.Arm, a.Task}
		ak := cellKey{a.Model, a.Arm, ""}
		for _, key := range []struct {
			m map[cellKey]*cellStats
			k cellKey
		}{{byCell, ck}, {byArm, ak}} {
			s := key.m[key.k]
			if s == nil {
				s = &cellStats{}
				key.m[key.k] = s
			}
			s.attempts++
			switch a.Outcome {
			case outcomeSuccess:
				s.success++
			case outcomeEnv:
				s.env++
			}
			if a.Outcome != outcomeEnv {
				s.turns += a.Turns
				s.tokens += a.PromptTokens + a.CompletionTokens
				s.calls += a.ToolCalls
				s.failed += a.FailedCalls
			}
		}
	}

	b.WriteString("## Success rate by surface\n\n")
	b.WriteString(fmt.Sprintf("%-14s %-14s %8s %8s %8s %8s %8s\n", "model", "arm", "passed", "rate", "turns", "tokens", "err/call"))
	for _, k := range sortedCellKeys(byArm) {
		s := byArm[k]
		fmt.Fprintf(&b, "%-14s %-14s %8s %8s %8.1f %8.0f %8s\n",
			k.model, k.arm, fmt.Sprintf("%d/%d", s.success, s.counted()), rate(s),
			mean(s.turns, s.counted()), mean(s.tokens, s.counted()),
			fmt.Sprintf("%d/%d", s.failed, s.calls))
	}

	b.WriteString("\n## Success rate by task\n\n")
	b.WriteString(fmt.Sprintf("%-14s %-14s %-16s %8s %8s\n", "model", "arm", "task", "passed", "turns"))
	for _, k := range sortedCellKeys(byCell) {
		s := byCell[k]
		fmt.Fprintf(&b, "%-14s %-14s %-16s %8s %8.1f\n",
			k.model, k.arm, k.task, fmt.Sprintf("%d/%d", s.success, s.counted()), mean(s.turns, s.counted()))
	}

	b.WriteString("\n## H1 — does the model emit an id where the schema does not show one?\n")
	b.WriteString("(insertBlocks publishes no id slot since §8.30/§8.31; replaceSubtree still does — the control)\n\n")
	b.WriteString(fmt.Sprintf("%-14s %-14s %14s %14s %16s %16s\n", "model", "arm", "insertBlocks", "…with id", "replaceSubtree", "…with id"))
	h1 := map[cellKey]*[4]int{}
	for _, a := range attempts {
		k := cellKey{a.Model, a.Arm, ""}
		if h1[k] == nil {
			h1[k] = &[4]int{}
		}
		h1[k][0] += a.Signals.InsertBlocksCalls
		h1[k][1] += a.Signals.InsertBlocksWithId
		h1[k][2] += a.Signals.ReplaceSubtreeCalls
		h1[k][3] += a.Signals.ReplaceSubtreeIds
	}
	for _, k := range sortedIntCellKeys(h1) {
		v := h1[k]
		fmt.Fprintf(&b, "%-14s %-14s %14d %14d %16d %16d\n", k.model, k.arm, v[0], v[1], v[2], v[3])
	}
	unknownArg, opAbsent, opWrong := 0, 0, 0
	for _, a := range attempts {
		unknownArg += a.Signals.UnknownArgCalls
		opAbsent += a.Signals.OpConstAbsent
		opWrong += a.Signals.OpConstWrong
	}
	fmt.Fprintf(&b, "\ncalls refused for naming an argument the tool does not have: %d\n", unknownArg)
	b.WriteString("(the wrapper's add_blocks has no id channel at all — on that surface the field is unemittable by construction)\n")
	fmt.Fprintf(&b, "ops-arm `op` discriminator: %d absent, %d wrong (set from the tool name either way — never an outcome)\n",
		opAbsent, opWrong)

	b.WriteString("\n## H2 — was an echoed block id the exact string the read served?\n\n")
	refClasses := []string{refExact, refSuffix, refStale, refCaseFold, refSubstring, refHandleLike, refInvented, refNoRead}
	h2 := map[cellKey]map[string]int{}
	for _, a := range attempts {
		k := cellKey{a.Model, a.Arm, ""}
		if h2[k] == nil {
			h2[k] = map[string]int{}
		}
		for _, r := range a.Signals.Refs {
			h2[k][r.Class]++
		}
	}
	writeClassCounts(&b, h2, refClasses)

	b.WriteString("\n## H3 — after a refusal, what did the next turn do?\n\n")
	repairClasses := []string{repairFixedNamed, repairChangedElse, repairIdentical, repairSwitchRead, repairSwitchTool, repairAbandoned}
	h3 := map[cellKey]map[string]int{}
	for _, a := range attempts {
		k := cellKey{a.Model, a.Arm, ""}
		if h3[k] == nil {
			h3[k] = map[string]int{}
		}
		for _, r := range a.Signals.Repairs {
			h3[k][r.Class]++
		}
	}
	writeClassCounts(&b, h3, repairClasses)
	wrongTarget := 0
	for _, a := range attempts {
		if a.WrongTargetWrites > 0 {
			wrongTarget++
		}
	}
	if wrongTarget > 0 {
		fmt.Fprintf(&b, "\nattempts that wrote to an object other than their fixture: %d\n", wrongTarget)
	}

	recovered, withErrors := 0, 0
	for _, a := range attempts {
		if a.Outcome == outcomeEnv || a.FailedCalls == 0 {
			continue
		}
		withErrors++
		if a.Outcome == outcomeSuccess {
			recovered++
		}
	}
	fmt.Fprintf(&b, "\nattempts that hit at least one refusal and still passed: %d/%d\n", recovered, withErrors)

	b.WriteString("\n## Refusals by (status, code, path)\n\n")
	type errKey struct {
		status int
		code   string
		path   string
	}
	errCounts := map[errKey]int{}
	errSample := map[errKey]string{}
	for _, a := range attempts {
		if a.Transcript == nil {
			continue
		}
		for _, c := range a.Transcript.Calls {
			for _, ex := range c.Exchanges {
				if ex.Status < 400 {
					continue
				}
				path := ""
				if len(ex.Issues) > 0 {
					path = ex.Issues[0].Path
				}
				k := errKey{ex.Status, ex.Code, path}
				errCounts[k]++
				if errSample[k] == "" {
					errSample[k] = ex.Message
				}
			}
		}
	}
	type errRow struct {
		k errKey
		n int
	}
	var rows []errRow
	for k, n := range errCounts {
		rows = append(rows, errRow{k, n})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].n != rows[j].n {
			return rows[i].n > rows[j].n
		}
		return rows[i].k.code < rows[j].k.code
	})
	for _, r := range rows {
		fmt.Fprintf(&b, "%4d× %d %s %s\n     %s\n", r.n, r.k.status, r.k.code, r.k.path, firstLine(errSample[r.k]))
	}
	if len(rows) == 0 {
		b.WriteString("(none)\n")
	}

	// wrapper-side refusals never become HTTP calls — they are counted apart
	wrapperRefusals := map[string]int{}
	for _, a := range attempts {
		if a.Transcript == nil {
			continue
		}
		for _, c := range a.Transcript.Calls {
			if !c.IsError || len(c.Exchanges) > 0 {
				continue
			}
			wrapperRefusals[firstLine(c.ResultText)]++
		}
	}
	if len(wrapperRefusals) > 0 {
		b.WriteString("\n## Refusals raised before any HTTP call (wrapper-side validation)\n\n")
		type wr struct {
			text string
			n    int
		}
		var wrs []wr
		for text, n := range wrapperRefusals {
			wrs = append(wrs, wr{text, n})
		}
		sort.Slice(wrs, func(i, j int) bool { return wrs[i].n > wrs[j].n })
		for _, w := range wrs {
			fmt.Fprintf(&b, "%4d× %s\n", w.n, w.text)
		}
	}

	// One quoted failure per (arm, task): a rate says how often the loop
	// breaks, a transcript says why, and a paraphrase of a transcript is
	// worth neither.
	quoted := map[cellKey]bool{}
	var failures []attemptRecord
	for _, a := range attempts {
		if a.Outcome != outcomeFailure {
			continue
		}
		k := cellKey{"", a.Arm, a.Task}
		if quoted[k] {
			continue
		}
		quoted[k] = true
		failures = append(failures, a)
	}
	if len(failures) > 0 {
		b.WriteString("\n## One failing transcript per (arm, task)\n")
		for _, a := range failures {
			fmt.Fprintf(&b, "\n%s · %s · %s #%d\n", a.Model, a.Arm, a.Task, a.Seq)
			fmt.Fprintf(&b, "  prompt: %s\n", a.Prompt)
			if a.Transcript != nil {
				b.WriteString(summarizeCalls(a.Transcript.Calls))
				if a.Transcript.FinalContent != "" {
					fmt.Fprintf(&b, "  said: %s\n", firstLine(a.Transcript.FinalContent))
				}
			}
			fmt.Fprintf(&b, "  check: %s\n", a.CheckDetail)
		}
	}

	envAttempts := 0
	for _, a := range attempts {
		if a.Outcome == outcomeEnv {
			envAttempts++
		}
	}
	if envAttempts > 0 {
		b.WriteString("\n## Environment failures (excluded from every rate above)\n\n")
		for _, a := range attempts {
			if a.Outcome != outcomeEnv {
				continue
			}
			fmt.Fprintf(&b, "%-14s %-14s %-16s #%d — %s\n", a.Model, a.Arm, a.Task, a.Seq, firstLine(a.EnvError))
		}
	}
	return b.String()
}

// writeClassCounts renders one classification per (model, arm) as a compact
// key=value line. A column per class would be 160 characters wide and
// mostly zeros; the classes that fired are the ones worth reading.
func writeClassCounts(b *strings.Builder, counts map[cellKey]map[string]int, classes []string) {
	for _, k := range sortedClassKeys(counts) {
		var parts []string
		total := 0
		for _, class := range classes {
			if n := counts[k][class]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s=%d", class, n))
				total += n
			}
		}
		if total == 0 {
			parts = append(parts, "(none)")
		}
		fmt.Fprintf(b, "%-14s %-14s %3d · %s\n", k.model, k.arm, total, strings.Join(parts, " · "))
	}
	if len(counts) == 0 {
		b.WriteString("(none)\n")
	}
}

func rate(s *cellStats) string {
	if s.counted() == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(s.success)/float64(s.counted()))
}

func mean(total, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(total) / float64(n)
}

func sortedCellKeys(m map[cellKey]*cellStats) []cellKey {
	keys := make([]cellKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortCellKeys(keys)
	return keys
}

func sortedIntCellKeys(m map[cellKey]*[4]int) []cellKey {
	keys := make([]cellKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortCellKeys(keys)
	return keys
}

func sortedClassKeys(m map[cellKey]map[string]int) []cellKey {
	keys := make([]cellKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortCellKeys(keys)
	return keys
}

func sortCellKeys(keys []cellKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].model != keys[j].model {
			return keys[i].model < keys[j].model
		}
		if keys[i].arm != keys[j].arm {
			return keys[i].arm < keys[j].arm
		}
		return keys[i].task < keys[j].task
	})
}
