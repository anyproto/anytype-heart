package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

// editCollectionDoc is a collection with one member.
const editCollectionDoc = `{"version":1,"id":"obj1","type":"collection","properties":{"name":"List"},"items":["memberA"]}`

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
		want := apimodel.V2DiffStats{BlocksChanged: 1}

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
		assert.Equal(t, apimodel.V2DiffStats{BlocksAdded: 2}, result.DiffStats)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksAdded: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "parent", "first child", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[2]["indent"], "inside: payload indent 0 = the container's child level")
	})

	t.Run("insertBlocks needs exactly one target", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","inside":"blockParent1","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, apimodel.V2CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "exactly one of after, before, inside")
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksAdded: 2, BlocksRemoved: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "a", "b", "the Q3 report and Q3 plan"}, blockTexts(blocks))
	})

	t.Run("replaceBlock keeps id, position and descendants", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceBlock","id":"blockParent1","block":{"type":"quote","text":"new **text**"}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, apimodel.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "quote", blocks[1]["type"])
		assert.Equal(t, "blockParent1", blocks[1]["id"])
		assert.Equal(t, "child", blocks[2]["text"], "descendants are kept")
		assert.Equal(t, float64(1), blocks[2]["indent"])
	})

	t.Run("replaceBlock to a leaf type with descendants names the count", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceBlock","id":"blockParent1","block":{"type":"divider"}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, `cannot change block "blockParent1" to leaf type "divider"`)
		assert.Contains(t, apiErr.Message, "1 descendant block")
	})

	t.Run("moveBlock inside moves the subtree and reindents", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockSibling2","inside":"blockParent1","position":"last"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, apimodel.V2DiffStats{BlocksMoved: 1}, result.DiffStats)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksRemoved: 2}, result.DiffStats)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		rows := blocks[0]["rows"].([]any)
		cells := rows[1].(map[string]any)["cells"].([]any)
		assert.Equal(t, []any{"Export", "done"}, cells)
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
		assert.Equal(t, apimodel.V2DiffStats{PropertiesChanged: 3}, result.DiffStats)
		doc := stateDoc(t, *captured)
		props := doc["properties"].(map[string]any)
		assert.Equal(t, "Renamed", props["name"])
		assert.Equal(t, true, props["done"])
		_, hasDescription := props["description"]
		assert.False(t, hasDescription)
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
		want := &apimodel.V2SideEffects{Options: []apimodel.V2CreatedOption{{Property: "severity", Name: "Critical"}}}

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"severity":["Critical"]}}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Created)
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
		assert.Equal(t, apimodel.V2CodeAmbiguousInput, apiErr.Code)
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

	t.Run("stale If-Match is a 409 with the current etag", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`), `"deadbeef"`, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusConflict, apiErr.Status)
		assert.Equal(t, apimodel.V2CodeEtagMismatch, apiErr.Code)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksRemoved: 2}, result.DiffStats)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksAdded: 2, BlocksRemoved: 4}, result.DiffStats,
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
		assert.Equal(t, apimodel.V2CodeEtagMismatch, apiErr.Code)
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
		assert.Equal(t, apimodel.V2DiffStats{BlocksChanged: 1}, result.DiffStats)
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
