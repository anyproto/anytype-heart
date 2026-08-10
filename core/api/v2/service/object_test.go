package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	testSpaceId = "space1"
	// testAccountId feeds the §6.2 current-user placeholder substitution
	// (Phase 4): _participant_<space>_<account>.
	testAccountId = "accountA"
)

type v2Fixture struct {
	*V2Service
	mwMock      *mock_apicore.MockClientCommands
	readerMock  *mock_apicore.MockObjectReader
	creatorMock *mock_apicore.MockObjectCreator
	mutatorMock *mock_apicore.MockObjectMutator
	objectStore *objectstore.StoreFixture
}

// newV2FixtureBare builds the service with an empty tech space (no space
// registered). Used by the ListSpaces tests, which manage the tech space's
// spaceViews themselves.
func newV2FixtureBare(t *testing.T) *v2Fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	readerMock := mock_apicore.NewMockObjectReader(t)
	creatorMock := mock_apicore.NewMockObjectCreator(t)
	mutatorMock := mock_apicore.NewMockObjectMutator(t)
	objectStore := objectstore.NewStoreFixture(t)
	return &v2Fixture{
		V2Service:   NewV2Service(mwMock, readerMock, creatorMock, mutatorMock, objectStore, objectstore.TestTechSpaceId, testAccountId),
		mwMock:      mwMock,
		readerMock:  readerMock,
		creatorMock: creatorMock,
		mutatorMock: mutatorMock,
		objectStore: objectStore,
	}
}

// newV2Fixture builds the service with the default test space registered, so
// the C2 ensureSpace guard resolves testSpaceId as a real space.
func newV2Fixture(t *testing.T) *v2Fixture {
	fx := newV2FixtureBare(t)
	fx.registerSpace(t, testSpaceId)
	return fx
}

// registerSpace adds a spaceView for spaceId to the tech space so
// ensureSpace (via GetSpaceViewDetails) resolves it.
func (fx *v2Fixture) registerSpace(t *testing.T, spaceId string) {
	fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("spaceView_" + spaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
		},
	})
}

func textContent(text string, style model.BlockContentTextStyle) *model.BlockContentOfText {
	return &model.BlockContentOfText{Text: &model.BlockContentText{Text: text, Style: style}}
}

// testObjectRead builds a live read of a small document:
// heading "Section", paragraph "parent" with child "child", then "sibling".
func testObjectRead() apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("obj1"),
				"name": pbtypes.String("Doc"),
			}},
			ObjectTypes: []string{"ot-page"},
			Blocks: []*model.Block{
				{Id: "obj1", ChildrenIds: []string{"h1", "p1", "p3"},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: "h1", Content: textContent("Section", model.BlockContentText_Header1)},
				{Id: "p1", ChildrenIds: []string{"p2"}, Content: textContent("parent", model.BlockContentText_Paragraph)},
				{Id: "p2", Content: textContent("child", model.BlockContentText_Paragraph)},
				{Id: "p3", Content: textContent("sibling", model.BlockContentText_Paragraph)},
			},
		},
		Heads: []string{"headB", "headA"},
	}
}

// Minted-shape (24-hex) block ids for the relabeling fixtures — only
// machine-minted opaque ids relabel, so a fixture that wants a real (not
// identity) relabel operation must mint like the editor does.
const (
	testMintedHeadingId = "0000000000000000000aaaa1" // label "aaaa1"
	testMintedParentId  = "0000000000000000000bbbb1" // label "bbbb1"
	testMintedChildId   = "0000000000000000000cccc1" // label "cccc1"
	testMintedSiblingId = "0000000000000000000dddd1" // label "dddd1"
	testMintedLinkId    = "0000000000000000000eeee1" // label "eeee1"
)

