package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSalvageToolCalls(t *testing.T) {
	t.Run("recovers the shape the host actually dropped", func(t *testing.T) {
		// given: reasoning_content copied verbatim from a captured LM Studio
		// response whose tool_calls arrived empty with finish_reason "stop"
		reasoning := "The outline doesn't show the Q3/Q4 content. Let me read in full mode.\n\n" +
			"<tool_call>\n<function=read>\n<parameter=object>\n1\n</parameter>\n" +
			"<parameter=space>\nhvkwsi\n</parameter>\n<parameter=mode>\nfull\n</parameter>\n</function>\n</tool_call>"
		want := map[string]any{"object": "1", "space": "hvkwsi", "mode": "full"}

		// when
		got := salvageToolCalls(reasoning, "")

		// then
		require.Len(t, got, 1)
		assert.Equal(t, "read", got[0].Function.Name)
		// arguments must be a JSON STRING on the wire — an object here is
		// refused when the assistant message is echoed on the next turn
		var encoded string
		require.NoError(t, json.Unmarshal(got[0].Function.Arguments, &encoded),
			"arguments serialize as a JSON string, the shape a real call has")
		var args map[string]any
		require.NoError(t, json.Unmarshal([]byte(encoded), &args))
		assert.Equal(t, want, args, "parameter values are trimmed of the template's newlines")
	})

	t.Run("several blocks yield only the last — the settled-on call", func(t *testing.T) {
		// given: a deliberation that names one block then another. Executing
		// both would act on a plan the model had already moved past.
		text := "<tool_call><function=delete_block><parameter=block>a1</parameter></function></tool_call>" +
			"<tool_call><function=delete_block><parameter=block>b2</parameter></function></tool_call>"

		// when
		got := salvageToolCalls(text)

		// then
		require.Len(t, got, 1)
		var encoded string
		require.NoError(t, json.Unmarshal(got[0].Function.Arguments, &encoded))
		assert.Contains(t, encoded, "b2", "the last block wins")
	})

	t.Run("ordinary prose salvages nothing", func(t *testing.T) {
		// given: thinking that merely TALKS about calling a tool
		text := "I should call read on the object, then edit_text to change Q3."

		// when
		got := salvageToolCalls(text)

		// then
		assert.Nil(t, got, "nil distinguishes 'nothing to recover' from 'recovered nothing'")
	})

	t.Run("a call with no parameters is still a call", func(t *testing.T) {
		// given
		text := "<tool_call><function=read_object></function></tool_call>"

		// when
		got := salvageToolCalls(text)

		// then
		require.Len(t, got, 1)
		assert.Equal(t, "read_object", got[0].Function.Name)
		assert.JSONEq(t, `"{}"`, string(got[0].Function.Arguments))
	})
}

func TestSalvageRefusesWhatTheModelDidNotMean(t *testing.T) {
	t.Run("a call the model then argued itself out of is not executed", func(t *testing.T) {
		// given: the shape LM Studio's own parser got wrong (#1592)
		text := "<tool_call><function=delete_block><parameter=block>a1</parameter></function></tool_call>\n" +
			"Actually, I should read the document first before deleting anything."

		// when
		got := salvageToolCalls(text)

		// then
		assert.Nil(t, got, "an unintended write is worse than a wasted turn")
	})

	t.Run("only the last call in a deliberation is taken", func(t *testing.T) {
		// given: the model considers one call, then settles on another
		text := "<tool_call><function=delete_block><parameter=block>a1</parameter></function></tool_call>" +
			"<tool_call><function=edit_text><parameter=find>Q3</parameter></function></tool_call>"

		// when
		got := salvageToolCalls(text)

		// then
		require.Len(t, got, 1)
		assert.Equal(t, "edit_text", got[0].Function.Name, "the settled-on call, not the discarded one")
	})

	t.Run("salvageable requires all three signals of a real drop", func(t *testing.T) {
		drop := &chatResponse{FinishReason: "stop"}
		assert.True(t, salvageable(drop))

		spoke := &chatResponse{FinishReason: "stop"}
		spoke.Message.Content = "I have finished the edit."
		assert.False(t, salvageable(spoke), "a model that answered in prose did not lose a call")

		yielded := &chatResponse{FinishReason: "tool_calls"}
		assert.False(t, salvageable(yielded), "finish_reason tool_calls means the channel worked")
	})
}
