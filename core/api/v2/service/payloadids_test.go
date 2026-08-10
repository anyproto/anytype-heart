package v2service

// payloadids_test.go pins F1 of the post-PUT-removal audit (APIV2.md
// §8.29): a PATCH payload's id slots resolve exactly like its reference
// slots, so a document echoed back from a read keeps its identity.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// editMintedTableDoc is a 2x2 table whose table, row and column ids are all
// minted-shaped (24 lowercase hex), so a default read relabels them and the
// echo loop is a real one.
//
// The two column ids do NOT relabel, and that is the §8.28 property-1
// collision guard at work rather than an oversight: a cell's derived id is
// `rowId-colId`, so every cell shares its column's 5-char tail and the
// census inside mintedSuffixLabels sees "0cc11" twice. Property 1 says a
// contested tail keeps every claimant full — asserted below.
const editMintedTableDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"00000000000000000000aaa1","type":"table",` +
	`"columns":[{"id":"00000000000000000000cc11"},{"id":"00000000000000000000cc22"}],` +
	`"rows":[{"id":"00000000000000000000dd11","isHeader":true,"cells":["Name","Status"]},` +
	`{"id":"00000000000000000000dd22","cells":["Export"]}]}]}`

// editSharedViewIdDoc is a page with two inline dataviews whose views share
// the id "default" — the shape the app itself produces (every set,
// collection and type mints its first view as "default", and creating an
// inline set from one copies its views verbatim). View ids are unique within
// a dataview BLOCK, not document-wide (SPEC §6.2), so this is valid.
const editSharedViewIdDoc = `{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc"},"blocks":[` +
	`{"id":"dvFirst1","type":"dataview","properties":[{"key":"name","format":"text"}],"views":[{"id":"default","name":"First","columns":[{"property":"name"}]}]},` +
	`{"id":"dvSecond2","type":"dataview","properties":[{"key":"name","format":"text"}],"views":[{"id":"default","name":"Second","columns":[{"property":"name"}]}]}]}`

// editTailCollisionDoc holds two minted block ids sharing their last five
// characters — the §8.28 property-1 case with no cells involved.
const editTailCollisionDoc = `{"version":1,"id":"obj1","type":"page","blocks":[` +
	`{"id":"1111111111111111117ffff9","type":"paragraph","text":"one"},` +
	`{"id":"2222222222222222227ffff9","type":"paragraph","text":"two"}]}`

// readObject wires the reader for a plain GET and returns the served body.
func (fx *v2Fixture) readObject(t *testing.T, doc string, q V2ObjectQuery) []byte {
	t.Helper()
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, doc), nil).Maybe()
	body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", q)
	require.NoError(t, err)
	return body
}

// envelopeField extracts one raw field of a served envelope.
func envelopeField(t *testing.T, body []byte, field string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	raw, ok := fields[field]
	require.True(t, ok, "the read serves %q", field)
	return raw
}

// blockIdsOf lists the ids of a marshaled document's blocks array.
func blockIdsOf(blocks []map[string]any) []string {
	ids := make([]string, len(blocks))
	for i, b := range blocks {
		ids[i] = blockId(b)
	}
	return ids
}

