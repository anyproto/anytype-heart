package anyblockjson

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func rawRun(blocks ...string) []json.RawMessage {
	out := make([]json.RawMessage, len(blocks))
	for i, b := range blocks {
		out[i] = json.RawMessage(b)
	}
	return out
}

func TestUnmarshalBlocks(t *testing.T) {
	t.Run("run with relative indents builds the subtree", func(t *testing.T) {
		// given
		run := rawRun(
			`{"id":"a1","type":"paragraph","text":"parent"}`,
			`{"indent":1,"id":"a2","type":"paragraph","text":"child"}`,
			`{"id":"a3","type":"quote","text":"sibling"}`,
		)

		// when
		blocks, topIds, err := UnmarshalBlocks(run, Options{})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"a1", "a3"}, topIds)
		require.Len(t, blocks, 3)
		assert.Equal(t, []string{"a2"}, blocks[0].ChildrenIds, "indent 1 nests under the predecessor")
	})

	t.Run("missing ids are generated", func(t *testing.T) {
		n := 0
		opts := Options{GenerateId: func() string { n++; return "gen" + string(rune('0'+n)) }}

		blocks, topIds, err := UnmarshalBlocks(rawRun(`{"type":"paragraph","text":"x"}`), opts)

		require.NoError(t, err)
		require.Len(t, blocks, 1)
		assert.Equal(t, "gen1", blocks[0].Id)
		assert.Equal(t, []string{"gen1"}, topIds)
	})

	t.Run("V1 monotonicity applies to the run", func(t *testing.T) {
		_, _, err := UnmarshalBlocks(rawRun(
			`{"type":"paragraph","text":"a"}`,
			`{"indent":2,"type":"paragraph","text":"b"}`,
		), Options{})

		var verr *ValidationError
		require.ErrorAs(t, err, &verr)
	})

	t.Run("structural block types are rejected explicitly", func(t *testing.T) {
		for typ, body := range map[string]string{
			"title":               `{"type":"title","text":"x"}`,
			"description":         `{"type":"description","text":"x"}`,
			"featured_properties": `{"type":"featured_properties"}`,
		} {
			_, _, err := UnmarshalBlocks(rawRun(body), Options{})

			var verr *ValidationError
			require.ErrorAs(t, err, &verr, typ)
			require.Len(t, verr.Issues, 1)
			assert.Equal(t, "/blocks/0/type", verr.Issues[0].Path)
			assert.Contains(t, verr.Issues[0].Message, "structural block")
		}
	})

	t.Run("no primary-dataview pinning in fragments", func(t *testing.T) {
		// a whole-document import would rename this block to the fixed
		// "dataview" id (§7); a fragment must not
		blocks, _, err := UnmarshalBlocks(rawRun(`{"type":"dataview","views":[{"name":"All"}]}`),
			Options{GenerateId: func() string { return "genDataview1" }})

		require.NoError(t, err)
		require.NotEmpty(t, blocks)
		assert.Equal(t, "genDataview1", blocks[0].Id, "the fragment dataview keeps a generated id")
	})

	t.Run("table run carries its internal subtree", func(t *testing.T) {
		blocks, topIds, err := UnmarshalBlocks(rawRun(
			`{"id":"tbl1","type":"table","columns":[{"id":"colA"}],"rows":[{"id":"rowA","cells":["hi"]}]}`,
		), Options{})

		require.NoError(t, err)
		assert.Equal(t, []string{"tbl1"}, topIds)
		ids := make(map[string]bool, len(blocks))
		for _, b := range blocks {
			ids[b.Id] = true
		}
		assert.True(t, ids["colA"], "column block present")
		assert.True(t, ids["rowA"], "row block present")
		assert.True(t, ids["rowA-colA"], "derived cell id present")
	})
}