// testObjectReadLongIds mirrors testObjectRead but with minted-shape block
// ids, so relabeling is a real (not identity) operation — the case the
// short-id fixtures cannot exercise (M1).
func testObjectReadLongIds() apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_Page,
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("obj1"),
				"name": pbtypes.String("Doc"),
			}},
			ObjectTypes: []string{"ot-page"},
			Blocks: []*model.Block{
				{Id: "obj1", ChildrenIds: []string{testMintedHeadingId, testMintedParentId, testMintedSiblingId},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: testMintedHeadingId, Content: textContent("Section", model.BlockContentText_Header1)},
				{Id: testMintedParentId, ChildrenIds: []string{testMintedChildId}, Content: textContent("parent", model.BlockContentText_Paragraph)},
				{Id: testMintedChildId, Content: textContent("child", model.BlockContentText_Paragraph)},
				{Id: testMintedSiblingId, Content: textContent("sibling", model.BlockContentText_Paragraph)},
			},
		},
		Heads: []string{"headB", "headA"},
	}
}

func TestV2OutlineBlockRoundTrip(t *testing.T) {
	// M1: a compact outline label must resolve in a follow-up ?block= read —
	// the core outline-then-fetch flow on a large document.
	fx := newV2Fixture(t)
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectReadLongIds(), nil).Times(2)

	// outline read yields compact block labels (not the full ids)
	outlineBody, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Outline: true})
	require.NoError(t, err)
	entries := decodeBody(t, outlineBody)["outline"].([]any)
	require.Len(t, entries, 4)
	label := entries[1].(map[string]any)["id"].(string) // the parent paragraph
	require.NotEqual(t, testMintedParentId, label, "outline must emit a compact label, not the full id")
	require.True(t, strings.HasSuffix(testMintedParentId, label), "label is the id suffix")

	// the label round-trips: ?block=<label> returns the parent + its child,
	// under the SAME labels (the subtree read is the edit shape too, so the
	// ids an agent sees never change spelling between the two calls)
	blockBody, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Block: label})
	require.NoError(t, err, "the outline label must resolve in ?block=")
	blocks := decodeBody(t, blockBody)["blocks"].([]any)
	require.Len(t, blocks, 2, "subtree = parent + child, not the sibling")
	assert.Equal(t, label, blocks[0].(map[string]any)["id"])
	assert.Equal(t, "cccc1", blocks[1].(map[string]any)["id"])

	// and the export shape still spells them in full, so a GET body PUTs
	// back as a minimal diff (APIV2.md §3(b))
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectReadLongIds(), nil).Once()
	fullBody, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Ids: V2IdsFull})
	require.NoError(t, err)
	fullBlocks := decodeBody(t, fullBody)["blocks"].([]any)
	require.Len(t, fullBlocks, 4)
	assert.Equal(t, testMintedParentId, fullBlocks[1].(map[string]any)["id"])
}