// TestPatchPayloadIdsResolve is F1: the payload id slots that used to take a
// served id LITERALLY. Each case is the documented loop — read the object,
// echo what it served back into an op — and asserts the stored ids are
// UNCHANGED and diffStats reports the truth (a preserved-identity replace is
// not "added + removed").
func TestPatchPayloadIdsResolve(t *testing.T) {
	ctx := context.Background()

	t.Run("replaceSubtree echoing a compact ?block= read is a no-op", func(t *testing.T) {
		// before: 200 with blocksAdded/blocksRemoved, and the stored 24-hex
		// id permanently replaced by the 5-char label
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editMintedDoc), "headB")
		served := fx.readObject(t, editMintedDoc, V2ObjectQuery{Block: "aaaa1"})
		blocks := envelopeField(t, served, "blocks")
		require.Contains(t, string(blocks), `"id":"aaaa1"`, "the default read serves the compact label")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			[]byte(`{"ops":[{"op":"replaceSubtree","id":"aaaa1","blocks":`+string(blocks)+`}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{}, result.DiffStats, "echoing a subtree back unchanged is a genuine no-op")
		assert.Empty(t, result.CreatedBlocks, "a resolved payload id created nothing")
		assert.Equal(t, []string{testMintedHeadingId, testMintedParentId},
			blockIdsOf(docBlocks(stateDoc(t, *captured))), "the stored ids survive the echo")
	})

	t.Run("replaceSubtree still edits through the label", func(t *testing.T) {
		// identity preserved is not "nothing happened": the same op with
		// edited text changes the block in place
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editMintedDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"aaaa1","blocks":[{"id":"aaaa1","type":"heading1","text":"Renamed"}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		blocks := docBlocks(stateDoc(t, *captured))
		assert.Equal(t, testMintedHeadingId, blockId(blocks[0]))
		assert.Equal(t, "Renamed", blocks[0]["text"])
	})

	t.Run("updateBlock set.rows echoing a compact read keeps the row ids", func(t *testing.T) {
		// before: 200 reported as the innocuous BlocksChanged:1 while every
		// row id became its label
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editMintedTableDoc), "headB")
		served := fx.readObject(t, editMintedTableDoc, V2ObjectQuery{})
		table := docBlocks(mustDoc(t, served))[0]
		rows, err := json.Marshal(table["rows"])
		require.NoError(t, err)
		require.Contains(t, string(rows), `"id":"0dd11"`, "the default read serves relabeled row ids")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			[]byte(`{"ops":[{"op":"updateBlock","id":"0aaa1","set":{"rows":`+string(rows)+`}}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{}, result.DiffStats)
		stored := docBlocks(stateDoc(t, *captured))[0]
		assert.Equal(t, []any{"00000000000000000000dd11", "00000000000000000000dd22"}, rowIdsOf(stored))
	})

	t.Run("setCell value echoing a compact read keeps the cell descendant id", func(t *testing.T) {
		// cell descendants are served relabeled and are the one id domain no
		// reference slot addresses (F4) — so the echo was the only way to
		// reach them, and it renamed them
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTableCellChildDoc), "headB")
		served := fx.readObject(t, editTableCellChildDoc, V2ObjectQuery{})
		cell, err := json.Marshal(firstCell(t, served))
		require.NoError(t, err)
		require.Contains(t, string(cell), `"id":"dddd1"`, "the default read serves the relabeled descendant")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			[]byte(`{"ops":[{"op":"setCell","tableId":"tblOne1","row":"rowA","col":"colA","value":`+string(cell)+`}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{}, result.DiffStats)
		// assert the PARSED descendant id, not a substring of the marshaled
		// document: `NotContains("dddd1,")` only held because `id` happens
		// not to be the last key of a block object
		assert.Equal(t, []string{"0000000000000000000dddd1"},
			cellDescendantIds(t, docBlocks(stateDoc(t, *captured))[0]),
			"the descendant keeps its stored id — the label was not adopted")
	})

	t.Run("a payload id matching nothing is refused, never minted over", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockParent1","blocks":[{"id":"notAnId","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].blocks[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "omit id", "the way to author new content is named (C6)")
	})

	t.Run("an ambiguous payload id suffix is a 400 naming the candidates", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTailCollisionDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"1111111111111111117ffff9","blocks":[{"id":"7ffff9","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "1111111111111111117ffff9")
		assert.Contains(t, apiErr.Issues[0].Message, "2222222222222222227ffff9")
	})

	t.Run("omitting id stays the way to author new content", func(t *testing.T) {
		// including inside a table: a new row without an id is minted, and a
		// resolved sibling row keeps its identity in the same payload
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTableDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"tblOne1","set":{"rows":[`+
				`{"id":"rowH","isHeader":true,"cells":["Name","Status"]},`+
				`{"id":"rowB","cells":["Export"]},`+
				`{"cells":["Fresh"]}]}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		stored := docBlocks(stateDoc(t, *captured))[0]
		ids := rowIdsOf(stored)
		require.Len(t, ids, 3)
		assert.Equal(t, []any{"rowH", "rowB"}, ids[:2], "named rows keep their identity")
		assert.NotEmpty(t, ids[2], "the unnamed row got a minted id")
	})

	t.Run("authoring a whole new table through insertBlocks needs no ids", func(t *testing.T) {
		// the capability the refuse-unmatched decision is measured against:
		// nothing a caller AUTHORS has to name an id, tables included
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","blocks":[{"type":"table",`+
				`"columns":[{},{}],`+
				`"rows":[{"isHeader":true,"cells":["Name","Status"]},{"cells":["Export","Done"]}]}]}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksAdded: 1}, result.DiffStats)
		table := docBlocks(stateDoc(t, *captured))[4]
		assert.Equal(t, "table", blockType(table))
		for _, id := range rowIdsOf(table) {
			assert.NotEmpty(t, id, "row ids are minted")
		}
	})

	t.Run("an id nested in an insertBlocks payload is refused as not part of the op", func(t *testing.T) {
		// §8.30: the nested id slots go the same way as the block's own —
		// the row id below names a REAL row of the document, so before the
		// split it resolved and then failed as a duplicate, which reads as a
		// value problem rather than as "this op has no such field"
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTableDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","blocks":[{"type":"table",`+
				`"columns":[{}],"rows":[{"id":"rowH","cells":["Fresh"]}]}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].blocks[0].rows[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "not part of this op")
	})

	t.Run("a payload id naming a block outside the replaced subtree is a duplicate", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockParent1","blocks":[{"id":"blockHeading1","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].blocks[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "already exists in the document")
	})

	t.Run("a duplicate reached through a compact label names both spellings", func(t *testing.T) {
		// the payloadIdOrigin diagnosis, written for exactly this case and
		// never exercised: every other duplicate test writes a full literal
		// id, so no origin is recorded and the message stays generic. Here
		// "bbbb1" is a label off a default read — it READS as a fresh id but
		// resolves to the block it labels, which is the whole confusion the
		// message exists to name.
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editMintedDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"aaaa1","blocks":[{"id":"bbbb1","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"bbbb1"`, "the spelling the caller wrote")
		assert.Contains(t, apiErr.Issues[0].Message, testMintedParentId, "the id it resolves to")
		assert.Contains(t, apiErr.Issues[0].Message, "compact label off a default read")
	})
}