func TestUnmarshalBlock(t *testing.T) {
	t.Run("forcedId overrides the payload id", func(t *testing.T) {
		blocks, err := UnmarshalBlock(json.RawMessage(`{"type":"checkbox","checked":true,"text":"todo"}`), "keepMe1", Options{})

		require.NoError(t, err)
		require.Len(t, blocks, 1)
		assert.Equal(t, "keepMe1", blocks[0].Id)
		assert.True(t, blocks[0].GetText().Checked)
	})

	t.Run("round-trips through MarshalBlockSubtree", func(t *testing.T) {
		// given
		src := json.RawMessage(`{"id":"b1","type":"quote","text":"a **bold** word","color":"red"}`)
		blocks, err := UnmarshalBlock(src, "", Options{})
		require.NoError(t, err)

		// when
		out, err := MarshalBlockSubtree(blocks, Options{})

		// then
		require.NoError(t, err)
		arr := fragmentBlocks(t, out)
		require.Len(t, arr, 1)
		assert.Equal(t, "quote", arr[0]["type"])
		assert.Equal(t, "a **bold** word", arr[0]["text"], "inline markup survives the round-trip")
		assert.Equal(t, "red", arr[0]["color"])
	})

	t.Run("invalid shape fails the §5 checks", func(t *testing.T) {
		_, err := UnmarshalBlock(json.RawMessage(`{"type":"nonsense"}`), "", Options{})

		var verr *ValidationError
		require.ErrorAs(t, err, &verr)
	})
}

func TestMarshalBlockSubtree(t *testing.T) {
	t.Run("subtree renders as a flat run with depths", func(t *testing.T) {
		// given
		subtree := []*model.Block{
			{Id: "p1", ChildrenIds: []string{"p2"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "parent"}}},
			{Id: "p2",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "child"}}},
		}

		// when
		out, err := MarshalBlockSubtree(subtree, Options{})

		// then
		require.NoError(t, err)
		arr := fragmentBlocks(t, out)
		require.Len(t, arr, 2)
		assert.Equal(t, "p1", arr[0]["id"])
		assert.Nil(t, arr[0]["indent"])
		assert.Equal(t, float64(1), arr[1]["indent"])
	})

	t.Run("empty subtree errors", func(t *testing.T) {
		_, err := MarshalBlockSubtree(nil, Options{})
		require.Error(t, err)
	})
}

// fragmentLabelSubtree is two minted block ids, parent and child: the only
// shape doc-local relabeling touches (isMintedLocalId).
func fragmentLabelSubtree() []*model.Block {
	return []*model.Block{
		{Id: "64b2c1d2e3f4a5b6c7d8e9f0", ChildrenIds: []string{"1111111111111111111a1b2c"},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "parent"}}},
		{Id: "1111111111111111111a1b2c",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "child"}}},
	}
}