// TestV2MixedIdPopulationsRoundTrip is the review's coverage-hole fixture:
// three id populations in ONE document — a minted 24-hex id (relabels), a
// hyphenated readable id and a short id (both stay full) — asserting which
// come back relabeled, that ?block= resolves BOTH spellings for each, and
// that the served document passes anyblockjson.Validate. Mixed documents are
// exactly where the aliasing and adoption defects lived.
//
// The short id is "b1" and sits FIRST in document order — deliberately a
// suffix of the minted parent's stored id ("…bbbb1" ends with "b1"). The
// stored-id fallback of ?block= used to map the matched stored id back to a
// served spelling with a first-match-wins suffix scan, which served THIS
// decoy instead of the minted block; the fixture's earlier ordering (short
// id last, not a suffix) could not catch that.
func TestV2MixedIdPopulationsRoundTrip(t *testing.T) {
	mixedRead := func() apicore.ObjectRead {
		return apicore.ObjectRead{
			SbType: model.SmartBlockType_Page,
			Snapshot: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					"id": pbtypes.String("obj1"), "name": pbtypes.String("Mixed"),
				}},
				ObjectTypes: []string{"ot-page"},
				Blocks: []*model.Block{
					{Id: "obj1", ChildrenIds: []string{"b1", testMintedParentId, "pages-roadmap-home-1"},
						Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
					{Id: "b1", Content: textContent("short decoy", model.BlockContentText_Paragraph)},
					{Id: testMintedParentId, ChildrenIds: []string{testMintedChildId}, Content: textContent("minted", model.BlockContentText_Paragraph)},
					{Id: testMintedChildId, Content: textContent("minted child", model.BlockContentText_Paragraph)},
					{Id: "pages-roadmap-home-1", Content: textContent("readable", model.BlockContentText_Paragraph)},
				},
			},
			Heads: []string{"headA"},
		}
	}
	fx := newV2Fixture(t)
	// one read for the default shape + one per ?block= case below
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(mixedRead(), nil).Times(6)

	// the default read: minted ids relabel, the others serve verbatim — and
	// the served document stays schema-valid net of the v2 envelope
	// additions every write path strips (etag/warnings — normalizeCreateBody)
	body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{})
	require.NoError(t, err)
	stripped := decodeBody(t, body)
	delete(stripped, "etag")
	delete(stripped, "warnings")
	strippedJSON, err := json.Marshal(stripped)
	require.NoError(t, err)
	require.NoError(t, anyblockjson.Validate(strippedJSON), "the served default shape must validate")
	var served []string
	for _, b := range decodeBody(t, body)["blocks"].([]any) {
		served = append(served, b.(map[string]any)["id"].(string))
	}
	assert.Equal(t, []string{"b1", "bbbb1", "cccc1", "pages-roadmap-home-1"}, served)

	// ?block= resolves BOTH spellings for each population: the served
	// spelling and the stored one. The stored-spelling case is the trap: the
	// decoy "b1" tails testMintedParentId and comes first in document order.
	for _, tc := range []struct {
		ref       string
		wantFirst string // the served id of the subtree's anchor
		wantLen   int
	}{
		{ref: "bbbb1", wantFirst: "bbbb1", wantLen: 2},                               // minted, by label
		{ref: testMintedParentId, wantFirst: "bbbb1", wantLen: 2},                    // minted, by full id (the decoy trap)
		{ref: "pages-roadmap-home-1", wantFirst: "pages-roadmap-home-1", wantLen: 1}, // readable, full
		{ref: "home-1", wantFirst: "pages-roadmap-home-1", wantLen: 1},               // readable, unique suffix
		{ref: "b1", wantFirst: "b1", wantLen: 1},                                     // short id, exact
	} {
		sub, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Block: tc.ref})
		require.NoErrorf(t, err, "?block=%s must resolve", tc.ref)
		blocks := decodeBody(t, sub)["blocks"].([]any)
		require.Lenf(t, blocks, tc.wantLen, "?block=%s subtree size", tc.ref)
		assert.Equalf(t, tc.wantFirst, blocks[0].(map[string]any)["id"], "?block=%s anchor", tc.ref)
	}
}

// TestV2BlockRefStoredIdFallback pins the ?block= stored-id fallback on the
// reproduced wrong-block shape: blocks "b1" (short, first in document order)
// and 000000000000000000009ab1 (served as "09ab1"). "b1" is a suffix of the
// minted block's stored id, so the retired suffix scan mapped
// ?block=<full stored id> onto "b1" — the WRONG block, served 200 with the
// wrong content. The mapping is positional now (stored[i] belongs to served
// blocks[i]); a stored id that is never served — the root here — is a 404,
// not a scan hit.
func TestV2BlockRefStoredIdFallback(t *testing.T) {
	const mintedId = "000000000000000000009ab1" // label "09ab1"; "b1" tails it
	trapRead := func() apicore.ObjectRead {
		return apicore.ObjectRead{
			SbType: model.SmartBlockType_Page,
			Snapshot: &model.SmartBlockSnapshotBase{
				Details: &types.Struct{Fields: map[string]*types.Value{
					"id": pbtypes.String("obj1"), "name": pbtypes.String("Trap"),
				}},
				ObjectTypes: []string{"ot-page"},
				Blocks: []*model.Block{
					{Id: "obj1", ChildrenIds: []string{"b1", mintedId},
						Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
					{Id: "b1", Content: textContent("decoy", model.BlockContentText_Paragraph)},
					{Id: mintedId, Content: textContent("target", model.BlockContentText_Paragraph)},
				},
			},
			Heads: []string{"headA"},
		}
	}
	fx := newV2Fixture(t)
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(trapRead(), nil).Times(2)

	// the full stored id resolves to ITS block, not the earlier suffix decoy
	sub, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Block: mintedId})
	require.NoError(t, err)
	blocks := decodeBody(t, sub)["blocks"].([]any)
	require.Len(t, blocks, 1)
	block := blocks[0].(map[string]any)
	assert.Equal(t, "09ab1", block["id"], "?block=<full stored id> must serve the block that OWNS the id")
	assert.Equal(t, "target", block["text"])

	// the root's stored id is never served — 404, not a silent scan hit
	_, _, err = fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Block: "obj1"})
	apiErr := v2Err(t, err)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
}

