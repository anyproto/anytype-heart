package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	testSpaceId = "space1"
)

type v2Fixture struct {
	*V2Service
	mwMock      *mock_apicore.MockClientCommands
	readerMock  *mock_apicore.MockObjectReader
	objectStore *objectstore.StoreFixture
}

// newV2FixtureBare builds the service with an empty tech space (no space
// registered). Used by the ListSpaces tests, which manage the tech space's
// spaceViews themselves.
func newV2FixtureBare(t *testing.T) *v2Fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	readerMock := mock_apicore.NewMockObjectReader(t)
	objectStore := objectstore.NewStoreFixture(t)
	return &v2Fixture{
		V2Service:   NewV2Service(mwMock, readerMock, objectStore, objectstore.TestTechSpaceId),
		mwMock:      mwMock,
		readerMock:  readerMock,
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
		want := []apimodel.V2OutlineEntry{
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
		var got []apimodel.V2OutlineEntry
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
		var v2Err *apimodel.V2Error
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
		want := []apimodel.V2ObjectRow{
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
