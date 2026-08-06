package v2service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// editBaseDoc is the base test document: a heading, a parent paragraph with
// one child, and a sibling whose text has two "Q3" occurrences.
const editBaseDoc = `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc","description":"about"},"blocks":[` +
	`{"id":"blockHeading1","type":"heading1","text":"Section"},` +
	`{"id":"blockParent1","type":"paragraph","text":"parent"},` +
	`{"indent":1,"id":"blockChild1","type":"paragraph","text":"child"},` +
	`{"id":"blockSibling2","type":"paragraph","text":"the Q3 report and Q3 plan"}]}`

// editTableDoc holds one 2x2 table.
const editTableDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"tblOne1","type":"table",` +
	`"columns":[{"id":"colA"},{"id":"colB"}],` +
	`"rows":[{"id":"rowH","isHeader":true,"cells":["Name","Status"]},{"id":"rowB","cells":["Export"]}]}]}`

// editEmptyDoc has no blocks at all — SPEC §7 keeps title/description out of
// the document, so a fresh object has zero addressable blocks.
const editEmptyDoc = `{"version":1,"id":"obj1","type":"page","properties":{"name":"Empty"},"blocks":[]}`

// editCollectionDoc is a collection with one member.
const editCollectionDoc = `{"version":1,"id":"obj1","type":"collection","properties":{"name":"List"},"items":["memberA"]}`

// editLayoutDoc holds a row/column layout — the V3 containment case a payload
// fragment cannot see on its own (review B′1).
const editLayoutDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"rowOne1","type":"row"},` +
	`{"indent":1,"id":"colOne1","type":"column"},` +
	`{"indent":2,"id":"inCol1","type":"paragraph","text":"in column"}]}`

// editRead builds a live read from an AnyBlock document.
func editRead(t *testing.T, doc string) apicore.ObjectRead {
	t.Helper()
	sbType, snapshot, err := anyblockjson.Unmarshal([]byte(doc), anyblockjson.Options{})
	require.NoError(t, err)
	return apicore.ObjectRead{SbType: sbType, Snapshot: snapshot, Heads: []string{"headA"}}
}

// expectMutate wires the mutator mock the way the adapter behaves: apply
// runs against a state built from read's snapshot and, on success, that
// mutated state is captured for assertions and newHeads reported.
func (fx *v2Fixture) expectMutate(read apicore.ObjectRead, newHeads ...string) **state.State {
	var captured *state.State
	// PatchObject reads the object and checks preconditions BEFORE prewarming
	// create-missing refs and taking the lock (review A′1), so every PATCH
	// test needs the read wired.
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
	fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId, objectId string, apply func(apicore.ObjectEdit) error) ([]string, error) {
			st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
			if err != nil {
				return nil, err
			}
			if err := apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st}); err != nil {
				return nil, err
			}
			captured = st
			return newHeads, nil
		})
	return &captured
}

// expectReset wires the PUT reset path: build gets read, the returned
// snapshot is captured for assertions.
func (fx *v2Fixture) expectReset(read apicore.ObjectRead, newHeads ...string) **model.SmartBlockSnapshotBase {
	var captured *model.SmartBlockSnapshotBase
	fx.mutatorMock.EXPECT().ResetObject(mock.Anything, testSpaceId, "obj1", mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId, objectId string, build func(apicore.ObjectRead) (*model.SmartBlockSnapshotBase, error)) ([]string, error) {
			snapshot, err := build(read)
			if err != nil {
				return nil, err
			}
			captured = snapshot
			return newHeads, nil
		})
	return &captured
}

// snapshotDoc marshals a captured snapshot back to its document form.
func snapshotDoc(t *testing.T, snapshot *model.SmartBlockSnapshotBase) map[string]any {
	t.Helper()
	require.NotNil(t, snapshot)
	body, err := anyblockjson.Marshal(model.SmartBlockType_Page, snapshot, anyblockjson.Options{})
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	return doc
}

// stateDoc marshals a captured edit state back to its document form.
func stateDoc(t *testing.T, st *state.State) map[string]any {
	t.Helper()
	require.NotNil(t, st)
	return snapshotDoc(t, snapshotFromState(st))
}

