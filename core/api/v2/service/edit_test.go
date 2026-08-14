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
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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

// editSoleParentDoc is one top-level block with one child: the document is
// exactly one subtree, so moving that subtree leaves no other block to anchor
// against.
const editSoleParentDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"blockParent1","type":"paragraph","text":"parent"},` +
	`{"indent":1,"id":"blockChild1","type":"paragraph","text":"child"}]}`

// editTwinDoc mirrors §5.4's locator eval doc: two sections whose body text
// is IDENTICAL ("Budget: TBD" under both Planning and Execution). A fixture
// whose blocks all have distinct text cannot catch the ambiguity refusal —
// a resolver that guesses the first match would sail through it — so the
// twin text is the point of this document.
const editTwinDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"secPlanning1","type":"heading1","text":"Planning"},` +
	`{"id":"budgetPlan1","type":"paragraph","text":"Budget: TBD"},` +
	`{"id":"secExec1","type":"heading1","text":"Execution"},` +
	`{"id":"budgetExec1","type":"paragraph","text":"Budget: TBD"}]}`

// editTableCellChildDoc holds a table whose only cell is the F10 array form:
// a toggle cell block (no id — derived) with one minted-id DESCENDANT. Cell
// descendants render as flat blocks, carry ids and are in the relabel pool —
// the id population the docLocalIds comment used to deny existed.
const editTableCellChildDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"tblOne1","type":"table",` +
	`"columns":[{"id":"colA"}],` +
	`"rows":[{"id":"rowA","cells":[[{"type":"toggle","text":"cell"},` +
	`{"indent":1,"id":"0000000000000000000dddd1","type":"paragraph","text":"inside"}]]}]}]}`

// editMintedDoc mirrors the base document with editor-shaped (24-hex) block
// ids — the documents whose default read serves compact labels.
const editMintedDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"0000000000000000000aaaa1","type":"heading1","text":"Section"},` +
	`{"id":"0000000000000000000bbbb1","type":"paragraph","text":"parent"}]}`

// editCollectionDoc is a collection with one member.
const editCollectionDoc = `{"version":1,"id":"obj1","type":"collection","properties":{"name":"List"},"items":["memberA"]}`