// fragmentBlocks digs the `blocks` run out of a fragment envelope.
func fragmentBlocks(t *testing.T, out json.RawMessage) []map[string]any {
	t.Helper()
	var env struct {
		Blocks []map[string]any `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(out, &env))
	return env.Blocks
}

// fragmentLegend digs the three legends out of a fragment envelope.
func fragmentLegend(t *testing.T, out json.RawMessage) Legend {
	t.Helper()
	var env struct {
		PropertyKeys map[string]string            `json:"property_internal_keys"`
		TypeKeys     map[string]string            `json:"type_internal_keys"`
		OptionIds    map[string]map[string]string `json:"option_ids"`
	}
	require.NoError(t, json.Unmarshal(out, &env))
	return Legend{PropertyKeys: env.PropertyKeys, TypeKeys: env.TypeKeys, OptionIds: env.OptionIds}
}

func fragmentIds(t *testing.T, out json.RawMessage) []string {
	t.Helper()
	blocks := fragmentBlocks(t, out)
	ids := make([]string, len(blocks))
	for i, b := range blocks {
		ids[i], _ = b["id"].(string)
	}
	return ids
}

// A fragment is addressed by the ids it carries, so the two options that take
// its addresses away are REFUSED rather than honoured.
//
// This surface exists for wiring that edits a live document op-by-op. OmitIds
// drops every block id, the view id and the filter id, so the run says what
// to write and not where. Block-label compaction rewrites doc-local ids to
// short suffixes that are local to the emitted run and are not the object's
// ids at all — the fragment used to hand back `8e9f0` for the block stored as
// `64b2c1d2e3f4a5b6c7d8e9f0`. Both produced a fragment that reads correctly
// and cannot be applied, which is the failure this format refuses to make
// silently.
func TestMarshalBlockSubtree_RefusesTheOptionsThatTakeAwayItsAddresses(t *testing.T) {
	for name, tc := range map[string]struct {
		opts Options
		want string
	}{
		"OmitIds":            {Options{OmitIds: true}, "a fragment is addressed by the ids it carries"},
		"CompactBlockLabels": {Options{CompactBlockLabels: true}, "not the object's ids"},
		"CompactIds":         {Options{CompactIds: true}, "not the object's ids"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := MarshalBlockSubtree(fragmentLabelSubtree(), tc.opts)
			require.Error(t, err, "emitted:\n%s", out)
			assert.Contains(t, err.Error(), tc.want)
			assert.Nil(t, out, "a refused fragment hands back nothing")
		})
	}

	t.Run("without them every id is the object's own", func(t *testing.T) {
		out, err := MarshalBlockSubtree(fragmentLabelSubtree(), Options{})
		require.NoError(t, err)
		assert.Equal(t, []string{"64b2c1d2e3f4a5b6c7d8e9f0", "1111111111111111111a1b2c"},
			fragmentIds(t, out))
	})
}

// A fragment has no envelope, so it can carry no legend — which is why the
// only compaction it may do is the legend-less one. Object references are
// therefore written in full here under EVERY option, exactly as in a whole
// document (§9a).
//
// This is the surviving, testable half of what 42396b448 fixed in passing:
// the fragment used to build its plan under the object-ref flag too, and
// emitted short object labels into an array that had nowhere to define them.
// That flag is deleted, so the bug is gone by construction and cannot be
// reproduced; what can be pinned is the property that made it a bug, and this
// fails the moment any legend-backed compaction reaches the fragment path
// again.
func TestMarshalBlockSubtree_ObjectRefsAreNeverCompacted(t *testing.T) {
	const target = "bafyreimentiontargetidxxx"
	subtree := []*model.Block{{Id: "64b2c1d2e3f4a5b6c7d8e9f0",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: "ping Roman", Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{{
					Range: &model.Range{From: 5, To: 10},
					Type:  model.BlockContentTextMark_Mention, Param: target}}}}}}}

	out, err := MarshalBlockSubtree(subtree, Options{})
	require.NoError(t, err)
	assert.Contains(t, string(out), `object_id=\"`+target+`\"`,
		"the mention target must be spelled in full — a fragment defines no object labels")
	assert.NotContains(t, string(out), `object_id=\"idxxx\"`)
}

func TestUnmarshalPropertyValue(t *testing.T) {
	t.Run("date strings parse per §3", func(t *testing.T) {
		v := UnmarshalPropertyValue("dueDate", "2026-07-30", Options{})

		require.NotNil(t, v)
		assert.NotZero(t, v.GetNumberValue(), "a date resolves to epoch seconds")
	})

	t.Run("round-trips with MarshalPropertyValue", func(t *testing.T) {
		v := UnmarshalPropertyValue("dueDate", "2026-07-30T00:00:00Z", Options{})
		back, _ := MarshalPropertyValue("dueDate", v, Options{})
		assert.Equal(t, "2026-07-30T00:00:00Z", back)
	})

	t.Run("nil keeps presence as an explicit null", func(t *testing.T) {
		v := UnmarshalPropertyValue("anything", nil, Options{})
		require.NotNil(t, v)
		assert.IsType(t, &types.Value_NullValue{}, v.GetKind())
	})
}

func TestInlineTextCodec(t *testing.T) {
	t.Run("parse and render invert each other", func(t *testing.T) {
		// given
		md := "plain **bold** and *italic* text"

		// when
		text, marks, err := ParseInlineText(md)

		// then
		require.NoError(t, err)
		assert.Equal(t, "plain bold and italic text", text)
		require.Len(t, marks, 2)
		assert.Equal(t, md, RenderInlineText(text, marks))
	})

	t.Run("render escapes markup characters", func(t *testing.T) {
		rendered := RenderInlineText("2*3*4", nil)
		text, marks, err := ParseInlineText(rendered)
		require.NoError(t, err)
		assert.Equal(t, "2*3*4", text)
		assert.Empty(t, marks)
	})
}