// TestFilterBlockSubtreeStoredIdFallback pins the fallback machinery in
// isolation: the served-vocabulary ambiguity refusal survives even when the
// stored vocabulary would resolve, and the stored resolution maps by
// position, not by suffix scanning the served ids.
func TestFilterBlockSubtreeStoredIdFallback(t *testing.T) {
	const minted = "000000000000000000009ab1"
	staticIds := func(ids []string) func() ([]string, error) {
		return func() ([]string, error) { return ids, nil }
	}

	t.Run("a served ambiguity stays a 400 — never silently resolved via stored ids", func(t *testing.T) {
		// two long served ids both end with the minted block's full stored
		// id, so the ref is ambiguous in the served vocabulary while the
		// stored vocabulary holds exactly one exact match
		fields := map[string]json.RawMessage{"blocks": json.RawMessage(`[
			{"id":"x` + minted + `","type":"paragraph","text":"a"},
			{"id":"y` + minted + `","type":"paragraph","text":"b"},
			{"id":"09ab1","type":"paragraph","text":"c"}]`)}
		err := filterBlockSubtree(fields, minted,
			staticIds([]string{"x" + minted, "y" + minted, minted}))
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
	})

	t.Run("stored resolution is positional", func(t *testing.T) {
		fields := map[string]json.RawMessage{"blocks": json.RawMessage(`[
			{"id":"b1","type":"paragraph","text":"decoy"},
			{"id":"09ab1","type":"paragraph","text":"target"}]`)}
		require.NoError(t, filterBlockSubtree(fields, minted,
			staticIds([]string{"b1", minted})))
		var run []map[string]any
		require.NoError(t, json.Unmarshal(fields["blocks"], &run))
		require.Len(t, run, 1)
		assert.Equal(t, "09ab1", run[0]["id"])
	})

	t.Run("a stored-id list out of step with the served blocks is an error, not a guess", func(t *testing.T) {
		fields := map[string]json.RawMessage{"blocks": json.RawMessage(`[
			{"id":"b1","type":"paragraph","text":"decoy"}]`)}
		err := filterBlockSubtree(fields, minted, staticIds([]string{"b1", minted}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stored-id fallback")
	})
}

// TestV2GetObjectCompactBody pins C3 for the WHOLE body, not just its
// envelope: anyblockjson.Marshal returns the format's canonical two-space
// indented bytes (SPEC §4) and the envelope re-embeds them verbatim, so every
// default read used to be compact on top and pretty-printed underneath —
// 16–26 % of the served tokens (TOKENS §1.1).
func TestV2GetObjectCompactBody(t *testing.T) {
	// given
	fx := newV2Fixture(t)
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)

	// when
	body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{})

	// then
	require.NoError(t, err)
	assert.NotContains(t, string(body), "\n", "no pretty-printing survives inside the envelope")
	assert.NotContains(t, string(body), `": "`, "no indent spacing after a key")
	require.Len(t, decodeBody(t, body)["blocks"].([]any), 4, "and it is still the same document")
}

// testLinkTargetId is a realistic 59-char object id — the thing the refs
// legend exists to shorten.
const testLinkTargetId = "bafyreih6ymjl42i6pevii77dnlulv4n52hsxmjflmwc5ttygotovbrcteq"