// TestPatchPayloadIdSeam pins §8.31: the two halves of the id rule — what an
// id may NAME (the resolver, whose vocabulary is doc.localIds()) and what an
// id may CLAIM (claimPayloadIds, whose domain was a.st.Exists) — now agree
// about what exists. They did not, and dataview VIEW ids fell through the
// gap: not blocks, so st.Exists could not see them, but doc-local, so the
// resolver could.
func TestPatchPayloadIdSeam(t *testing.T) {
	ctx := context.Background()

	t.Run("two views cannot share an id in one payload", func(t *testing.T) {
		// before: 200, and the document stored TWO views under "viewAll1" —
		// a later updateView renamed only the first, so every subsequent view
		// op addressed view #1 forever. Columns and rows in the identical
		// position were refused correctly; views were the only slot claiming
		// nothing.
		//
		// The FORMAT owns this one now (anyblockjson checkDataviewViews), so
		// the fragment import refuses it before the API's guard is reached —
		// which is the point of fixing it at the format layer: create and
		// import are closed too, not just PATCH.
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editSetDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"dataview","set":{"views":[`+
				`{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},`+
				`{"id":"viewAll1","name":"Second","columns":[{"property":"name"}]}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].set[0].views.1.id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `duplicate view id "viewAll1"`)
	})

	t.Run("a view cannot adopt a live block's id", func(t *testing.T) {
		// the other half of the same seam, and the half the format cannot
		// see: view ids are unique per dataview block (§6.2), so a view
		// wearing a BLOCK's id is format-legal — it is the API's own
		// vocabulary that is document-wide (one relabel pool, one payload
		// resolver), so this is the API's guard to keep
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoDataviewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"dvFirst1","set":{"views":[`+
				`{"id":"blockPara1","name":"All","columns":[{"property":"name"}]}]}}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].set.views[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "already exists in the document")
	})

	t.Run("a block cannot adopt a live view's id", func(t *testing.T) {
		// before: 200, storing a paragraph whose id equals a live view's.
		// Nothing broke immediately — block refs and view refs resolve
		// against different lists — but the document held an identity
		// collision no read can distinguish.
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoDataviewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockPara1","blocks":[{"id":"viewA1","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].blocks[0].id", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "already exists in the document")
	})

	t.Run("a block cannot adopt a view's id through a suffix either", func(t *testing.T) {
		// the compact-label route: a view id is in the relabel pool (§9a), so
		// a suffix of one resolves through the payload vocabulary exactly
		// like a block label does
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editTwoDataviewsDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockPara1","blocks":[{"id":"ewA1","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"ewA1"`, "the spelling the caller wrote")
		assert.Contains(t, apiErr.Issues[0].Message, "viewA1", "the view it resolves to")
	})

	t.Run("echoing a dataview's own views back keeps them", func(t *testing.T) {
		// the guard must not swing the other way: the ids an op REPLACES are
		// the ids it may reuse, and a dataview's views go with the block, so
		// collectSubtreeIds has to carry them
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTwoViewsDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"dataview","set":{"views":[`+
				`{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},`+
				`{"id":"viewBoard2","name":"Board","type":"kanban","groupBy":"severity","columns":[{"property":"name"},{"property":"severity","hidden":true}]}]}}`), "", false)

		require.NoError(t, err)
		assert.Empty(t, result.CreatedViews, "a resolved view id created nothing")
		assert.Equal(t, []string{"viewAll1", "viewBoard2"},
			viewIdsOf(t, dataviewOf(t, *captured, "dataview")), "the stored view ids survive the echo")
	})

	t.Run("two dataviews may hold one view id — the scope is the block", func(t *testing.T) {
		// SPEC §6.2: view ids are unique WITHIN a dataview block. The app
		// itself makes this case — every set/collection/type mints its
		// default view as "default", and creating an inline set copies those
		// views verbatim — so a document-wide domain would refuse a real edit
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSharedViewIdDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateView","block":"dvSecond2","view":"default","set":{"name":"Renamed"}}`), "", false)

		require.NoError(t, err)
		assert.Equal(t, v2model.DiffStats{BlocksChanged: 1}, result.DiffStats)
		assert.Equal(t, []string{"default"}, viewIdsOf(t, dataviewOf(t, *captured, "dvSecond2")))
		assert.Equal(t, "Renamed", viewsOf(t, dataviewOf(t, *captured, "dvSecond2"))[0]["name"])
		assert.Equal(t, "First", viewsOf(t, dataviewOf(t, *captured, "dvFirst1"))[0]["name"],
			"the other block's identically-named view is untouched")
	})
}

