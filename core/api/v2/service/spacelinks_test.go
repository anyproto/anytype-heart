package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestV2CrossSpaceObjectLinksExpandShortSpaceRefs: the API serves space ids
// short (§8.35) and accepts the short form anywhere a space id is accepted
// — a cross-space object link a caller writes with a served reference must
// reach the store with the full id, or the client cannot open it.
func TestV2CrossSpaceObjectLinksExpandShortSpaceRefs(t *testing.T) {
	// a fixture whose visible spaces are the real-shaped ids, so a short
	// reference resolves exactly as it does in production
	census := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		for i, id := range realSpaceIds() {
			fx.registerSpaceView(t, id, "Space "+string(rune('A'+i)), "")
		}
		return fx
	}
	link := func(space string) string {
		return "anytype://object?objectId=bafyreiobj&spaceId=" + space
	}

	t.Run("the spaceId parameter is rewritten, everything else kept", func(t *testing.T) {
		fx := census(t)
		e := fx.newSpaceLinkExpander(context.Background())

		assert.Equal(t, link(realSpaceEval), e.destination(link("hxwz2i")))
		assert.Empty(t, e.Warnings("/blocks"))
	})

	t.Run("a full id, a same-space link and an ordinary URL pass untouched", func(t *testing.T) {
		fx := census(t)
		e := fx.newSpaceLinkExpander(context.Background())

		for _, dest := range []string{
			link(realSpaceEval),
			"anytype://object?objectId=bafyreiobj",
			"https://example.com/?spaceId=hxwz2i",
			"anytype://object?objectId=bafyreiobj&spaceId=hxwz2i&spaceId=hxwz2i",
			"https://example.com/anytype://object?objectId=x&spaceId=hxwz2i",
		} {
			assert.Equal(t, dest, e.destination(dest), dest)
		}
		assert.Empty(t, e.Warnings("/blocks"), "nothing above is an unresolvable reference")
	})

	t.Run("only Link marks on text blocks are rewritten", func(t *testing.T) {
		fx := census(t)
		e := fx.newSpaceLinkExpander(context.Background())
		linkMark := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Link, Param: link("hxwz2i")}
		object := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Object, Param: "hxwz2i"}
		mention := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Mention, Param: "hxwz2i"}
		blocks := []*model.Block{
			{Id: "root", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "p", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "see", Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{linkMark, object, mention}}}}},
		}

		e.Blocks(blocks)

		assert.Equal(t, link(realSpaceEval), linkMark.Param)
		assert.Equal(t, "hxwz2i", object.Param, "an Object mark's param is an object id, never a space")
		assert.Equal(t, "hxwz2i", mention.Param)
	})

	t.Run("one census read per distinct reference, however many links carry it", func(t *testing.T) {
		fx := census(t)
		e := fx.newSpaceLinkExpander(context.Background())

		for i := 0; i < 50; i++ {
			assert.Equal(t, link(realSpaceEval), e.destination(link("hxwz2i")))
		}

		assert.Len(t, e.resolved, 1, "the reference is resolved once and memoized")
	})

	t.Run("a reference that names no visible space keeps the link and warns once", func(t *testing.T) {
		fx := census(t)
		e := fx.newSpaceLinkExpander(context.Background())

		assert.Equal(t, link("nosuch"), e.destination(link("nosuch")))
		assert.Equal(t, link("nosuch"), e.destination(link("nosuch")))

		warnings := e.Warnings("/blocks")
		require.Len(t, warnings, 1, "one warning per reference, not per link")
		assert.Equal(t, "/blocks", warnings[0].Path)
		assert.Contains(t, warnings[0].Message, `"nosuch"`)
		assert.Contains(t, warnings[0].Message, "will not open")
		assert.Equal(t, []v2model.Ref{v2model.RefListSpaces()}, warnings[0].SeeAlso)
	})

	t.Run("an ambiguous reference keeps the link and carries the candidates", func(t *testing.T) {
		fx := newV2Fixture(t)
		// two visible spaces whose tails collide, so the suffix is ambiguous
		fx.registerSpaceView(t, realSpacePersonal, "Space A", "")
		fx.registerSpaceView(t, twinPersonal, "Space B", "")
		e := fx.newSpaceLinkExpander(context.Background())

		assert.Equal(t, link(realSpacePersonalShort), e.destination(link(realSpacePersonalShort)))

		warnings := e.Warnings("/blocks")
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "ambiguous")
		assert.Contains(t, warnings[0].Message, "candidates:")
	})

	t.Run("expansion never pushes a destination past what the renderer can write", func(t *testing.T) {
		fx := census(t)
		e := fx.newSpaceLinkExpander(context.Background())
		// a destination that parses (under the 2048 bound as spelled) but
		// cannot survive the ~66 characters the full id adds
		long := "anytype://object?objectId=bafyreiobj&spaceId=hxwz2i&pad=" + strings.Repeat("a", 2020)

		got := e.destination(long)

		assert.Equal(t, long, got, "kept as sent rather than stored unwritable")
		warnings := e.Warnings("/blocks")
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "too long to write back")
	})

	t.Run("a created document's link carries the full id", func(t *testing.T) {
		fx := census(t)
		captured := fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Doc"},"blocks":[{"type":"paragraph","text":"see [the deal](`+link("hxwz2i")+`)"}]}`), false, true)

		require.NoError(t, err)
		marks := (*captured).Blocks[1].GetText().GetMarks().GetMarks()
		require.Len(t, marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Link, marks[0].Type)
		assert.Equal(t, link(realSpaceEval), marks[0].Param)
		assert.Empty(t, result.Warnings, "a resolved link says nothing")
	})

	t.Run("a created document whose link cannot resolve carries the warning", func(t *testing.T) {
		fx := census(t)
		fx.expectCreate("newObj")
		fx.expectEtagRead("newObj")

		result, err := fx.CreateObject(context.Background(), testSpaceId,
			[]byte(`{"formatVersion":"2.0","type":"page","properties":{"name":"Doc"},"blocks":[{"type":"paragraph","text":"see [the deal](`+link("nosuch")+`)"}]}`), false, true)

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/blocks", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `"nosuch"`)
	})

	t.Run("every write path that turns caller text into marks expands", func(t *testing.T) {
		// the list is the claim in APIV2.md §8.35: documents, block ops,
		// replace_text, chat messages
		for _, tc := range []struct {
			name string
			run  func(t *testing.T, fx *v2Fixture) []*model.BlockContentTextMark
		}{
			{"insert_blocks", func(t *testing.T, fx *v2Fixture) []*model.BlockContentTextMark {
				committed := fx.expectMutateState(editRead(t, editBaseDoc), nil)
				_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
					patchBody(`{"op":"insert_blocks","blocks":[{"type":"paragraph","text":"see [d](`+link("hxwz2i")+`)"}]}`), "", false, true)
				require.NoError(t, err)
				return marksOfText(t, *committed, "see d")
			}},
			{"update_block set text", func(t *testing.T, fx *v2Fixture) []*model.BlockContentTextMark {
				committed := fx.expectMutateState(editRead(t, editBaseDoc), nil)
				_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
					patchBody(`{"op":"update_block","id":"blockParent1","set":{"text":"see [d](`+link("hxwz2i")+`)"}}`), "", false, true)
				require.NoError(t, err)
				return marksOfText(t, *committed, "see d")
			}},
			{"replace_text", func(t *testing.T, fx *v2Fixture) []*model.BlockContentTextMark {
				read := editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[{"id":"blockParent1","type":"paragraph","text":"keep [d](`+link("hxwz2i")+`) tail"}]}`)
				committed := fx.expectMutateState(read, nil)
				_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
					patchBody(`{"op":"replace_text","find":"tail","replace":"end"}`), "", false, true)
				require.NoError(t, err)
				return marksOfText(t, *committed, "keep d end")
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fx := census(t)
				marks := tc.run(t, fx)
				require.NotEmpty(t, marks, "the link mark survived the op")
				var links int
				for _, mark := range marks {
					if mark.Type == model.BlockContentTextMark_Link {
						links++
						assert.Equal(t, link(realSpaceEval), mark.Param)
					}
				}
				assert.Equal(t, 1, links)
			})
		}
	})

	t.Run("a chat message's link is expanded, and an unresolved one is reported", func(t *testing.T) {
		fx := census(t)
		fx.addChat(t, testChatId, "Team chat", 1000)
		var sent []*model.BlockContentTextMark
		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.MatchedBy(func(req *pb.RpcChatAddMessageRequest) bool {
			sent = req.Message.Message.Marks
			return true
		})).Return(&pb.RpcChatAddMessageResponse{MessageId: "msgNew"}).Twice()

		got, err := fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "see [d](" + link("hxwz2i") + ")"}, false)

		require.NoError(t, err)
		require.Len(t, sent, 1)
		assert.Equal(t, link(realSpaceEval), sent[0].Param)
		assert.Empty(t, got.Warnings)

		got, err = fx.AddChatMessage(context.Background(), testSpaceId, testChatId,
			v2model.AddChatMessageRequest{Text: "see [d](" + link("nosuch") + ")"}, false)

		require.NoError(t, err)
		require.Len(t, got.Warnings, 1)
		assert.Equal(t, "/text", got.Warnings[0].Path)
	})
}

// marksOfText returns the marks of the committed block whose plain text
// matches want.
func marksOfText(t *testing.T, st interface {
	Blocks() []*model.Block
}, want string) []*model.BlockContentTextMark {
	t.Helper()
	for _, block := range st.Blocks() {
		text := block.GetText()
		if text == nil || text.Text != want {
			continue
		}
		return text.GetMarks().GetMarks()
	}
	t.Fatalf("no block with text %q; blocks: %s", want, blockTextDump(st.Blocks()))
	return nil
}

func blockTextDump(blocks []*model.Block) string {
	var out []string
	for _, block := range blocks {
		if text := block.GetText(); text != nil {
			out = append(out, fmt.Sprintf("%q", text.Text))
		}
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}