// testObjectReadWithRef adds a link block to the long-id fixture, so both id
// populations (doc-local block ids and cross-document object refs) are
// present in one read.
func testObjectReadWithRef() apicore.ObjectRead {
	read := testObjectReadLongIds()
	root := read.Snapshot.Blocks[0]
	root.ChildrenIds = append(root.ChildrenIds, testMintedLinkId)
	read.Snapshot.Blocks = append(read.Snapshot.Blocks, &model.Block{
		Id:      testMintedLinkId,
		Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: testLinkTargetId}},
	})
	return read
}

// TestV2GetObjectIdShapes pins the Wave-0 split of `?ids=` into two document
// SHAPES with opposite id economics (TOKENS §1.2/§10): the default edit read
// relabels block ids short and leaves object refs full inline, while the
// export read keeps block ids full and pays the lossless refs legend.
func TestV2GetObjectIdShapes(t *testing.T) {
	t.Run("default: short block labels, full inline object refs, no legend", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectReadWithRef(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		assert.NotContains(t, doc, "refs", "the refs legend costs more than it saves on real documents (TOKENS §1.2)")
		blocks := doc["blocks"].([]any)
		require.Len(t, blocks, 5)
		assert.Equal(t, "bbbb1", blocks[1].(map[string]any)["id"], "block ids relabel to their short suffix")
		assert.Equal(t, testLinkTargetId, blocks[4].(map[string]any)["objectId"], "object refs stay full inline — no legend hop to write one back")
	})

	t.Run("ids=full: full block ids AND full inline object refs — no legend", func(t *testing.T) {
		// given — the export shape used to carry the refs legend, which this
		// work's own measurement shows as a pure loss (§8.25's legend axis:
		// 0.9–11.5 % per document) and §8.25 itself calls a write-back trap;
		// no shape serves it now, so full block ids come without the
		// indirection
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectReadWithRef(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Ids: V2IdsFull})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		blocks := doc["blocks"].([]any)
		require.Len(t, blocks, 5)
		assert.Equal(t, testMintedParentId, blocks[1].(map[string]any)["id"], "the export shape must not relabel — relabeling is lossy")
		assert.NotContains(t, doc, "refs", "no shape serves the legend (input resolution stays total, SPEC §9a)")
		assert.Equal(t, testLinkTargetId, blocks[4].(map[string]any)["objectId"], "object refs are full inline")
	})

}

func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	return doc
}