// TestPatchReportsMintedNestedIds pins the receipt the refusals promise. Both
// unresolvedPayloadIdError and newContentIdError tell the caller to omit the
// id because the server mints one and reports it — but createdBlocks used to
// be written only for TOP-LEVEL run blocks, so exactly the slots those
// refusals fire on (a table's rows and columns, a cell descendant, a view)
// went unreported and the caller had to re-read to learn an id it had just
// created.
func TestPatchReportsMintedNestedIds(t *testing.T) {
	ctx := context.Background()

	t.Run("a table created through insertBlocks reports its row and column ids", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insertBlocks","blocks":[{"type":"table",`+
				`"columns":[{},{}],`+
				`"rows":[{"isHeader":true,"cells":["Name","Status"]},{"cells":["Export","Done"]}]}]}`), "", false)

		require.NoError(t, err)
		table := docBlocks(stateDoc(t, *captured))[4]
		assert.Equal(t, result.CreatedBlocks["ops[0].blocks[0]"], blockId(table))
		assert.Equal(t, []any{result.CreatedBlocks["ops[0].blocks[0].rows[0]"],
			result.CreatedBlocks["ops[0].blocks[0].rows[1]"]}, rowIdsOf(table))
		assert.Equal(t, []any{result.CreatedBlocks["ops[0].blocks[0].columns[0]"],
			result.CreatedBlocks["ops[0].blocks[0].columns[1]"]}, columnIdsOf(table))
		for key, id := range result.CreatedBlocks {
			assert.NotEmpty(t, id, key)
		}
	})

	t.Run("a minted cell descendant is reported under its slot", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editTableDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"setCell","tableId":"tblOne1","row":"rowB","col":"colA",`+
				`"value":[{"type":"toggle","text":"cell"},{"indent":1,"type":"paragraph","text":"inside"}]}`), "", false)

		require.NoError(t, err)
		minted := result.CreatedBlocks["ops[0].value[1]"]
		require.NotEmpty(t, minted, "the cell descendant the op minted is reported")
		assert.Contains(t, cellDescendantIds(t, docBlocks(stateDoc(t, *captured))[0]), minted)
	})

	t.Run("a view minted through updateBlock is reported in createdViews", func(t *testing.T) {
		// a view is not a block, so it lands in the view-family map — the
		// same map insertView already reports through
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editSetDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"updateBlock","id":"dataview","set":{"views":[`+
				`{"id":"viewAll1","name":"All","columns":[{"property":"name"}]},`+
				`{"name":"Fresh","columns":[{"property":"name"}]}]}}`), "", false)

		require.NoError(t, err)
		minted := result.CreatedViews["ops[0].set.views[1]"]
		require.NotEmpty(t, minted, "the minted view id is reported under its payload slot")
		assert.Equal(t, []string{"viewAll1", minted},
			viewIdsOf(t, dataviewOf(t, *captured, "dataview")))
		assert.Empty(t, result.CreatedBlocks, "a view is not reported as a block")
	})
}

