package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestV2CrossSpaceObjectLinksExpandShortSpaceRefs: the API serves space ids
// short (§8.35) and accepts the short form anywhere a space id is accepted
// — a cross-space object link a caller writes with a served reference must
// reach the store with the full id, or the client cannot open it.
func TestV2CrossSpaceObjectLinksExpandShortSpaceRefs(t *testing.T) {
	resolve := func(ref string) string {
		if ref == "hxwz2i" {
			return realSpaceEval
		}
		return ref
	}

	t.Run("the spaceId parameter is rewritten, everything else kept", func(t *testing.T) {
		got := expandSpaceRefInObjectLink("anytype://object?objectId=bafyreiobj&spaceId=hxwz2i", resolve)
		assert.Equal(t, "anytype://object?objectId=bafyreiobj&spaceId="+realSpaceEval, got)
	})

	t.Run("a full id, a same-space link and an ordinary URL pass untouched", func(t *testing.T) {
		for _, dest := range []string{
			"anytype://object?objectId=bafyreiobj&spaceId=" + realSpaceEval,
			"anytype://object?objectId=bafyreiobj",
			"https://example.com/?spaceId=hxwz2i",
			"anytype://object?objectId=bafyreiobj&spaceId=nosuch",
		} {
			assert.Equal(t, dest, expandSpaceRefInObjectLink(dest, resolve), dest)
		}
	})

	t.Run("only Link marks on text blocks are rewritten", func(t *testing.T) {
		link := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Link, Param: "anytype://object?objectId=bafyreiobj&spaceId=hxwz2i"}
		object := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Object, Param: "hxwz2i"}
		blocks := []*model.Block{
			{Id: "root", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "p", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "see", Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{link, object}}}}},
		}

		expandSpaceRefsInBlocks(blocks, resolve)

		assert.Equal(t, "anytype://object?objectId=bafyreiobj&spaceId="+realSpaceEval, link.Param)
		assert.Equal(t, "hxwz2i", object.Param, "an Object mark's param is an object id, never a space")
	})

	t.Run("the service expander resolves against the caller's visible spaces", func(t *testing.T) {
		fx := newV2FixtureBare(t)
		for i, id := range realSpaceIds() {
			fx.registerSpaceView(t, id, "Space "+string(rune('A'+i)), "")
		}

		expand := fx.spaceRefExpander(context.Background())

		assert.Equal(t, realSpaceEval, expand("hxwz2i"))
		assert.Equal(t, realSpaceEval, expand(realSpaceEval), "a full id needs no lookup")
		assert.Equal(t, "nosuch", expand("nosuch"), "an unknown reference stays verbatim")
	})

	t.Run("a created document's link carries the full id", func(t *testing.T) {
		fx := newV2Fixture(t)
		for i, id := range realSpaceIds() {
			fx.registerSpaceView(t, id, "Space "+string(rune('A'+i)), "")
		}
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		_, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Doc"},"blocks":[{"type":"paragraph","text":"see [the deal](anytype://object?objectId=bafyreiobj&spaceId=hxwz2i)"}]}`), false, true)

		require.NoError(t, err)
		marks := (*captured).Blocks[1].GetText().GetMarks().GetMarks()
		require.Len(t, marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Link, marks[0].Type)
		assert.Equal(t, "anytype://object?objectId=bafyreiobj&spaceId="+realSpaceEval, marks[0].Param)
	})
}
