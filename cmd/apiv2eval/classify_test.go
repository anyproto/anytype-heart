package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// servedDoc is a read result in the shape the API serves: compact 5-char
// labels for minted block ids.
const servedDoc = `{"id":"obj1","blocks":[` +
	`{"id":"a1b2c","type":"heading2","text":"Summary"},` +
	`{"id":"d4e5f","type":"paragraph","text":"Revenue target for Q3 is 1.2M."},` +
	`{"id":"t9d2c","type":"table","columns":[{"id":"c0011"},{"id":"c0022"}],` +
	`"rows":[{"id":"r0011","isHeader":true,"cells":["Component","Status"]},{"id":"r0022","cells":["Beta","Pending"]}]}` +
	`]}`

func call(turn int, tool, args string) callRecord {
	return callRecord{Turn: turn, Tool: tool, Args: json.RawMessage(args)}
}

func TestAnalyzeInsertBlocksIdEmission(t *testing.T) {
	t.Run("an id anywhere in an insert_blocks payload is counted, with its path", func(t *testing.T) {
		// given
		calls := []callRecord{
			call(0, "insert_blocks", `{"op":"insert_blocks","blocks":[{"id":"b17","type":"paragraph","text":"hi"}]}`),
			call(1, "insert_blocks", `{"op":"insert_blocks","markdown":"## Risks"}`),
		}

		// when
		got := analyze(calls)

		// then
		assert.Equal(t, 2, got.InsertBlocksCalls)
		assert.Equal(t, 1, got.InsertBlocksWithId)
		require.Len(t, got.IdEmissions, 1)
		assert.Equal(t, "blocks[0].id", got.IdEmissions[0].Path)
		assert.Equal(t, "b17", got.IdEmissions[0].Value)
	})

	t.Run("a nested row id counts too — the schema stopped showing that one as well", func(t *testing.T) {
		// given
		calls := []callRecord{
			call(0, "insert_blocks", `{"op":"insert_blocks","blocks":[{"type":"table","rows":[{"id":"r7","cells":["a"]}]}]}`),
		}

		// when
		got := analyze(calls)

		// then
		assert.Equal(t, 1, got.InsertBlocksWithId)
		require.Len(t, got.IdEmissions, 1)
		assert.Equal(t, "blocks[0].rows[0].id", got.IdEmissions[0].Path)
	})

	t.Run("replace_subtree is the control: the op's own id does not count, a payload id does", func(t *testing.T) {
		// given
		calls := []callRecord{
			call(0, "replace_subtree", `{"op":"replace_subtree","id":"d4e5f","blocks":[{"type":"paragraph","text":"x"}]}`),
			call(1, "replace_subtree", `{"op":"replace_subtree","id":"d4e5f","blocks":[{"id":"d4e5f","type":"paragraph","text":"x"}]}`),
		}

		// when
		got := analyze(calls)

		// then
		assert.Equal(t, 2, got.ReplaceSubtreeCalls)
		assert.Equal(t, 1, got.ReplaceSubtreeIds)
	})
}

