package snapshotdiff

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func str(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func num(n float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: n}}
}

func fields(kv map[string]*types.Value) *types.Struct {
	return &types.Struct{Fields: kv}
}

func textBlock(id, text string) *model.Block {
	return &model.Block{Id: id, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{Text: text},
	}}
}

func snapshot(details map[string]*types.Value, blocks ...*model.Block) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{Details: fields(details), Blocks: blocks}
}

func TestCompare(t *testing.T) {
	t.Run("identical snapshots have no drift", func(t *testing.T) {
		// given
		a := snapshot(map[string]*types.Value{"name": str("Doc")}, textBlock("b1", "hello"))
		b := snapshot(map[string]*types.Value{"name": str("Doc")}, textBlock("b1", "hello"))

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		assert.Empty(t, got)
	})

	t.Run("lost text block is reported", func(t *testing.T) {
		// given
		a := snapshot(map[string]*types.Value{}, textBlock("b1", "hello"), textBlock("b2", "world"))
		b := snapshot(map[string]*types.Value{}, textBlock("b1", "hello"))

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		require.Len(t, got, 1)
		assert.Contains(t, got[0], `text block lost (1x): "world"`)
	})

	t.Run("changed detail is reported", func(t *testing.T) {
		// given
		a := snapshot(map[string]*types.Value{"name": str("Doc")})
		b := snapshot(map[string]*types.Value{"name": str("Renamed")})

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		require.Len(t, got, 1)
		assert.Contains(t, got[0], `detail "name" changed`)
	})

	t.Run("added detail is reported", func(t *testing.T) {
		// given: the round trip introduced a detail absent from the original
		a := snapshot(map[string]*types.Value{"name": str("Doc")})
		b := snapshot(map[string]*types.Value{"name": str("Doc"), "description": str("new")})

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		require.Len(t, got, 1)
		assert.Contains(t, got[0], `detail "description" added`)
	})

	t.Run("date sub-second truncation is not drift", func(t *testing.T) {
		// given: dueDate is a bundled date relation
		a := snapshot(map[string]*types.Value{"dueDate": num(1700000000.7)})
		b := snapshot(map[string]*types.Value{"dueDate": num(1700000000)})

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		assert.Empty(t, got)
	})

	t.Run("stripped local keys are ignored", func(t *testing.T) {
		// given: lastOpenedDate is local-only, stripped on export by design
		a := snapshot(map[string]*types.Value{"lastOpenedDate": num(1)})
		b := snapshot(map[string]*types.Value{})

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		assert.Empty(t, got)
	})

	t.Run("moved text is not drift, the inventory is a multiset", func(t *testing.T) {
		// given
		a := snapshot(nil, textBlock("b1", "one"), textBlock("b2", "two"))
		b := snapshot(nil, textBlock("x9", "two"), textBlock("x8", "one"))

		// when
		got := Compare(a, b, anyblockjson.Options{})

		// then
		assert.Empty(t, got)
	})
}

func TestTextInventory(t *testing.T) {
	// given: title/description styles and emoji-marked blocks are excluded
	title := &model.Block{Id: "t", Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{Text: "Title", Style: model.BlockContentText_Title},
	}}
	emoji := &model.Block{Id: "e", Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{Text: "hi :)", Marks: &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{{Type: model.BlockContentTextMark_Emoji}},
		}},
	}}
	s := snapshot(nil, title, emoji, textBlock("b1", "kept"), textBlock("b2", "kept"))

	// when
	inv := TextInventory(s)

	// then
	assert.Equal(t, map[string]int{"kept": 2}, inv)
}
