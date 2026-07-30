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
			"title":              `{"type":"title","text":"x"}`,
			"description":        `{"type":"description","text":"x"}`,
			"featuredProperties": `{"type":"featuredProperties"}`,
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
		var arr []map[string]any
		require.NoError(t, json.Unmarshal(out, &arr))
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
		var arr []map[string]any
		require.NoError(t, json.Unmarshal(out, &arr))
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

func TestUnmarshalPropertyValue(t *testing.T) {
	t.Run("date strings parse per §3", func(t *testing.T) {
		v := UnmarshalPropertyValue("dueDate", "2026-07-30", Options{})

		require.NotNil(t, v)
		assert.NotZero(t, v.GetNumberValue(), "a date resolves to epoch seconds")
	})

	t.Run("round-trips with MarshalPropertyValue", func(t *testing.T) {
		v := UnmarshalPropertyValue("dueDate", "2026-07-30T00:00:00Z", Options{})
		back := MarshalPropertyValue("dueDate", v, Options{})
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