func TestV2GetObject(t *testing.T) {
	t.Run("default read returns the full document with etag", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)
		wantEtag := ComputeEtag([]string{"headA", "headB"})

		// when
		body, etag, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{})

		// then
		require.NoError(t, err)
		assert.Equal(t, wantEtag, etag)
		doc := decodeBody(t, body)
		assert.Equal(t, wantEtag, doc["etag"])
		assert.Equal(t, "obj1", doc["id"])
		assert.Equal(t, "page", doc["type"])
		require.Contains(t, doc, "properties")
		blocks := doc["blocks"].([]any)
		require.Len(t, blocks, 4)
		// block ids stay full on default reads (C4)
		assert.Equal(t, "h1", blocks[0].(map[string]any)["id"])
	})

	t.Run("include=properties suppresses blocks", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Include: "properties"})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		assert.Contains(t, doc, "properties")
		assert.NotContains(t, doc, "blocks")
	})

	t.Run("include=blocks suppresses properties", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Include: "blocks"})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		assert.NotContains(t, doc, "properties")
		assert.Contains(t, doc, "blocks")
	})

	t.Run("outline returns every block, text only on headings", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)
		want := []v2model.OutlineEntry{
			{Indent: 0, Id: "h1", Type: "heading1", Text: "Section"},
			{Indent: 0, Id: "p1", Type: "paragraph"},
			{Indent: 1, Id: "p2", Type: "paragraph"},
			{Indent: 0, Id: "p3", Type: "paragraph"},
		}

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Outline: true})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		assert.NotContains(t, doc, "blocks")
		assert.NotContains(t, doc, "properties", "outline without include=properties has no properties map")
		raw, err := json.Marshal(doc["outline"])
		require.NoError(t, err)
		var got []v2model.OutlineEntry
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, want, got)
	})

	t.Run("outline with include=properties keeps the properties map", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Outline: true, Include: "properties"})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		assert.Contains(t, doc, "outline")
		assert.Contains(t, doc, "properties")
	})

	t.Run("block subtree read returns the contiguous indent-run", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)

		// when
		body, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Block: "p1"})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		blocks := doc["blocks"].([]any)
		require.Len(t, blocks, 2)
		assert.Equal(t, "p1", blocks[0].(map[string]any)["id"])
		assert.Equal(t, "p2", blocks[1].(map[string]any)["id"])
	})

	t.Run("unknown block id is a 404 with steering", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)

		// when
		_, _, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Block: "nope"})

		// then
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Contains(t, v2Err.Message, "outline=true")
	})

	t.Run("format=md returns the markdown envelope", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(testObjectRead(), nil)
		fx.mwMock.EXPECT().ObjectExport(mock.Anything, &pb.RpcObjectExportRequest{
			SpaceId:  testSpaceId,
			ObjectId: "obj1",
			Format:   model.Export_Markdown,
		}).Return(&pb.RpcObjectExportResponse{Result: "# Section\n\nparent\n"})

		// when
		body, etag, err := fx.GetObject(context.Background(), testSpaceId, "obj1", V2ObjectQuery{Format: "md"})

		// then
		require.NoError(t, err)
		doc := decodeBody(t, body)
		assert.Equal(t, "obj1", doc["id"])
		assert.Equal(t, "page", doc["type"])
		assert.Equal(t, etag, doc["etag"])
		assert.Equal(t, "# Section\n\nparent\n", doc["markdown"])
		assert.NotContains(t, doc, "blocks")
	})

	t.Run("read error surfaces wrapped", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "gone").
			Return(apicore.ObjectRead{}, assert.AnError)

		// when
		_, _, err := fx.GetObject(context.Background(), testSpaceId, "gone", V2ObjectQuery{})

		// then
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("a missing object maps to 404, not 500", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "gone").
			Return(apicore.ObjectRead{}, treestorage.ErrUnknownTreeId)

		// when
		_, _, err := fx.GetObject(context.Background(), testSpaceId, "gone", V2ObjectQuery{})

		// then
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
	})
}

func TestV2ListObjects(t *testing.T) {
	addListObjects := func(fx *v2Fixture, t *testing.T) {
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-task"),
				bundle.RelationKeyName:           domain.String("Task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:               domain.String("obj1"),
				bundle.RelationKeyName:             domain.String("Buy milk"),
				bundle.RelationKeyType:             domain.String("type-task"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_todo)),
				bundle.RelationKeyDone:             domain.Bool(true),
				bundle.RelationKeyLastModifiedDate: domain.Int64(200),
			},
			{
				bundle.RelationKeyId:               domain.String("obj2"),
				bundle.RelationKeyName:             domain.String("Older note"),
				bundle.RelationKeyType:             domain.String("type-task"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(100),
			},
		})
	}

	t.Run("rows carry id, name and the type key, newest first", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		addListObjects(fx, t)
		want := []v2model.ObjectRow{
			{Id: "obj1", Name: "Buy milk", Type: "task"},
			{Id: "obj2", Name: "Older note", Type: "task"},
		}

		// when
		rows, total, hasMore, err := fx.ListObjects(context.Background(), testSpaceId, nil, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("requested fields appear as property values", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		addListObjects(fx, t)

		// when
		rows, _, _, err := fx.ListObjects(context.Background(), testSpaceId, []string{"done"}, 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, map[string]any{"done": true}, rows[0].Properties)
		assert.Nil(t, rows[1].Properties, "absent property stays absent")
	})

	t.Run("pagination reports has_more", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		addListObjects(fx, t)

		// when
		rows, total, hasMore, err := fx.ListObjects(context.Background(), testSpaceId, nil, 0, 1)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "obj1", rows[0].Id)
		assert.Equal(t, 2, total)
		assert.True(t, hasMore)
	})
}