// docBlocks extracts the blocks array of a marshaled document.
func docBlocks(doc map[string]any) []map[string]any {
	raw, _ := doc["blocks"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, b := range raw {
		out = append(out, b.(map[string]any))
	}
	return out
}

func blockTexts(blocks []map[string]any) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i], _ = b["text"].(string)
	}
	return out
}

func patchBody(ops ...string) []byte {
	return []byte(`{"ops":[` + joinStrings(ops, ",") + `]}`)
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

func TestPatchObject(t *testing.T) {
	ctx := context.Background()

	t.Run("updateBlock merges fields, suffix-addressed", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")
		want := v2model.V2DiffStats{BlocksChanged: 1}

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"Child1","set":{"text":"edited child"}}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, ComputeEtag([]string{"headB"}), result.Etag)
		assert.Equal(t, want, result.DiffStats)
		assert.Empty(t, result.CreatedBlocks)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "parent", "edited child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, "blockChild1", blocks[2]["id"], "the block id survives the merge")
	})

	t.Run("updateBlock rejects indent and id", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"blockChild1","set":{"indent":2}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Issues[0].Message, "use moveBlock")
		assert.Equal(t, "ops[0].set.indent", apiErr.Issues[0].Path)
	})

	t.Run("insertBlocks after anchor with nested payload mints ids", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","blocks":[{"type":"checkbox","text":"todo"},{"indent":1,"type":"paragraph","id":"clientId1","text":"note"}]}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2}, result.DiffStats)
		require.Len(t, result.CreatedBlocks, 2)
		assert.Len(t, result.CreatedBlocks["ops[0].blocks[0]"], 24, "minted id is editor-shaped")
		assert.Equal(t, "clientId1", result.CreatedBlocks["ops[0].blocks[1]"], "client-supplied ids are echoed")
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "todo", "note", "parent", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[2]["indent"], "payload indent is relative to the anchor level")
	})

	t.Run("insertBlocks inside position first lands at the child level", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","inside":"blockParent1","position":"first","blocks":[{"type":"paragraph","text":"first child"}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "parent", "first child", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[2]["indent"], "inside: payload indent 0 = the container's child level")
	})

	t.Run("insertBlocks with more than one target is ambiguous", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","inside":"blockParent1","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.V2CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "at most one of after, before, inside")
	})

	t.Run("insertBlocks with no anchor appends at the document end", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when: no after/before/inside — root-append, with a nested payload
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","blocks":[{"type":"paragraph","text":"appended"},{"indent":1,"type":"paragraph","text":"nested"}]}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "parent", "child", "the Q3 report and Q3 plan", "appended", "nested"}, blockTexts(blocks))
		_, hasIndent := blocks[4]["indent"]
		assert.False(t, hasIndent, "root-append lands at document level 0")
		assert.Equal(t, float64(1), blocks[5]["indent"], "payload indent stays relative to the insertion level")
	})

	t.Run("insertBlocks with no anchor gives an empty object its first content", func(t *testing.T) {
		// given: zero blocks — nothing is addressable, so anchored targeting
		// cannot work and PUT used to be the only way in (the corruption vector)
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editEmptyDoc), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","blocks":[{"type":"heading1","text":"First"},{"type":"paragraph","text":"body"}]}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"First", "body"}, blockTexts(blocks))
	})

	t.Run("insertBlocks position without a target is rejected", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","position":"first","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "position only applies to inside")
		assert.Contains(t, apiErr.Issues[0].Message, "appends at the end of the document")
	})

	t.Run("insertBlocks markdown payload is parsed into blocks (the authoring channel)", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when: markdown instead of blocks — same targeting, same pipeline
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","markdown":"- [ ] todo\n  - sub item"}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2}, result.DiffStats)
		require.Len(t, result.CreatedBlocks, 2)
		assert.Len(t, result.CreatedBlocks["ops[0].markdown[0]"], 24, "created ids are keyed by parsed position under markdown[j]")
		assert.Len(t, result.CreatedBlocks["ops[0].markdown[1]"], 24)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "todo", "sub item", "parent", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, "checkbox", blocks[1]["type"], "markdown checkbox syntax maps to a checkbox block")
		assert.Equal(t, float64(1), blocks[2]["indent"], "markdown indentation nests")
	})

	t.Run("insertBlocks markdown root-append works on an empty object", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editEmptyDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","markdown":"# First\n\nbody"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"First", "body"}, blockTexts(blocks))
		assert.Equal(t, "heading1", blocks[0]["type"])
	})

	t.Run("insertBlocks with both blocks and markdown is ambiguous", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","blocks":[{"type":"paragraph","text":"x"}],"markdown":"y"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.V2CodeAmbiguousInput, apiErr.Code)
		assert.Equal(t, "provide blocks or markdown, not both", apiErr.Message)
	})

	t.Run("insertBlocks with neither blocks nor markdown is rejected", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "insertBlocks needs a payload", apiErr.Message)
		assert.Contains(t, apiErr.Issues[0].Message, "markdown (parsed server-side)")
	})

	t.Run("insertBlocks blank markdown is rejected with a path", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","markdown":"  \n\n"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "markdown produced no blocks", apiErr.Message)
		assert.Equal(t, "ops[0].markdown", apiErr.Issues[0].Path)
	})

	t.Run("insertBlocks markdown over the block cap is rejected with the limit", func(t *testing.T) {
		// the markdown channel is byte-bounded, but 3 bytes encode one block —
		// the parsed run must share the blocks channel's 256 cap
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		md := strings.Repeat(`- x\n`, v2MaxMarkdownBlocksPerOp+1)
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(fmt.Sprintf(`{"op":"insertBlocks","after":"blockHeading1","markdown":"%s"}`, md)), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "markdown produced too many blocks", apiErr.Message)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].markdown", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "256")
		assert.Contains(t, apiErr.Issues[0].Message, "split the content")
	})

	t.Run("moveBlock with no anchor moves the subtree to the end", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockParent1"}`), "", false)

		require.NoError(t, err)
		// both reordered siblings count as moved (the documented diff rule)
		assert.Equal(t, v2model.V2DiffStats{BlocksMoved: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "the Q3 report and Q3 plan", "parent", "child"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[3]["indent"], "the subtree rides along")
	})

	t.Run("insertBlocks inside a leaf block is rejected", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTableDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","inside":"tblOne1","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `"table" blocks cannot have children`)
	})

	t.Run("payload monotonicity violation names both indents", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockParent1","blocks":[{"type":"paragraph","text":"a"},{"indent":2,"type":"paragraph","text":"b"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "monotonic")
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].blocks[1].indent", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "indent 2 follows indent 0")
	})

	t.Run("replaceSubtree swaps block and descendants", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockParent1","blocks":[{"type":"bulletedListItem","text":"a"},{"indent":1,"type":"paragraph","text":"b"}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2, BlocksRemoved: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "a", "b", "the Q3 report and Q3 plan"}, blockTexts(blocks))
	})

	t.Run("updateBlock retypes a block keeping id, position and descendants", func(t *testing.T) {
		// migrated from replaceBlock (folded into updateBlock, v0.3.5)
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"blockParent1","set":{"type":"quote","text":"new **text**"}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "quote", blocks[1]["type"])
		assert.Equal(t, "blockParent1", blocks[1]["id"])
		assert.Equal(t, "new **text**", blocks[1]["text"])
		assert.Equal(t, "child", blocks[2]["text"], "descendants are kept")
		assert.Equal(t, float64(1), blocks[2]["indent"])
	})

	t.Run("updateBlock null clears a field, unnamed fields stay", func(t *testing.T) {
		// the merge-with-null-clears semantics that made replaceBlock redundant
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"blockSibling2","set":{"text":null}}`), "", false)

		require.NoError(t, err)
		blocks := docBlocks(stateDoc(t, *captured))
		text, _ := blocks[3]["text"].(string)
		assert.Empty(t, text, "explicit null clears the text")
		assert.Equal(t, "paragraph", blocks[3]["type"], "unnamed fields survive the merge")
	})

	t.Run("updateBlock to a leaf type with descendants names the count", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"blockParent1","set":{"type":"divider"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `cannot change block "blockParent1" to leaf type "divider"`)
		assert.Contains(t, apiErr.Message, "1 descendant block")
	})

	t.Run("replaceBlock is gone and the hint names updateBlock", func(t *testing.T) {
		// folded into updateBlock pre-release (v0.3.5) — the unknown-op error
		// must steer an agent that learned the old vocabulary
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceBlock","id":"blockParent1","block":{"type":"quote"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `unknown op "replaceBlock"`)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Hint, "replaceBlock was removed — use updateBlock {id, set}")
		assert.NotContains(t, apiErr.Issues[0].Hint, "allowed ops: setProperties, updateBlock, replaceBlock",
			"the op list no longer carries replaceBlock")
	})

	t.Run("moveBlock inside moves the subtree and reindents", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockSibling2","inside":"blockParent1","position":"last"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksMoved: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "parent", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[3]["indent"], "moved under the parent")
	})

	t.Run("moveBlock into its own subtree is a cycle", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockParent1","inside":"blockChild1"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "inside its own subtree")
	})

	t.Run("deleteBlock without recursive names the descendant count", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockParent1"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `block "blockParent1" has 1 descendant block`)
		assert.Contains(t, apiErr.Message, `"recursive": true`)
	})

	t.Run("deleteBlock recursive removes the subtree", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockParent1","recursive":true}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksRemoved: 2}, result.DiffStats)
		assert.Equal(t, []string{"Section", "the Q3 report and Q3 plan"}, blockTexts(docBlocks(stateDoc(t, *captured))))
	})

	t.Run("replaceText no match steers to exact copy", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","id":"blockSibling2","find":"Q5","replace":"Q6"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `no match found for "Q5" in block "blockSibling2"`)
	})

	t.Run("replaceText multiple matches asks for more context", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","id":"blockSibling2","find":"Q3","replace":"Q4"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "found 2 matches")
		assert.Contains(t, apiErr.Message, "provide more context")
		assert.Contains(t, apiErr.Message, `"replace_all": true`)
	})

	t.Run("replaceText unique match replaces once", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","id":"blockSibling2","find":"Q3 report","replace":"Q4 report"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "the Q4 report and Q3 plan", blocks[3]["text"])
	})

	t.Run("replaceText replace_all replaces every match", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","id":"blockSibling2","find":"Q3","replace":"Q4","replace_all":true}`), "", false)

		require.NoError(t, err)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "the Q4 report and Q4 plan", blocks[3]["text"])
	})

	t.Run("setCell writes one cell", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTableDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setCell","tableId":"tblOne1","row":"rowB","col":"colB","value":"done"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		rows := blocks[0]["rows"].([]any)
		cells := rows[1].(map[string]any)["cells"].([]any)
		assert.Equal(t, []any{"Export", "done"}, cells)
	})

	t.Run("setCell keeps the table's wrapper block ids (A′3)", func(t *testing.T) {
		// the format does not carry the column/row layout wrappers, so the
		// importer mints fresh ids for them. Reusing the live ids keeps a cell
		// edit a cell edit — otherwise every table op replaces both wrappers
		// and re-parents every row and column, and concurrent edits on two
		// devices merge into a table with duplicated rows/columns.
		fx := newV2Fixture(t)
		read := editRead(t, editTableDoc)
		var liveWrappers []string
		for _, b := range read.Snapshot.Blocks {
			if b.Id == "tblOne1" {
				liveWrappers = append([]string(nil), b.ChildrenIds...)
			}
		}
		require.Len(t, liveWrappers, 2, "table has column and row wrappers")
		captured := fx.expectMutate(read, "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setCell","tableId":"tblOne1","row":"rowB","col":"colB","value":"done"}`), "", false)

		require.NoError(t, err)
		table := (*captured).Pick("tblOne1")
		require.NotNil(t, table)
		assert.Equal(t, liveWrappers, table.Model().ChildrenIds,
			"the wrapper ids must survive a cell edit")
		for _, id := range liveWrappers {
			assert.NotNil(t, (*captured).Pick(id), "wrapper %s must still exist", id)
		}
	})

	t.Run("setCell unknown row lists the rows", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTableDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setCell","tableId":"tblOne1","row":"rowZ","col":"colB","value":"x"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, `row "rowZ" not found in table "tblOne1"`)
		assert.Contains(t, apiErr.Message, "rowH, rowB")
	})

	t.Run("setCell with an invalid cell block fails post-op validation", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTableDoc))

		// cells never carry ids (SPEC §6.1) — the R5 net catches it
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setCell","tableId":"tblOne1","row":"rowB","col":"colB","value":{"id":"x1","type":"paragraph","text":"y"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "the ops would produce an invalid document — no op was applied", apiErr.Message)
		require.NotEmpty(t, apiErr.Issues)
	})

	t.Run("setProperties writes presence and unsets", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"name":"Renamed","done":true},"unset":["description"]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{PropertiesChanged: 3}, result.DiffStats)
		doc := stateDoc(t, *captured)
		props := doc["properties"].(map[string]any)
		assert.Equal(t, "Renamed", props["name"])
		assert.Equal(t, true, props["done"])
		_, hasDescription := props["description"]
		assert.False(t, hasDescription)
	})

	t.Run("a rejected precondition creates no options (A′1)", func(t *testing.T) {
		// create-missing resolution must run only after the request is known
		// to be legitimate: prewarming first meant a stale If-Match (412), a
		// missing object (404) or a restricted object (403) still permanently
		// created every option the batch named.
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)
		// no ObjectCreateRelationOption expectation and no mutator expectation:
		// reaching either fails the test

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"severity":["BrandNewOption"]}}`), `"deadbeef"`, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusConflict, apiErr.Status)
		assert.Equal(t, v2model.V2CodeEtagMismatch, apiErr.Code)
	})

	t.Run("dry run reports a created option once (C′2)", func(t *testing.T) {
		// prewarm and the op itself both resolve the same name; dry_run must
		// preview exactly what the real run reports
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"severity":["BrandNewOption"]}}`), "", true)

		require.NoError(t, err)
		require.NotNil(t, result.Created)
		assert.Len(t, result.Created.Options, 1, "the option is previewed once, not once per resolution")
		assert.Equal(t, "BrandNewOption", result.Created.Options[0].Name)
	})

	t.Run("a restricted object is refused on the dry run too (C′3)", func(t *testing.T) {
		// the restriction verdict rides the read, so dry_run cannot report a
		// success the real edit would refuse
		fx := newV2Fixture(t)
		read := editRead(t, editBaseDoc)
		read.EditRefused = v2model.V2ValidationFailed("this object's blocks cannot be edited through the API")
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`), "", true)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "cannot be edited")
	})

	t.Run("V3 row→column containment is enforced on the spliced result (B′1)", func(t *testing.T) {
		// a paragraph inside a row is legal as an isolated fragment — only the
		// whole document shows the violation, which is why the post-op validate
		// has to run
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editLayoutDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","inside":"rowOne1","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2InvalidDocMessage, apiErr.Message)
	})

	t.Run("a legal edit inside a column still passes (no false rejection)", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editLayoutDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","inside":"colOne1","blocks":[{"type":"paragraph","text":"added"}]}`), "", false)

		require.NoError(t, err)
		assert.NotNil(t, *captured)
	})

	t.Run("an over-deep insert is refused, not silently clamped (B′2)", func(t *testing.T) {
		// fragment validation is run-RELATIVE, so a deep run passes on its own;
		// spliced in it can push the document past the format's depth bound.
		// The exporter clamps rather than failing when a warning sink is
		// installed, which used to corrupt the view for later ops in the same
		// batch (deleteBlock then saw no descendants and dropped a subtree) and
		// leave the object permanently un-PATCHable.
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		run := make([]string, 0, 34)
		for i := 0; i < 34; i++ {
			indent := ""
			if i > 0 {
				indent = fmt.Sprintf(`"indent":%d,`, i)
			}
			run = append(run, fmt.Sprintf(`{%s"type":"toggle","text":"d%d"}`, indent, i))
		}
		body := patchBody(fmt.Sprintf(
			`{"op":"insertBlocks","after":"blockSibling2","blocks":[%s]}`, strings.Join(run, ",")))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", body, "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
	})

	t.Run("a batch beyond the op cap is refused (A′2)", func(t *testing.T) {
		// every op re-renders the view under the object lock, so the batch is
		// bounded; the cap is checked before any read or lock
		fx := newV2Fixture(t)
		ops := make([]string, v2MaxOpsPerPatch+1)
		for i := range ops {
			ops[i] = `{"op":"setProperties","set":{"name":"x"}}`
		}

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(ops...), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, "/ops", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "exceeds the 512-op limit")
	})

	t.Run("setProperties rejects output-only and unknown keys path-addressed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"resolvedLayout":"todo","totallyUnknown":1}}`), "", false)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 2)
		assert.Equal(t, "ops[0].set.resolvedLayout", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "output-only")
		assert.Equal(t, "ops[0].set.totallyUnknown", apiErr.Issues[1].Path)
		assert.Contains(t, apiErr.Issues[1].Message, "unknown property key")
	})

	t.Run("setProperties creates missing select options and reports them", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-critical",
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		})
		fx.expectMutate(editRead(t, editBaseDoc), "headB")
		want := &v2model.V2SideEffects{Options: []v2model.V2CreatedOption{{Property: "severity", Name: "Critical"}}}

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"severity":["Critical"]}}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Created)
	})

	t.Run("setProperties add on a select that already has a value is refused", func(t *testing.T) {
		// select holds ONE value; appending would leave a two-valued
		// single-select the UI renders arbitrarily. No create expectation:
		// the guard must fire before the option is minted.
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		read := editRead(t, editBaseDoc)
		read.Snapshot.Details.Fields["severity"] = pbtypes.StringList([]string{"opt-high"})
		fx.expectMutate(read)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","add":{"severity":["High"]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, "ops[0].add.severity", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "single value")
		assert.Contains(t, apiErr.Issues[0].Hint, "use set")
	})

	t.Run("setProperties add on an EMPTY select is allowed", func(t *testing.T) {
		// the guard is about overflowing an occupied single slot, not about
		// forbidding add on selects outright
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","add":{"severity":["High"]}}`), "", false)

		require.NoError(t, err)
		props := stateDoc(t, *captured)["properties"].(map[string]any)
		assert.Equal(t, []any{"opt-high"}, props["severity"],
			"stateDoc marshals without an option resolver, so the stored id shows")
	})

	t.Run("setProperties add appends to a multiSelect without duplicating", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when: op 0 adds Urgent (twice — dedupe within one op), op 1 adds
		// Urgent again plus Later — the existing entry is never duplicated
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(
				`{"op":"setProperties","add":{"tags":["Urgent","Urgent"]}}`,
				`{"op":"setProperties","add":{"tags":["Urgent","Later"]}}`), "", false)

		// then
		require.NoError(t, err)
		got := (*captured).CombinedDetails().Get(domain.RelationKey("tags")).StringList()
		assert.Equal(t, []string{"opt-urgent", "opt-later"}, got)
	})

	t.Run("setProperties remove deletes matching entries and no-ops on absent ones", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when: seed both tags, then remove one plus a name that resolves to
		// nothing — removal must never create the option it names
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(
				`{"op":"setProperties","add":{"tags":["Urgent","Later"]}}`,
				`{"op":"setProperties","remove":{"tags":["Urgent","Nonexistent"]}}`), "", false)

		// then: no ObjectCreateRelationOption expectation is wired — a create
		// RPC for "Nonexistent" would fail the test
		require.NoError(t, err)
		got := (*captured).CombinedDetails().Get(domain.RelationKey("tags")).StringList()
		assert.Equal(t, []string{"opt-later"}, got)
	})

	t.Run("setProperties remove on an absent key stays absent", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","remove":{"tags":["Urgent"]}}`), "", false)

		require.NoError(t, err)
		assert.False(t, (*captured).CombinedDetails().Has(domain.RelationKey("tags")),
			"remove never creates presence")
	})

	t.Run("setProperties add on a scalar format names the format", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","add":{"done":[true]}}`), "", false)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].add.done", apiErr.Issues[0].Path)
		assert.Equal(t, `"done" has format "checkbox" — add only applies to list-shaped formats (select, multiSelect, objects, files); use set`, apiErr.Issues[0].Message)
	})

	t.Run("setProperties rejects a key in more than one of set/unset/add/remove", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"tags":["Urgent"]},"add":{"tags":["Later"]}}`), "", false)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].add.tags", apiErr.Issues[0].Path)
		assert.Equal(t, `"tags" appears in both set and add — pick one`, apiErr.Issues[0].Message)
	})

	t.Run("setProperties add validates keys like set", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","add":{"resolvedLayout":["todo"],"totallyUnknown":["x"]}}`), "", false)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 2)
		assert.Equal(t, "ops[0].add.resolvedLayout", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "output-only")
		assert.Equal(t, "ops[0].add.totallyUnknown", apiErr.Issues[1].Path)
		assert.Contains(t, apiErr.Issues[1].Message, "unknown property key")
	})

	t.Run("setProperties add takes an array, not a scalar", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addTagProperty(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","add":{"tags":"Urgent"}}`), "", false)

		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].add.tags", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "add takes an array of entries")
	})

	t.Run("setProperties add creates missing option names and reports them", func(t *testing.T) {
		// the same create-missing resolution as set, including the pre-lock
		// prewarm (which scans add too)
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.Anything).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-critical",
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		}).Once()
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")
		want := &v2model.V2SideEffects{Options: []v2model.V2CreatedOption{{Property: "severity", Name: "Critical"}}}

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","add":{"severity":["Critical"]}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, want, result.Created, "created once — prewarm and the op share the resolver cache")
		got := (*captured).CombinedDetails().Get(domain.RelationKey("severity")).StringList()
		assert.Equal(t, []string{"opt-critical"}, got)
	})

	t.Run("addItems and removeItems edit the collection membership", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editCollectionDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"addItems","items":["memberB","memberA"]}`, `{"op":"removeItems","items":["memberA"]}`), "", false)

		require.NoError(t, err)
		doc := stateDoc(t, *captured)
		assert.Equal(t, []any{"memberB"}, doc["items"])
	})

	t.Run("addItems on a non-collection is rejected", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"addItems","items":["memberB"]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `addItems requires a collection — this object's type is "page"`)
	})

	t.Run("ambiguous suffix reference steers to the full id", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		// "1" is a suffix of blockHeading1, blockParent1 and blockChild1
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"1"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.V2CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "matches more than one block")
	})

	t.Run("missing block steers to the outline", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"nowhere"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "?outline=true")
	})

	t.Run("unknown op lists the op set", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"frobnicate"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `unknown op "frobnicate"`)
		assert.Contains(t, apiErr.Issues[0].Hint, "replaceText")
	})

	t.Run("stale If-Match is a 409 with the current etag, before the lock", func(t *testing.T) {
		// A′1: the precondition is checked on a plain read BEFORE prewarming
		// create-missing refs and before taking the object lock — so a stale
		// If-Match never reaches the mutator and can never mint options.
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)
		// deliberately NO mutator expectation: reaching it would fail the test

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`), `"deadbeef"`, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusConflict, apiErr.Status)
		assert.Equal(t, v2model.V2CodeEtagMismatch, apiErr.Code)
		assert.Contains(t, apiErr.Message, ComputeEtag([]string{"headA"}))
	})

	t.Run("matching If-Match passes", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`), QuoteEtag(ComputeEtag([]string{"headA"})), false)

		require.NoError(t, err)
	})

	t.Run("dry run computes the outcome without the mutator", func(t *testing.T) {
		// given: no MutateObject expectation — a call would fail the test
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockParent1","recursive":true}`), "", true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Empty(t, result.Etag)
		assert.Equal(t, v2model.V2DiffStats{BlocksRemoved: 2}, result.DiffStats)
	})

	t.Run("empty ops list is rejected", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", []byte(`{"ops":[]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "ops must not be empty")
	})

	t.Run("atomicity: a failing later op applies nothing", func(t *testing.T) {
		// given: the mutator returns whatever build produced — a nil snapshot
		// (never captured) proves the first op never landed
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc))

		// when: op 0 is fine, op 1 addresses a missing block
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`, `{"op":"deleteBlock","id":"nowhere"}`), "", false)

		// then
		require.Error(t, err)
		assert.Nil(t, *captured)
	})
}

func TestPutObject(t *testing.T) {
	ctx := context.Background()

	t.Run("id round-trip gives a minimal diff", func(t *testing.T) {
		// given: the natural loop — GET body, one text edited, PUT back
		fx := newV2Fixture(t)
		captured := fx.expectReset(editRead(t, editBaseDoc), "headB")
		edited := jsonReplace(t, editBaseDoc, "child", "edited child")

		// when
		result, err := fx.PutObject(ctx, testSpaceId, "obj1", edited, "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
		assert.Equal(t, ComputeEtag([]string{"headB"}), result.Etag)
		blocks := docBlocks(snapshotDoc(t, *captured))
		assert.Equal(t, "edited child", blocks[2]["text"])
	})

	t.Run("a body without block ids is the full-rewrite signal", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectReset(editRead(t, editBaseDoc), "headB")
		body := `{"version":1,"type":"page","properties":{"name":"Doc","description":"about"},"blocks":[` +
			`{"type":"heading1","text":"Section"},{"type":"paragraph","text":"parent"}]}`

		result, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(body), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.V2DiffStats{BlocksAdded: 2, BlocksRemoved: 4}, result.DiffStats,
			"fresh ids on every block read as remove-everything-add-everything (DELEGATE-52 signature)")
	})

	t.Run("a GET body with etag round-trips (etag is stripped)", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectReset(editRead(t, editBaseDoc), "headB")
		body := `{"version":1,"etag":"abcd1234","id":"obj1","type":"page","blocks":[{"id":"blockHeading1","type":"heading1","text":"Section"}]}`

		result, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(body), "", false)

		require.NoError(t, err)
		assert.NotEmpty(t, result.Etag)
	})

	t.Run("a mismatched document id is rejected", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(`{"version":1,"id":"otherObject","blocks":[]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "/id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `the URL addresses "obj1"`)
	})

	t.Run("unknown property keys get did-you-mean before any lock", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(`{"version":1,"properties":{"unknownKey":1},"blocks":[]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Issues[0].Message, "unknown property key")
	})

	t.Run("stale If-Match is a 409", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectReset(editRead(t, editBaseDoc))

		_, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(editBaseDoc), `"deadbeef"`, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.V2CodeEtagMismatch, apiErr.Code)
	})

	t.Run("system-managed objects are excluded", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editBaseDoc)
		read.SbType = model.SmartBlockType_FileObject
		fx.expectReset(read)

		_, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(editBaseDoc), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "system-managed")
	})

	t.Run("dry run computes the diff without the mutator", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		result, err := fx.PutObject(ctx, testSpaceId, "obj1", jsonReplace(t, editBaseDoc, "child", "x"), "", true)

		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Empty(t, result.Etag)
		assert.Equal(t, v2model.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
	})

	t.Run("an absent type keeps the live object's type", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectReset(editRead(t, editBaseDoc), "headB")
		body := `{"version":1,"id":"obj1","blocks":[{"id":"blockHeading1","type":"heading1","text":"Section"}]}`

		_, err := fx.PutObject(ctx, testSpaceId, "obj1", []byte(body), "", false)

		require.NoError(t, err)
		doc := snapshotDoc(t, *captured)
		assert.Equal(t, "page", doc["type"])
	})
}

// jsonReplace swaps the first occurrence of a literal substring in a JSON
// document (test helper).
func jsonReplace(t *testing.T, doc, from, to string) []byte {
	t.Helper()
	out := strings.Replace(doc, from, to, 1)
	require.NotEqual(t, doc, out, "replacement must apply")
	return []byte(out)
}
