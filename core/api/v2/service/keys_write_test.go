package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// The WRITE half's key vocabulary (§7.5a-5). Every import channel of this
// service rides creatingResolvers.Options(); before `Keys: r.reads` landed
// there the write half fell back to BundledKeyVocabulary while the read half
// exported through storeresolver, and the two halves named different entities.
//
// Every fixture here is deliberately BSON-keyed with a stored apiObjectKey, or
// a stored key that the bundled table resolves elsewhere: a fixture whose key
// the bundled table happens to invert (name, page, dueDate) cannot tell the
// two vocabularies apart and proves nothing.

// slugDataviewDoc is a set whose dataview names the BSON-keyed property by its
// STORED key — the shape a real space holds. editRead imports with the bundled
// vocabulary, so the stored spelling survives into the snapshot verbatim.
const slugDataviewDoc = `{"version":1,"id":"obj1","type":"set","properties":{"name":"Bugs","setOf":["ot-bug"]},"blocks":[` +
	`{"id":"dataview","type":"dataview",` +
	`"properties":[{"property":"name","format":"text"},{"property":"` + slugPropKey + `","format":"text"}],` +
	`"views":[{"id":"viewAll1","name":"All",` +
	`"columns":[{"property":"name"},{"property":"` + slugPropKey + `","hidden":true,"width":100}]}]}]}`

// TestV2WriteVocabularyIsTheReadVocabulary is the pin for the asymmetry:
// revert `Keys: r.reads` in creatingResolvers.Options() (resolver.go) and every
// subtest here fails.
func TestV2WriteVocabularyIsTheReadVocabulary(t *testing.T) {
	ctx := context.Background()

	t.Run("update_view re-import keeps the stored relation key, not the slug", func(t *testing.T) {
		// given: the executed defect — the whole-dataview re-import behind
		// every view op rewrote the stored key to the literal slug, so the
		// dataview named a relation key no relation object owns
		fx := slugSpaceFixture(t)
		captured := fx.expectMutate(editRead(t, slugDataviewDoc), "headB")

		// when: the column is addressed by the slug the listings serve
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"update_view","columns":{"manual_property":{"hidden":false}}}`), "", false)

		// then
		require.NoError(t, err)
		view := viewsOf(t, dataviewOf(t, *captured, "dataview"))[0]
		assert.Nil(t, columnByProperty(t, view, "manual_property"),
			"the slug must never become a stored relation key")
		col := columnByProperty(t, view, slugPropKey)
		require.NotNil(t, col, "the column keeps the stored key it was addressed through")
		_, stillHidden := col["hidden"]
		assert.False(t, stillHidden, "and the edit itself happened")
		assert.Equal(t, float64(100), col["width"])

		// the dataview's properties list is the same slot, one level over
		dv := dataviewOf(t, *captured, "dataview")
		props, _ := dv["properties"].([]any)
		var keys []string
		for _, p := range props {
			keys = append(keys, p.(map[string]any)["key"].(string))
		}
		assert.Contains(t, keys, slugPropKey)
		assert.NotContains(t, keys, "manual_property")
	})

	t.Run("a document key a live stored key claims does not fold into the bundled property", func(t *testing.T) {
		// given: a space holding a legacy relation STORED under `due_date`
		// beside the installed bundled `dueDate`. canonicalizeDocumentKeys
		// resolves `due_date` correctly at chain step 1 — and the import
		// vocabulary then rewrote it to `dueDate`, landing the value on the
		// bundled property (chain step 1 has no meaning in the bundled table).
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-legacy-due"),
			bundle.RelationKeyRelationKey: domain.String("due_date"),
			bundle.RelationKeyName:        domain.String("Legacy due date"),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-bundled-due"),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
		})
		captured := fx.expectCreate("obj-new")
		fx.expectEtagRead("obj-new")

		// when
		_, err := fx.CreateObject(ctx, testSpaceId, []byte(`{
			"version":1,"type":"page",
			"properties":{"name":"Ship it","due_date":"2026-01-02T00:00:00Z"}}`), false)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		snapshot := *captured
		assert.NotEmpty(t, pbtypes.Get(snapshot.Details, "due_date"),
			"an exact live stored key wins the whole chain, import included")
		assert.Nil(t, pbtypes.Get(snapshot.Details, "dueDate"),
			"the value must not land on the bundled property")
	})

	t.Run("insert_blocks lands a property block on the stored key", func(t *testing.T) {
		// the fragment-import channel (stateops importOptions): a `property`
		// block's key is a key slot, so it inverts through the same vocabulary
		fx := slugSpaceFixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_blocks","blocks":[{"type":"property","key":"manual_property"}]}`), "", false)

		require.NoError(t, err)
		var keys []string
		for _, b := range docBlocks(stateDoc(t, *captured)) {
			if b["type"] == "property" {
				key, _ := b["key"].(string)
				keys = append(keys, key)
			}
		}
		assert.Equal(t, []string{slugPropKey}, keys,
			"the property block binds the stored key the slug names")
	})

	t.Run("PATCH types typeProperties speak the same vocabulary as the document", func(t *testing.T) {
		// given: the PATCH channel writes the SAME §2a array a type document
		// carries, so it must invert its key slots through the same
		// vocabulary — BuildRecommendedLists took a bare resolver and did not
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-legacy-due"),
			bundle.RelationKeyRelationKey: domain.String("due_date"),
			bundle.RelationKeyName:        domain.String("Legacy due date"),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-bundled-due"),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-chore"),
			bundle.RelationKeyUniqueKey: domain.String("ot-chore"),
			bundle.RelationKeyName:      domain.String("Chore"),
		})
		var setDetails *pb.RpcObjectSetDetailsRequest
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
				setDetails = req
				return &pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}
			})
		fx.expectEtagRead("type-chore")

		// when
		_, err := fx.UpdateType(ctx, testSpaceId, "chore",
			[]byte(`{"type_properties":[{"property":"due_date","section":"featured"}]}`), false)

		// then
		require.NoError(t, err)
		require.NotNil(t, setDetails)
		byKey := map[string][]string{}
		for _, d := range setDetails.Details {
			byKey[d.Key] = pbtypes.GetStringListValue(d.Value)
		}
		assert.Equal(t, []string{"rel-legacy-due"}, byKey[bundle.RelationKeyRecommendedFeaturedRelations.String()],
			"the exact live stored key wins here too")
	})

	t.Run("a type document's typeProperties resolve the live stored key, not the bundled twin", func(t *testing.T) {
		// typeproperties.go inverts tp.Key through the same vocabulary before
		// the resolver ever sees it, so the bundled table's over-reach steered
		// the recommended list onto the wrong relation object
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-legacy-due"),
			bundle.RelationKeyRelationKey: domain.String("due_date"),
			bundle.RelationKeyName:        domain.String("Legacy due date"),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-bundled-due"),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
		})
		var created *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				created = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-chore",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-chore")

		// when
		_, err := fx.CreateType(ctx, testSpaceId, []byte(`{
			"kind":"object_type","key":"chore",
			"type_properties":[{"property":"due_date","section":"featured"}]}`), false)

		// then
		require.NoError(t, err)
		require.NotNil(t, created)
		assert.Equal(t, []string{"rel-legacy-due"},
			pbtypes.GetStringList(created.Details, bundle.RelationKeyRecommendedFeaturedRelations.String()),
			"the exact live stored key wins on the type-create path too")
	})
}
