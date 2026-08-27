package eval

// counters.go: the harness's token and turn counters (APIV2.md §2 Phase 0
// metrics: output tokens, turns).

// CountTokens approximates the token cost of a payload with the standard
// ~4-bytes-per-token rule of thumb for English/JSON text. The harness
// compares methods against each other, so a consistent approximation
// suffices; swap in a real tokenizer when absolute numbers matter.
func CountTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}

// RunMeter accumulates the per-run metrics of one (task, model, method)
// combination: agent turns and token totals per direction.
type RunMeter struct {
	Turns        int
	InputTokens  int
	OutputTokens int
}

// RecordTurn counts one agent turn with its input (prompt/tool result) and
// output (model completion) payloads.
func (m *RunMeter) RecordTurn(input, output string) {
	m.Turns++
	m.InputTokens += CountTokens(input)
	m.OutputTokens += CountTokens(output)
}
