package export

// collection_test.go covers the WHOLE-SPACE collection path — empty
// request ids, getExistedObjects — which every case in export_test.go
// bypasses by passing explicit ObjectIds. This is the branch where the
// closure mode actually gates object admission (objectValid), so an
// inverted closure mapping here would have been invisible to the rest of
// the suite.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// seedWholeSpace fills the store with one object per admission rule the two
// closures disagree about: a plain page (both take it), a relation object
// (derived only — validType admits it, validTypeForContentClosure does
// not), a collection-layout page (derived only — validLayoutForContentClosure
// refuses the layout), and an archived page (either, but only behind
// IncludeArchived). Returns the sbType each id resolves to.
func seedWholeSpace(t *testing.T, fx *fixture) map[string]smartblock.SmartBlockType {
	fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:      domain.String("pageId"),
			bundle.RelationKeyName:    domain.String("Plain page"),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		},
		prepareTestRelationForStore(t, "customRelation", int64(model.RelationFormat_longtext)),
		{
			bundle.RelationKeyId:             domain.String("collectionId"),
			bundle.RelationKeyName:           domain.String("A collection"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
			bundle.RelationKeySpaceId:        domain.String(spaceId),
		},
		{
			bundle.RelationKeyId:         domain.String("archivedId"),
			bundle.RelationKeyName:       domain.String("Archived page"),
			bundle.RelationKeyIsArchived: domain.Bool(true),
			bundle.RelationKeySpaceId:    domain.String(spaceId),
		},
	})
	types := map[string]smartblock.SmartBlockType{
		"pageId":         smartblock.SmartBlockTypePage,
		"customRelation": smartblock.SmartBlockTypeRelation,
		"collectionId":   smartblock.SmartBlockTypePage,
		"archivedId":     smartblock.SmartBlockTypePage,
	}
	fx.sbtProvider.EXPECT().Type(spaceId, mock.Anything).RunAndReturn(
		func(_ string, id string) (smartblock.SmartBlockType, error) {
			return types[id], nil
		}).Maybe()
	return types
}

// The content closure (md-style) admits pages and nothing derived; the
// derived closure admits everything validType lists. Both run through the
// same docsForExport the formats call, with no request ids — the branch
// nothing else covers.
//
// How this can fail: swap the closure arms in objectValid (the content run
// collects the relation and the derived run loses it — both assertions go
// red); drop the layout gate (the collection leaks into the content
// closure); or invert the IncludeArchived branch (the archived page
// appears without the flag).
func Test_docsForExport_WholeSpaceClosureRules(t *testing.T) {
	t.Run("content closure: pages only, no derived, no collection layouts", func(t *testing.T) {
		// given
		fx := newFixture(t)
		seedWholeSpace(t, fx)
		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId: spaceId,
			Format:  model.Export_Markdown,
		})

		// when
		err := expCtx.docsForExport(context.Background())

		// then
		require.NoError(t, err)
		assert.Contains(t, expCtx.docs, "pageId")
		assert.NotContains(t, expCtx.docs, "customRelation", "a relation object is derived-closure only")
		assert.NotContains(t, expCtx.docs, "collectionId", "a collection layout is derived-closure only")
		assert.NotContains(t, expCtx.docs, "archivedId", "archived needs the flag")
		assert.Len(t, expCtx.docs, 1)
	})

	t.Run("derived closure: relations and collections ride along, archived behind the flag", func(t *testing.T) {
		// given
		fx := newFixture(t)
		types := seedWholeSpace(t, fx)
		for id, sbType := range types {
			fx.picker.EXPECT().GetObject(context.Background(), id).
				Return(setupObject(id, "someType", sbType, nil), nil).Maybe()
		}
		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:         spaceId,
			Format:          model.Export_Protobuf,
			IncludeArchived: true,
		})

		// when
		err := expCtx.docsForExport(context.Background())

		// then
		require.NoError(t, err)
		assert.Contains(t, expCtx.docs, "pageId")
		assert.Contains(t, expCtx.docs, "customRelation")
		assert.Contains(t, expCtx.docs, "collectionId")
		assert.Contains(t, expCtx.docs, "archivedId")
	})

	t.Run("derived closure without the flag drops the archived page", func(t *testing.T) {
		// given
		fx := newFixture(t)
		types := seedWholeSpace(t, fx)
		for id, sbType := range types {
			fx.picker.EXPECT().GetObject(context.Background(), id).
				Return(setupObject(id, "someType", sbType, nil), nil).Maybe()
		}
		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId: spaceId,
			Format:  model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(context.Background())

		// then
		require.NoError(t, err)
		assert.NotContains(t, expCtx.docs, "archivedId")
		assert.Contains(t, expCtx.docs, "pageId")
	})
}