// editLayoutDoc holds a row/column layout — the V3 containment case a payload
// fragment cannot see on its own (review B′1).
const editLayoutDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"rowOne1","type":"row"},` +
	`{"indent":1,"id":"colOne1","type":"column"},` +
	`{"indent":2,"id":"inCol1","type":"paragraph","text":"in column"}]}`

// editManyBlocksDoc builds a document of n filler paragraphs after the base
// heading/parent/child/sibling four — the pre-existing-large-document shape
// of the M7 render-work bound.
func editManyBlocksDoc(n int) string {
	var sb strings.Builder
	sb.WriteString(`{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[` +
		`{"id":"blockHeading1","type":"heading1","text":"Section"},` +
		`{"id":"blockParent1","type":"paragraph","text":"parent"},` +
		`{"indent":1,"id":"blockChild1","type":"paragraph","text":"child"},` +
		`{"id":"blockSibling2","type":"paragraph","text":"the Q3 report and Q3 plan"}`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, `,{"id":"filler%06d","type":"paragraph","text":"filler %d"}`, i, i)
	}
	sb.WriteString(`]}`)
	return sb.String()
}

// editRead builds a live read from an AnyBlock document.
func editRead(t *testing.T, doc string) apicore.ObjectRead {
	t.Helper()
	sbType, snapshot, err := anyblockjson.Unmarshal([]byte(doc), anyblockjson.Options{})
	require.NoError(t, err)
	return apicore.ObjectRead{SbType: sbType, Snapshot: snapshot, Heads: []string{"headA"}}
}

// blocksRefusedProduction mirrors the adapter's checkRestriction output for
// the Blocks axis (core/api/objectmutateadapter.go): a bare error chain
// wrapping restriction.ErrRestricted, NOT a ready-made *v2model.Error. The
// earlier fixture fed a v2model.Error here, which kept the refusal tests
// green while production fell through to a 500 (surface review M2a) — the
// refusal test must eat what the adapter actually cooks.
func blocksRefusedProduction() error {
	return fmt.Errorf("%w: this object's blocks cannot be edited through the API",
		fmt.Errorf("%w: %s", restriction.ErrRestricted, model.Restrictions_Blocks.String()))
}

// expectMutate wires the mutator mock the way the adapter behaves: apply
// runs against a state built from read's snapshot and, on success, that
// mutated state is captured for assertions and newHeads reported.
func (fx *v2Fixture) expectMutate(read apicore.ObjectRead, newHeads ...string) **state.State {
	return fx.expectMutateState(read, nil, newHeads...)
}

// expectMutateHeader is expectMutate over a state that carries the structural
// header — the header wrapper and its title block, which every real page gets
// from template.InitTemplate at first open and which SPEC §7 keeps out of the
// served document. A document built from an AnyBlock snapshot alone has none,
// so nothing else in this file can see a placement rule that depends on it.
func (fx *v2Fixture) expectMutateHeader(read apicore.ObjectRead, newHeads ...string) **state.State {
	return fx.expectMutateState(read, func(st *state.State) {
		template.InitTemplate(st, template.WithTitle)
	}, newHeads...)
}

func (fx *v2Fixture) expectMutateState(read apicore.ObjectRead, prepare func(*state.State), newHeads ...string) **state.State {
	var captured *state.State
	// PatchObject reads the object and checks preconditions BEFORE prewarming
	// create-missing refs and taking the lock (review A′1), so every PATCH
	// test needs the read wired.
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
	fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
			st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
			if err != nil {
				return nil, err
			}
			if prepare != nil {
				prepare(st)
			}
			if err := apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st}); err != nil {
				return nil, err
			}
			captured = st
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
		want := v2model.DiffStats{BlocksChanged: 1}

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
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","blocks":[{"type":"checkbox","text":"todo"},{"indent":1,"type":"paragraph","text":"note"}]}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2}, result.DiffStats)
		require.Len(t, result.CreatedBlocks, 2)
		assert.Len(t, result.CreatedBlocks["ops[0].blocks[0]"], 24, "minted id is editor-shaped")
		assert.Len(t, result.CreatedBlocks["ops[0].blocks[1]"], 24)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "todo", "note", "parent", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[2]["indent"], "payload indent is relative to the anchor level")
	})

	t.Run("an insertBlocks payload id is refused as not part of the op", func(t *testing.T) {
		// reproduced before payload id resolution: a compact label ("bbbb1")
		// copied from a default read into an insertBlocks payload was stored
		// as the literal block id, and matchBlockRef resolves exact matches
		// FIRST — so the adopted label captured the reference and the next
		// replaceText on "bbbb1" edited the copy while the original block
		// silently lost its label. Resolution closed that (payloadids.go),
		// which left the slot with NO working value at all — so the field is
		// gone from the op's schema (§8.30) and the refusal says so rather
		// than reporting a duplicate the caller could not have foreseen.
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editMintedDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","blocks":[{"id":"bbbb1","type":"paragraph","text":"copy"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].blocks[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `"bbbb1"`)
		assert.Contains(t, apiErr.Issues[0].Message, "not part of this op")
		assert.Contains(t, apiErr.Issues[0].Hint, "createdBlocks", "the mint escape hatch is named")
	})

	t.Run("insertBlocks inside position first lands at the child level", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","inside":"blockParent1","position":"first","blocks":[{"type":"paragraph","text":"first child"}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 1}, result.DiffStats)
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
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
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
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"First", "body"}, blockTexts(blocks))
	})

	// §8.32: `position` with no targeting field was a guaranteed 400, and it
	// is the shape gemma4:e2b produced on 20 payloads — 10 of 10 in each of
	// the two cases that reach for it (add a section at the end, copy this
	// block as new content). The published description ("with inside only")
	// reads as "last is the default, so naming it is harmless". It now names
	// an end of the DOCUMENT, which is both the obvious intent and the only
	// expression of "insert at the beginning" that needs no prior read.
	t.Run("insertBlocks position last with no target appends at the document end", func(t *testing.T) {
		// given — the exact payload shape the probe recorded
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","markdown":"## Risks","position":"last"}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Section", "parent", "child", "the Q3 report and Q3 plan", "Risks"}, blockTexts(blocks))
	})

	t.Run("insertBlocks position first with no target inserts at the document start", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when: the direction that used to require reading the document first
		// just to learn the id of the block to sit before
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","position":"first","blocks":[{"type":"heading2","text":"Summary"},{"indent":1,"type":"paragraph","text":"note"}]}`), "", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"Summary", "note", "Section", "parent", "child", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		_, hasIndent := blocks[0]["indent"]
		assert.False(t, hasIndent, "root-first lands at document level 0")
		assert.Equal(t, float64(1), blocks[1]["indent"], "payload indent stays relative to the insertion level")
	})

	t.Run("insertBlocks position first keeps the structural header above it", func(t *testing.T) {
		// given: a real page's state root holds the header (title, description
		// — SPEC §7 keeps it OUT of the served document) as its FIRST child, so
		// prepending at the state root would land the block above the title
		fx := newV2Fixture(t)
		captured := fx.expectMutateHeader(editRead(t, editBaseDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","position":"first","blocks":[{"type":"paragraph","text":"top"}]}`), "", false)

		// then
		require.NoError(t, err)
		st := *captured
		require.NotNil(t, st)
		assert.Equal(t, template.HeaderLayoutId, st.Pick(st.RootId()).Model().ChildrenIds[0],
			"the header stays the first child of the state root")
		assert.Equal(t, []string{"top", "Section", "parent", "child", "the Q3 report and Q3 plan"},
			blockTexts(docBlocks(stateDoc(t, st))), "and the block is first in the document")
	})

	t.Run("insertBlocks position first on an empty object is the same slot as last", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editEmptyDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","position":"first","blocks":[{"type":"paragraph","text":"only"}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 1}, result.DiffStats)
		assert.Equal(t, []string{"only"}, blockTexts(docBlocks(stateDoc(t, *captured))))
	})

	t.Run("insertBlocks position with no target still rejects a value outside the enum", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","position":"middle","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "invalid position", apiErr.Message)
		assert.Equal(t, "ops[0].position", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "first, last")
	})

	t.Run("insertBlocks position alongside after is still refused", func(t *testing.T) {
		// the anchor already names the slot — this one is unchanged
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","after":"blockHeading1","position":"first","blocks":[{"type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "position only applies to inside")
		assert.Equal(t, "ops[0].position", apiErr.Issues[0].Path)
	})

	t.Run("moveBlock position first with no target moves the block to the document start", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockParent1","position":"first"}`), "", false)

		require.NoError(t, err)
		// every reordered block counts as moved (the documented diff rule)
		assert.Equal(t, v2model.DiffStats{BlocksMoved: 3}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, []string{"parent", "child", "Section", "the Q3 report and Q3 plan"}, blockTexts(blocks))
		assert.Equal(t, float64(1), blocks[1]["indent"], "the subtree rides along")
	})

	t.Run("moveBlock position first on the block already first keeps the order", func(t *testing.T) {
		// the moved subtree cannot anchor itself (InsertTo refuses its own
		// target) — the anchor search skips it, so the op succeeds where it
		// would otherwise have appended the block to the END
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockHeading1","position":"first"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, []string{"Section", "parent", "child", "the Q3 report and Q3 plan"},
			blockTexts(docBlocks(stateDoc(t, *captured))))
	})

	t.Run("moveBlock position first when the moved subtree is the whole document", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSoleParentDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"moveBlock","id":"blockParent1","position":"first"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, []string{"parent", "child"}, blockTexts(docBlocks(stateDoc(t, *captured))))
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
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2}, result.DiffStats)
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
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
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
		assert.Equal(t, v2model.DiffStats{BlocksMoved: 2}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 2, BlocksRemoved: 2}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksMoved: 1}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksRemoved: 2}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
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

	// ---- find-as-locator (Wave 2.1a, §8.43): id omitted, find locates ----

	t.Run("locator: omitted id resolves the one block containing find", func(t *testing.T) {
		// the fixture has FOUR blocks and the snippet lives in the LAST one —
		// a resolver that guessed the first block (or any fixed index) would
		// either error or edit the wrong text, and the full-texts assertion
		// would show it
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")
		want := v2model.DiffStats{BlocksChanged: 1}

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Q3 report","replace":"Q4 report"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, want, result.DiffStats)
		assert.Equal(t, []string{"Section", "parent", "child", "the Q4 report and Q3 plan"},
			blockTexts(docBlocks(stateDoc(t, *captured))), "the edit lands on the located block and nowhere else")
	})

	t.Run("locator: zero matches is a 404 steering to the outline read", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Q9","replace":"Q4"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
		assert.Contains(t, apiErr.Message, `no block contains "Q9"`)
		assert.Contains(t, apiErr.Message, "copy the find text exactly, including inline markup")
		assert.Contains(t, apiErr.Message, "markdown source", "the snippet may have missed only because of markup")
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].find", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "?outline=true")
	})

	t.Run("locator: several matching blocks refuse and list the candidates", func(t *testing.T) {
		// twin fixture: two blocks carry IDENTICAL text, so a guessed first
		// match would succeed here — the test demands the refusal instead,
		// with both blocks named for the retry
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwinDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Budget: TBD","replace":"Budget: $40k"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, `"Budget: TBD" appears in 2 blocks`)
		assert.Contains(t, apiErr.Message, "retry with id naming one of")
		assert.Contains(t, apiErr.Message, "block budgetPlan1 (paragraph)")
		assert.Contains(t, apiErr.Message, "block budgetExec1 (paragraph)")
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].find", apiErr.Issues[0].Path)
		assert.Nil(t, *captured, "a refusal never writes")
	})

	t.Run("locator: several occurrences within the ONE matching block get the more-context refusal, not ambiguity", func(t *testing.T) {
		// "Q3" appears twice in blockSibling2 and nowhere else — a fixture
		// where the snippet appears once per block could not tell this class
		// (within-block multiplicity, replace_all's territory) apart from the
		// several-blocks ambiguity above
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Q3","replace":"Q4"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code, "within-block multiplicity is not ambiguous_input")
		assert.Contains(t, apiErr.Message, `found 2 matches for "Q3" in block "blockSibling2"`,
			"the refusal names the RESOLVED block — a valid retry value")
		assert.Contains(t, apiErr.Message, "provide more context")
		assert.Contains(t, apiErr.Message, `"replace_all": true`)
	})

	t.Run("locator: replace_all resolves the block and replaces within it", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Q3","replace":"Q4","replace_all":true}`), "", false)

		require.NoError(t, err)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "the Q4 report and Q4 plan", blocks[3]["text"])
	})

	t.Run("locator: replace_all never widens the locator across blocks", func(t *testing.T) {
		// replace_all licenses every occurrence WITHIN the one matched block
		// (§5.3); on the twin fixture the locator still refuses
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwinDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Budget: TBD","replace":"Budget: $40k","replace_all":true}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Nil(t, *captured, "a refusal never writes")
	})

	t.Run("locator mid-batch: op i resolves against op i-1's edits (in-place view path)", func(t *testing.T) {
		// op 0 (itself a locator op, so the view is maintained in place — M7)
		// INTRODUCES the only occurrence of "needle"; op 1 locates by it. A
		// single-op test cannot catch mid-batch staleness: against the
		// pre-batch document op 1 has zero matches and the batch would refuse.
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"replaceText","find":"child","replace":"a needle appears"}`,
			`{"op":"replaceText","find":"needle","replace":"pin"}`,
		), "", false)

		require.NoError(t, err)
		assert.Equal(t, []string{"Section", "parent", "a pin appears", "the Q3 report and Q3 plan"},
			blockTexts(docBlocks(stateDoc(t, *captured))))
	})

	t.Run("locator mid-batch: an earlier op's edit makes a later locator ambiguous — refuse, never the stale unique match", func(t *testing.T) {
		// op 0 (a view-REBUILDING op, the other freshness path) writes "child"
		// into a second block. Against the pre-batch document op 1's find is
		// unique — so a resolver reading a stale view would silently edit
		// blockChild1; the fresh view demands the ambiguity refusal naming
		// both blocks. This is the silent-wrong-match failure the design
		// exists to prevent, pinned in the direction that hurts.
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"updateBlock","id":"blockParent1","set":{"text":"child of mine"}}`,
			`{"op":"replaceText","find":"child","replace":"kid"}`,
		), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "appears in 2 blocks")
		assert.Contains(t, apiErr.Message, "blockParent1")
		assert.Contains(t, apiErr.Message, "blockChild1")
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[1].find", apiErr.Issues[0].Path, "the failing op is the one addressed")
		assert.Nil(t, *captured, "the whole batch refuses — nothing is committed")
	})

	t.Run("locator: a dry run resolves identically and commits nothing (C9)", func(t *testing.T) {
		// no mutator expectation is wired at all — the dry run must never
		// reach it; resolution runs at apply time on the private state
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceText","find":"Q3 report","replace":"Q4 report"}`), "", true)

		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
	})

	t.Run("setCell writes one cell", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTableDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setCell","tableId":"tblOne1","row":"rowB","col":"colB","value":"done"}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
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
		assert.Equal(t, v2model.DiffStats{PropertiesChanged: 3}, result.DiffStats)
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
		assert.Equal(t, v2model.CodeEtagMismatch, apiErr.Code)
	})

	// M5 (surface review): create-missing is irreversible and was unbounded.
	// The two halves below catch different requests — the bound stops a batch
	// that WOULD succeed, the ordering stops one that cannot.
	t.Run("M5: a failing op creates nothing, even though an earlier op named new options", func(t *testing.T) {
		// the reproducer: op 1 names an option, op 2 is a 404. Before M5 the
		// batch failed AND the option existed, permanently, with no delete
		// surface to undo it.
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)
		// no ObjectCreateRelationOption expectation: creating anything fails
		// the test. No mutator expectation either — the batch must not commit.

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"severity":["BrandNewOption"]}}`,
				`{"op":"updateBlock","id":"doesNotExist","set":{"text":"hi"}}`), "", false)

		require.Error(t, err)
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status, "the batch fails on the bad block ref")
	})

	t.Run("M5: the set/unset conflict creates nothing", func(t *testing.T) {
		// the second reproducer: prewarm's skip list covered a key claimed by
		// both `add` and `set` but never read `unset`, so this created the
		// option and then 400ed. Validating the batch first covers the whole
		// family instead of one more special case.
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"severity":["BrandNewOption"]},"unset":["severity"]}`), "", false)

		require.Error(t, err)
	})

	t.Run("M5: a batch over the option cap is refused before any create", func(t *testing.T) {
		// this one WOULD apply cleanly — only the bound stops it. At the body
		// cap the same shape reaches ~10^6 permanent objects.
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil).Maybe()

		names := make([]string, 0, v2MaxCreatedOptionsPerPatch+1)
		for i := 0; i <= v2MaxCreatedOptionsPerPatch; i++ {
			names = append(names, fmt.Sprintf(`"Hallucinated-%d"`, i))
		}
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(fmt.Sprintf(`{"op":"setProperties","set":{"severity":[%s]}}`,
				strings.Join(names, ","))), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "would create")
		assert.Contains(t, apiErr.Issues[0].Message, "severity")
		assert.Contains(t, apiErr.Issues[0].Hint, "permanent")
	})

	t.Run("M5: a batch at the cap still applies", func(t *testing.T) {
		// the bound must not break legitimate bulk tagging
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		read := editRead(t, editBaseDoc)
		fx.expectMutate(read, "headB")
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, req *pb.RpcObjectCreateRelationOptionRequest) *pb.RpcObjectCreateRelationOptionResponse {
				name := req.Details.GetFields()[bundle.RelationKeyName.String()].GetStringValue()
				return &pb.RpcObjectCreateRelationOptionResponse{
					ObjectId: "opt-" + name,
					Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
				}
			}).Times(v2MaxCreatedOptionsPerPatch)

		names := make([]string, 0, v2MaxCreatedOptionsPerPatch)
		for i := 0; i < v2MaxCreatedOptionsPerPatch; i++ {
			names = append(names, fmt.Sprintf(`"Bulk-%d"`, i))
		}
		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(fmt.Sprintf(`{"op":"setProperties","set":{"severity":[%s]}}`,
				strings.Join(names, ","))), "", false)

		require.NoError(t, err)
		require.NotNil(t, result.Created)
		assert.Len(t, result.Created.Options, v2MaxCreatedOptionsPerPatch)
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

	// M1 (surface review): the gate is per-op. Sets and collections carry
	// Restrictions_Blocks but NOT Restrictions_Details, so a blanket
	// per-request check made renaming a set — and every addItems, the only v2
	// route into an existing collection — permanently refuse.
	t.Run("M1: a blocks-restricted object still accepts a property edit", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editBaseDoc)
		read.BlocksRefused = blocksRefusedProduction()
		captured := fx.expectMutate(read, "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"name":"Renamed"}}`), "", false)

		// before M1 this returned the blocks refusal, so a set could never be
		// renamed through v2 even though nothing restricted its details
		require.NoError(t, err)
		require.NotNil(t, *captured, "the mutator must be reached")
		assert.Equal(t, "Renamed", (*captured).Details().GetString(bundle.RelationKeyName))
	})

	t.Run("M1: the batch's needs carry only the axes its ops touch", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editBaseDoc)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()

		var got apicore.EditNeeds
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
				got = needs
				st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
				if err != nil {
					return nil, err
				}
				if err := apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st}); err != nil {
					return nil, err
				}
				return []string{"headB"}, nil
			})

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"name":"Renamed"}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, apicore.EditNeeds{Details: true}, got,
			"a property-only batch must not demand the Blocks axis")
	})

	t.Run("M1: a blocks-restricted object still refuses a block op, naming it", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editBaseDoc)
		read.BlocksRefused = blocksRefusedProduction()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)
		// no MutateObject expectation: reaching the mutator would fail the test

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setProperties","set":{"name":"Renamed"}}`,
				`{"op":"deleteBlock","id":"blockChild1"}`), "", false)

		// M2a: the refusal is PERMANENT, so it must be the C6 403 — not the
		// bare error RespondV2Error turns into a retryable 500
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusForbidden, apiErr.Status)
		assert.Equal(t, v2model.CodeForbidden, apiErr.Code)
		assert.Contains(t, apiErr.Message, "cannot be edited")
		assert.Contains(t, apiErr.Message, "/ops/1", "the refusal must address the offending op, not the request")
	})

	t.Run("M2a: a restriction refusal from the in-lock re-check is a 403, not a read-shaped 500", func(t *testing.T) {
		// the adapter re-checks restrictions under the lock (and Apply checks
		// per-block restrictions) — a refusal surfacing from MutateObject must
		// classify like the pre-lock gate's, not fall through mapReadError
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			Return(nil, blocksRefusedProduction())

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusForbidden, apiErr.Status)
		assert.Equal(t, v2model.CodeForbidden, apiErr.Code)
		assert.NotContains(t, apiErr.Message, "read object", "a refused write must not be dressed as a failed read")
	})

	t.Run("a restricted object is refused on the dry run too (C′3)", func(t *testing.T) {
		// the restriction verdict rides the read, so dry_run cannot report a
		// success the real edit would refuse
		fx := newV2Fixture(t)
		read := editRead(t, editBaseDoc)
		read.BlocksRefused = blocksRefusedProduction()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"blockChild1"}`), "", true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusForbidden, apiErr.Status, "the dry run reaches the same 403 the real edit would")
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

	t.Run("a blocks run beyond the advertised per-op cap is refused (M7)", func(t *testing.T) {
		// the op schemas advertise maxItems 256 on the blocks channel; before
		// M7 nothing enforced it, so one op could inflate the document by
		// 24,000 blocks that every later op re-rendered under the object lock
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))
		run := make([]string, v2MaxBlocksPerOp+1)
		for i := range run {
			run[i] = `{"type":"paragraph","text":"x"}`
		}

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(fmt.Sprintf(`{"op":"insertBlocks","after":"blockHeading1","blocks":[%s]}`, strings.Join(run, ","))), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, "too many blocks in one op", apiErr.Message)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].blocks", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "256")
	})

	t.Run("replaceSubtree shares the per-op blocks cap (M7)", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))
		run := make([]string, v2MaxBlocksPerOp+1)
		for i := range run {
			run[i] = `{"type":"paragraph","text":"x"}`
		}

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(fmt.Sprintf(`{"op":"replaceSubtree","id":"blockParent1","blocks":[%s]}`, strings.Join(run, ","))), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, "too many blocks in one op", apiErr.Message)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].blocks", apiErr.Issues[0].Path)
	})

	t.Run("a batch whose re-render product exceeds the work bound is refused whole (M7)", func(t *testing.T) {
		// the 512-op cap bounds one factor of the O(ops × document) product;
		// this bounds the product itself: 512 structural ops on a 2,500-block
		// document is ~1.28M block-renders of lock-held marshal work — over
		// v2MaxPatchRenderWork — and is refused after the begin() marshal,
		// before any op applies
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editManyBlocksDoc(2500)))
		ops := make([]string, v2MaxOpsPerPatch)
		for i := range ops {
			ops[i] = `{"op":"moveBlock","id":"blockParent1"}`
		}

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(ops...), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, "this PATCH is too much re-rendering work for one atomic batch", apiErr.Message)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "block-renders")
		assert.Contains(t, apiErr.Issues[0].Hint, "split the edit")
	})

	t.Run("a replaceText batch is exempt from the work bound and applies on a large document (M7)", func(t *testing.T) {
		// replaceText keeps the view valid in place (v2OpRebuildsView false),
		// so a full 512-op text-edit batch on a 2,500-block document costs two
		// document renders, not 512 — and must NOT trip the render-work bound
		// the moveBlock batch above trips
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editManyBlocksDoc(2500)), "headB")
		ops := make([]string, v2MaxOpsPerPatch)
		for i := range ops {
			ops[i] = `{"op":"replaceText","id":"blockSibling2","find":"report","replace":"report"}`
		}

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(ops...), "", false)

		require.NoError(t, err)
		require.NotNil(t, *captured)
	})

	t.Run("sequential replaceText matches against the canonical inline form (M7)", func(t *testing.T) {
		// op 1 splices "**re****port**" — raw adjacent bolds whose canonical
		// rendering is "**report**". The view op 2 addresses must carry the
		// canonical form (what a re-marshal emits and what the agent would
		// read back), whether the view was rebuilt or maintained in place.
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"replaceText","id":"blockSibling2","find":"report","replace":"**re****port**"}`,
			`{"op":"replaceText","id":"blockSibling2","find":"the Q3 **report**","replace":"the Q3 **REPORT**"}`,
		), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "the Q3 **REPORT** and Q3 plan", blocks[3]["text"])
	})

	t.Run("an op after replaceText sees the replaced text (M7)", func(t *testing.T) {
		// updateBlock merges on the view block — if the in-place text update
		// were wrong or stale, the merge would resurrect the pre-replace text
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(
			`{"op":"replaceText","id":"blockChild1","find":"child","replace":"kid"}`,
			`{"op":"updateBlock","id":"blockChild1","set":{"color":"red"}}`,
		), "", false)

		require.NoError(t, err)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, "kid", blocks[2]["text"], "the merge must ride on the replaced text")
		assert.Equal(t, "red", blocks[2]["color"])
	})

	t.Run("a modest batch on a large document stays under the work bound (M7)", func(t *testing.T) {
		// the bound must catch the abusive product, not ordinary edits to big
		// documents: 8 ops × 2,500 blocks is well inside the budget
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editManyBlocksDoc(2500)), "headB")
		ops := make([]string, 8)
		for i := range ops {
			ops[i] = `{"op":"replaceText","id":"blockSibling2","find":"report","replace":"report"}`
		}

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1", patchBody(ops...), "", false)

		require.NoError(t, err)
		require.NotNil(t, *captured)
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
		want := &v2model.SideEffects{Options: []v2model.CreatedOption{{Property: "severity", Name: "Critical"}}}

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
		want := &v2model.SideEffects{Options: []v2model.CreatedOption{{Property: "severity", Name: "Critical"}}}

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
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "matches more than one block")
	})

	t.Run("missing block steers to the outline, C6-shaped", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"deleteBlock","id":"nowhere"}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, `"nowhere"`)
		require.NotEmpty(t, apiErr.Issues, "the repair loop rides a C6 issue, not the message prose")
		assert.Equal(t, "ops[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "?outline=true")
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
		assert.Equal(t, v2model.CodeEtagMismatch, apiErr.Code)
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
		assert.Equal(t, v2model.DiffStats{BlocksRemoved: 2}, result.DiffStats)
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

// jsonReplace swaps the first occurrence of a literal substring in a JSON
// document (test helper).
func jsonReplace(t *testing.T, doc, from, to string) []byte {
	t.Helper()
	out := strings.Replace(doc, from, to, 1)
	require.NotEqual(t, doc, out, "replacement must apply")
	return []byte(out)
}

// TestApplierRenderCounts pins the M7 bounded-work property in the unit the
// render-work bound is denominated in: whole-document renders (marshalDoc
// calls). A wall-clock assertion would be flaky in CI; a render count is not.
func TestApplierRenderCounts(t *testing.T) {
	ctx := context.Background()

	// newApplier builds a raw applier over a private state — the same
	// construction applyPatchOps performs.
	newApplier := func(t *testing.T, fx *v2Fixture, doc string) *v2StateApplier {
		t.Helper()
		edit, err := editFromRead("obj1", editRead(t, doc))
		require.NoError(t, err)
		resolvers := fx.newCreatingResolvers(ctx, testSpaceId, false)
		return newV2StateApplier(fx.V2Service, testSpaceId, "obj1", edit.SbType, edit.State, resolvers)
	}

	t.Run("a replaceText batch renders the document exactly twice", func(t *testing.T) {
		// begin + the final after-document — NOT once per op: replaceText
		// maintains the view in place (M7), which is what turns a text-edit
		// batch from O(ops × document) into O(document) under the object lock
		fx := newV2Fixture(t)
		applier := newApplier(t, fx, editBaseDoc)
		_, err := applier.begin()
		require.NoError(t, err)

		op := json.RawMessage(`{"op":"replaceText","id":"blockSibling2","find":"report","replace":"report"}`)
		for i := 0; i < 50; i++ {
			require.NoError(t, applier.apply(i, op))
		}
		_, err = applier.currentDoc()

		require.NoError(t, err)
		assert.Equal(t, 2, applier.marshalCount,
			"50 replaceText ops must cost begin + final renders only")
	})

	t.Run("a locator batch renders the document exactly twice too", func(t *testing.T) {
		// find-as-locator resolution reads the SAME live view the op already
		// holds (§8.43) — it must add zero renders, or a 512-op locator batch
		// would re-inherit the O(ops × document) product M7 removed
		fx := newV2Fixture(t)
		applier := newApplier(t, fx, editBaseDoc)
		_, err := applier.begin()
		require.NoError(t, err)

		op := json.RawMessage(`{"op":"replaceText","find":"report","replace":"report"}`)
		for i := 0; i < 50; i++ {
			require.NoError(t, applier.apply(i, op))
		}
		_, err = applier.currentDoc()

		require.NoError(t, err)
		assert.Equal(t, 2, applier.marshalCount,
			"50 id-less replaceText ops must cost begin + final renders only")
	})

	t.Run("structural ops still rebuild the view per op", func(t *testing.T) {
		// the contrast pin: two moveBlocks cost begin + one rebuild (for the
		// second op's view) + the final after-document — the render-work
		// bound counts exactly these ops (v2OpRebuildsView)
		fx := newV2Fixture(t)
		applier := newApplier(t, fx, editBaseDoc)
		_, err := applier.begin()
		require.NoError(t, err)

		op := json.RawMessage(`{"op":"moveBlock","id":"blockParent1"}`)
		for i := 0; i < 2; i++ {
			require.NoError(t, applier.apply(i, op))
		}
		_, err = applier.currentDoc()

		require.NoError(t, err)
		assert.Equal(t, 3, applier.marshalCount)
	})
}

// TestPatchExcludesSystemManagedObjects is F3: the canUpdateObject switch in
// checkEditPreconditions lost its only coverage when PUT went away. It must
// FAIL if the switch is deleted — the assertion is on the refusal itself,
// and the mutator carries no expectation, so a PATCH that got through would
// blow up on the mock too.
func TestPatchExcludesSystemManagedObjects(t *testing.T) {
	ctx := context.Background()
	for _, sbType := range []model.SmartBlockType{
		model.SmartBlockType_STRelation,
		model.SmartBlockType_STRelationOption,
		model.SmartBlockType_FileObject,
		model.SmartBlockType_Participant,
	} {
		t.Run(sbType.String(), func(t *testing.T) {
			fx := newV2Fixture(t)
			read := editRead(t, editBaseDoc)
			read.SbType = sbType
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()

			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"updateBlock","id":"blockChild1","set":{"text":"edited"}}`), "", false)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			assert.Contains(t, apiErr.Message, "system-managed")
			assert.Contains(t, apiErr.Message, sbType.String())
		})
	}
}
