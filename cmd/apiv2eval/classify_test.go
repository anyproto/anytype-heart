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
	t.Run("an id anywhere in an insertBlocks payload is counted, with its path", func(t *testing.T) {
		// given
		calls := []callRecord{
			call(0, "insertBlocks", `{"op":"insertBlocks","blocks":[{"id":"b17","type":"paragraph","text":"hi"}]}`),
			call(1, "insertBlocks", `{"op":"insertBlocks","markdown":"## Risks"}`),
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
			call(0, "insertBlocks", `{"op":"insertBlocks","blocks":[{"type":"table","rows":[{"id":"r7","cells":["a"]}]}]}`),
		}

		// when
		got := analyze(calls)

		// then
		assert.Equal(t, 1, got.InsertBlocksWithId)
		require.Len(t, got.IdEmissions, 1)
		assert.Equal(t, "blocks[0].rows[0].id", got.IdEmissions[0].Path)
	})

	t.Run("replaceSubtree is the control: the op's own id does not count, a payload id does", func(t *testing.T) {
		// given
		calls := []callRecord{
			call(0, "replaceSubtree", `{"op":"replaceSubtree","id":"d4e5f","blocks":[{"type":"paragraph","text":"x"}]}`),
			call(1, "replaceSubtree", `{"op":"replaceSubtree","id":"d4e5f","blocks":[{"id":"d4e5f","type":"paragraph","text":"x"}]}`),
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
			refused(0, "insertBlocks", `{"op":"insertBlocks","blocks":[{"id":"x","type":"paragraph"}]}`,
				"ops[0].blocks[0].id", "id is not part of insertBlocks"),
			call(1, "insertBlocks", `{"op":"insertBlocks","blocks":[{"type":"paragraph"}]}`),
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