func TestAnalyzeReferenceEcho(t *testing.T) {
	t.Run("classes grade an echo against what the last read served", func(t *testing.T) {
		// given
		read := callRecord{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1"}`), ResultText: servedDoc}
		calls := []callRecord{
			read,
			call(1, "edit_text", `{"object":"1","block":"d4e5f","find":"Q3","replace":"Q4"}`),
			call(2, "edit_text", `{"object":"1","block":"D4E5F","find":"a","replace":"b"}`),
			call(3, "edit_text", `{"object":"1","block":"4e5f","find":"a","replace":"b"}`),
			call(4, "edit_text", `{"object":"1","block":"zzzzz","find":"a","replace":"b"}`),
			call(5, "check_item", `{"object":"1","block":"2","checked":true}`),
		}
		want := []string{refExact, refCaseFold, refSuffix, refInvented, refHandleLike}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Refs, len(want))
		for i, class := range want {
			assert.Equal(t, class, got.Refs[i].Class, "ref %d (%q)", i, got.Refs[i].Value)
		}
	})

	t.Run("a reference written before any read is its own class", func(t *testing.T) {
		// given
		calls := []callRecord{call(0, "edit_text", `{"object":"1","block":"abcde","find":"a","replace":"b"}`)}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Refs, 1)
		assert.Equal(t, refNoRead, got.Refs[0].Class)
	})

	t.Run("an id from an earlier read that the latest one no longer serves is stale, not invented", func(t *testing.T) {
		// given
		second := `{"id":"obj1","blocks":[{"id":"a1b2c","type":"heading2","text":"Summary"}]}`
		calls := []callRecord{
			{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1"}`), ResultText: servedDoc},
			{Turn: 1, Tool: "read", Args: json.RawMessage(`{"object":"1"}`), ResultText: second},
			call(2, "delete_block", `{"object":"1","block":"d4e5f"}`),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Refs, 1)
		assert.Equal(t, refStale, got.Refs[0].Class)
	})

	t.Run("table row and column labels are references too", func(t *testing.T) {
		// given
		calls := []callRecord{
			{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1"}`), ResultText: servedDoc},
			call(1, "set_cell", `{"object":"1","table":"t9d2c","row":"r0022","col":"c0022","value":"Done"}`),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Refs, 3)
		for _, ref := range got.Refs {
			assert.Equal(t, refExact, ref.Class, "%s=%q", ref.Arg, ref.Value)
		}
	})
}

func TestAnalyzeRepairAfterRefusal(t *testing.T) {
	refused := func(turn int, tool, args, path, text string) callRecord {
		c := call(turn, tool, args)
		c.IsError = true
		c.ResultText = text
		c.Exchanges = []exchange{{
			Status: 400, Code: "invalid_input",
			Issues: []v2model.Issue{{Path: path, Message: text}},
		}}
		return c
	}

	t.Run("changing the field the error named is the repair we hope for", func(t *testing.T) {
		// given
		calls := []callRecord{
			refused(0, "edit_text", `{"object":"1","block":"zz","find":"Q3","replace":"Q4"}`, "block", "unknown block"),
			call(1, "edit_text", `{"object":"1","block":"d4e5f","find":"Q3","replace":"Q4"}`),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Repairs, 1)
		assert.Equal(t, repairFixedNamed, got.Repairs[0].Class)
		assert.Equal(t, "block", got.Repairs[0].NamedField)
		assert.Equal(t, 400, got.Repairs[0].Status)
		assert.True(t, got.Repairs[0].NextOK)
	})

	t.Run("an identical re-send is the loop the refusal was meant to prevent", func(t *testing.T) {
		// given
		args := `{"object":"1","block":"zz","find":"Q3","replace":"Q4"}`
		calls := []callRecord{
			refused(0, "edit_text", args, "block", "unknown block"),
			refused(1, "edit_text", args, "block", "unknown block"),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Repairs, 2)
		assert.Equal(t, repairIdentical, got.Repairs[0].Class)
		assert.Equal(t, repairAbandoned, got.Repairs[1].Class)
	})

	t.Run("reaching for a read after a refusal is its own class", func(t *testing.T) {
		// given
		calls := []callRecord{
			refused(0, "edit_text", `{"object":"1","find":"Q3","replace":"Q4"}`, "find", "no block contains"),
			call(1, "read", `{"object":"1","mode":"outline"}`),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Repairs, 1)
		assert.Equal(t, repairSwitchRead, got.Repairs[0].Class)
	})

	t.Run("an op-path issue names the field at its last segment", func(t *testing.T) {
		// given
		calls := []callRecord{
			refused(0, "insert_blocks", `{"op":"insert_blocks","blocks":[{"id":"x","type":"paragraph"}]}`,
				"ops[0].blocks[0].id", "id is not part of insert_blocks"),
			call(1, "insert_blocks", `{"op":"insert_blocks","blocks":[{"type":"paragraph"}]}`),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.Repairs, 1)
		assert.Equal(t, "id", got.Repairs[0].NamedField)
		// the id moved out of a nested payload — a top-level compare cannot
		// see it, so the honest class is "changed something else"
		assert.Equal(t, repairChangedElse, got.Repairs[0].Class)
	})
}

func TestAnalyzeCountsMalformedArguments(t *testing.T) {
	// given
	calls := []callRecord{{Turn: 0, Tool: "read", ArgsError: "invalid character 'x'"}}

	// when
	got := analyze(calls)

	// then
	assert.Equal(t, 1, got.MalformedArgs)
	assert.Empty(t, got.Refs)
}

func TestReferenceOrderIsDeterministic(t *testing.T) {
	// given — one call carrying three references; map iteration would
	// reorder them between runs, and a record that reorders itself is not a
	// record
	calls := []callRecord{
		{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1"}`), ResultText: servedDoc},
		call(1, "set_cell", `{"object":"1","table":"t9d2c","row":"r0022","col":"c0022","value":"Done"}`),
	}

	// when
	first := analyze(calls)

	// then
	for i := 0; i < 20; i++ {
		again := analyze(calls)
		require.Equal(t, len(first.Refs), len(again.Refs))
		for j := range first.Refs {
			assert.Equal(t, first.Refs[j].Arg, again.Refs[j].Arg, "ref %d reordered on run %d", j, i)
		}
	}
	assert.Equal(t, []string{"col", "row", "table"}, []string{first.Refs[0].Arg, first.Refs[1].Arg, first.Refs[2].Arg})
}

func TestAnalyzeCountsTheOpDiscriminator(t *testing.T) {
	// given — the ops arm sets `op` from the tool name, so a model that
	// omits it or writes a positional word into it never fails for that;
	// the reading skill is still worth a number
	calls := []callRecord{
		call(0, "insert_blocks", `{"op":"insert_blocks","markdown":"## Risks"}`),
		call(1, "insert_blocks", `{"markdown":"## Risks"}`),
		call(2, "insert_blocks", `{"op":"last","markdown":"## Risks"}`),
		call(3, "edit_text", `{"object":"1","find":"a","replace":"b"}`),
	}

	// when
	got := analyze(calls)

	// then
	assert.Equal(t, 1, got.OpConstAbsent)
	assert.Equal(t, 1, got.OpConstWrong, "a wrapper tool without an op field must not be counted")
}

func TestAnalyzeCountsTheOptionalBlockAndTheReadsItCost(t *testing.T) {
	t.Run("the shape gemma4:e2b produced on 3 of 3 attempts: outline, full, edit with block", func(t *testing.T) {
		// given
		calls := []callRecord{
			{Turn: 0, Tool: "find", Args: json.RawMessage(`{"space":"s","query":"Zafuriko"}`),
				ResultText: "1. Zafuriko (page)\n1 matches"},
			{Turn: 1, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"outline"}`), ResultText: servedDoc},
			{Turn: 2, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"full"}`), ResultText: servedDoc},
			call(3, "edit_text", `{"object":"1","block":"d4e5f","find":"Q3","replace":"Q4"}`),
		}

		// when
		got := analyze(calls)

		// then
		assert.Equal(t, 1, got.EditTextCalls)
		assert.Equal(t, 1, got.EditTextWithBlock)
		require.Len(t, got.WastedReads, 1, "the outline was superseded by the full read")
		assert.Equal(t, wasteOutlineThenFull, got.WastedReads[0].Kind)
		assert.Equal(t, 1, got.FindCalls)
		assert.Equal(t, 0, got.FindMultiMatch)
		assert.Equal(t, 1, got.MaxFindMatches)
	})

	t.Run("a full read before a snippet-only edit is the read the A/B tries to remove", func(t *testing.T) {
		// given
		calls := []callRecord{
			{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"outline"}`), ResultText: servedDoc},
			{Turn: 1, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"full"}`), ResultText: servedDoc},
			call(2, "edit_text", `{"object":"1","find":"Q3","replace":"Q4"}`),
		}

		// when
		got := analyze(calls)

		// then
		require.Len(t, got.WastedReads, 2)
		assert.Equal(t, wasteOutlineThenFull, got.WastedReads[0].Kind)
		assert.Equal(t, wasteReadBeforeSnippetEdit, got.WastedReads[1].Kind)
		assert.Equal(t, 0, got.EditTextWithBlock)
	})

	t.Run("a read that fed an earlier write is not waste", func(t *testing.T) {
		// given — the read served the add_blocks anchor; the later snippet
		// edit did not need it
		calls := []callRecord{
			{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"full"}`), ResultText: servedDoc},
			call(1, "add_blocks", `{"object":"1","after":"d4e5f","markdown":"## Risks"}`),
			call(2, "edit_text", `{"object":"1","find":"Q3","replace":"Q4"}`),
		}

		// when
		got := analyze(calls)

		// then
		assert.Empty(t, got.WastedReads)
	})

	t.Run("find matches are counted so a contaminated run says so", func(t *testing.T) {
		// given — what the shared-stem fixtures produced: one more leftover
		// match on every attempt of a run
		calls := []callRecord{
			{Turn: 0, Tool: "find", Args: json.RawMessage(`{"space":"s","query":"Quarterly plan 84353d"}`),
				ResultText: "1. Quarterly plan 84353d (page)\n2. Quarterly plan 1425d9 (page)\n3 matches"},
		}

		// when
		got := analyze(calls)

		// then
		assert.Equal(t, 1, got.FindCalls)
		assert.Equal(t, 1, got.FindMultiMatch)
		assert.Equal(t, 3, got.MaxFindMatches)
	})
}

func TestAnalyzeCountsIdsWrittenIntoTheObjectArgument(t *testing.T) {
	// given — the two ways a model puts an id in the one slot its tool
	// offers: a block label served by a read (seen the first time an arm
	// published no block argument), and the space id (seen when the harness
	// named one without naming its argument)
	const spaceId = "bafyreispace.abc"
	calls := []callRecord{
		{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"` + spaceId + `","mode":"full"}`),
			ResultText: "no working session for object", IsError: true},
		{Turn: 1, Tool: "find", Args: json.RawMessage(`{"space":"` + spaceId + `","query":"Zafuriko"}`),
			ResultText: "1. Zafuriko (page)\n1 matches"},
		{Turn: 2, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"full"}`), ResultText: servedDoc},
		call(3, "edit_text", `{"object":"d4e5f","find":"Q3","replace":"Q4"}`),
	}

	// when
	got := analyze(calls)

	// then
	assert.Equal(t, 1, got.ObjectArgIsBlockRef, "d4e5f is a block the read served, not an object")
	assert.Equal(t, 1, countSpaceIdAsObject(calls, spaceId))
	assert.Equal(t, 0, countSpaceIdAsObject(calls, "some-other-space"))
}

func TestWastedReadsDoNotDependOnTheEditSucceeding(t *testing.T) {
	// given — the shape arm B1 produced live: the model read anyway, had no
	// block argument to put the label in, and put it in `object` instead.
	// Counting only successful edits would hide that read on exactly the arm
	// the experiment is trying to measure.
	calls := []callRecord{
		{Turn: 0, Tool: "read", Args: json.RawMessage(`{"object":"1","mode":"full"}`), ResultText: servedDoc},
		{Turn: 1, Tool: "edit_text", Args: json.RawMessage(`{"object":"d4e5f","find":"Q3","replace":"Q4"}`),
			ResultText: `object "d4e5f" not found in space "s1"`, IsError: true},
	}

	// when
	got := analyze(calls)

	// then
	require.Len(t, got.WastedReads, 1)
	assert.Equal(t, wasteReadBeforeSnippetEdit, got.WastedReads[0].Kind)
	assert.Equal(t, 1, got.ObjectArgIsBlockRef)
}
