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
		out, err := json.Marshal(stateDoc(t, *captured))
		require.NoError(t, err)
		assert.Contains(t, string(out), `"0000000000000000000dddd1"`, "the descendant keeps its stored id")
		assert.NotContains(t, string(out), `"dddd1",`, "the label was not adopted as an id")
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

	t.Run("a payload id naming a block outside the replaced subtree is a duplicate", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"replaceSubtree","id":"blockParent1","blocks":[{"id":"blockHeading1","type":"paragraph","text":"x"}]}`), "", false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "already exists in the document")
	})
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
