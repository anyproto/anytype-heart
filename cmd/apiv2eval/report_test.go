package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func TestSummaryExcludesEnvironmentFailuresFromTheRate(t *testing.T) {
	// given — three attempts in one cell: one pass, one real failure, one
	// the model host timed out on. The rate must be 1/2, never 1/3: an
	// environment failure is not evidence about the API.
	attempts := []attemptRecord{
		{Model: "m", Arm: "ops", Task: "edit-one-word", Seq: 1, Outcome: outcomeSuccess, Turns: 2, ToolCalls: 2},
		{Model: "m", Arm: "ops", Task: "edit-one-word", Seq: 2, Outcome: outcomeFailure, Turns: 4, ToolCalls: 4, FailedCalls: 2,
			CheckDetail: "Q3 still present"},
		{Model: "m", Arm: "ops", Task: "edit-one-word", Seq: 3, Outcome: outcomeEnv, Turns: 1, EnvError: "model timeout"},
	}

	// when
	got := buildSummary("run1", attempts, options{n: 3, maxTurns: 8})

	// then
	assert.Contains(t, got, "1/2")
	assert.NotContains(t, got, "1/3")
	assert.Contains(t, got, "Environment failures (excluded from every rate above)")
	assert.Contains(t, got, "model timeout")
}

func TestSummaryQuotesAFailingTranscriptAndItsRefusals(t *testing.T) {
	// given
	failing := attemptRecord{
		Model: "m", Arm: "wrapper/small", Task: "edit-one-word", Seq: 1,
		Outcome: outcomeFailure, CheckDetail: "Q3 still present",
		Prompt: `In the note titled "Quarterly plan ab12", change Q3 to Q4.`,
		Transcript: &transcript{
			Calls: []callRecord{{
				Turn: 1, Tool: "edit_text",
				Args:       json.RawMessage(`{"object":"1","block":"zz","find":"Q3","replace":"Q4"}`),
				IsError:    true,
				ResultText: "block \"zz\" not found\n  block: no block matches \"zz\"",
				Exchanges: []exchange{{
					Method: "PATCH", Path: "/v2/spaces/s1/objects/obj1", Status: 400, Code: "invalid_input",
					Message: `block "zz" not found`,
					Issues:  []v2model.Issue{{Path: "ops[0].id", Message: `no block matches "zz"`}},
				}},
			}},
			FinalContent: "I updated the note.",
		},
		Signals: signals{Repairs: []repair{{Class: repairAbandoned, Code: "invalid_input"}}},
	}

	// when
	got := buildSummary("run1", []attemptRecord{failing}, options{n: 1, maxTurns: 8})

	// then
	assert.Contains(t, got, "One failing transcript per (arm, task)")
	assert.Contains(t, got, `{"object":"1","block":"zz","find":"Q3","replace":"Q4"}`)
	assert.Contains(t, got, "400 invalid_input ops[0].id", "the refusal table keeps the wire path")
	assert.Contains(t, got, "said: I updated the note.", "the model's self-report is quoted, never believed")
	assert.Contains(t, got, "check: Q3 still present")
	assert.Contains(t, got, repairAbandoned)
}

func TestSummaryReportsTheInsertBlocksControlSideBySide(t *testing.T) {
	// given
	attempts := []attemptRecord{{
		Model: "m", Arm: "ops", Task: "append-section", Outcome: outcomeSuccess,
		Signals: signals{
			InsertBlocksCalls: 4, InsertBlocksWithId: 1,
			ReplaceSubtreeCalls: 2, ReplaceSubtreeIds: 2,
		},
	}}

	// when
	got := buildSummary("run1", attempts, options{n: 1, maxTurns: 8})

	// then
	require.Contains(t, got, "H1 — does the model emit an id where the schema does not show one?")
	assert.Contains(t, got, "replaceSubtree still does — the control")
	assert.Contains(t, got, "4")
}

func TestSummaryHandlesAnEmptyRun(t *testing.T) {
	// when — an interrupted run before its first attempt must still write a
	// summary rather than panic
	got := buildSummary("run1", nil, options{n: 3, maxTurns: 8})

	// then
	assert.Contains(t, got, "attempts: 0")
}