// cellDescendantIds lists the ids of every cell-run descendant of a table
// block (the F10 array form's elements past the cell block itself).
func cellDescendantIds(t *testing.T, table map[string]any) []string {
	t.Helper()
	var out []string
	rows, _ := table["rows"].([]any)
	for _, r := range rows {
		row, _ := r.(map[string]any)
		cells, _ := row["cells"].([]any)
		for _, c := range cells {
			run, isRun := c.([]any)
			if !isRun {
				continue
			}
			for i := 1; i < len(run); i++ {
				el, _ := run[i].(map[string]any)
				out = append(out, blockId(el))
			}
		}
	}
	return out
}

// columnIdsOf lists a marshaled table block's column ids.
func columnIdsOf(table map[string]any) []any {
	columns, _ := table["columns"].([]any)
	out := make([]any, 0, len(columns))
	for _, c := range columns {
		m, _ := c.(map[string]any)
		out = append(out, m["id"])
	}
	return out
}

// viewIdsOf lists a marshaled dataview block's view ids.
func viewIdsOf(t *testing.T, dv map[string]any) []string {
	t.Helper()
	views := viewsOf(t, dv)
	out := make([]string, len(views))
	for i, v := range views {
		out[i], _ = v["id"].(string)
	}
	return out
}

// mustDoc decodes a served envelope into its generic map form.
func mustDoc(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	return doc
}

// rowIdsOf lists a marshaled table block's row ids.
func rowIdsOf(table map[string]any) []any {
	rows, _ := table["rows"].([]any)
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		m, _ := r.(map[string]any)
		out = append(out, m["id"])
	}
	return out
}

// firstCell returns the first cell of the first row of the first block.
func firstCell(t *testing.T, body []byte) any {
	t.Helper()
	table := docBlocks(mustDoc(t, body))[0]
	rows, _ := table["rows"].([]any)
	require.NotEmpty(t, rows)
	row, _ := rows[0].(map[string]any)
	cells, _ := row["cells"].([]any)
	require.NotEmpty(t, cells)
	return cells[0]
}
