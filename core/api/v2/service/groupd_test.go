package v2service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/any-block/codec/anyblockjson"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// Round-two eval F11, F16, F18, F19, F20: writes say what they did and
// what they could not; reads do not narrate the export's bookkeeping.

const editChoreDoc = `{"formatVersion":"2.0","id":"obj1","type":"chore","properties":{"name":"Doc","severity":["High"]}}`

func TestV2FavoriteFlag(t *testing.T) {
	ctx := context.Background()
	favoriteOk := &pb.RpcObjectListSetIsFavoriteResponse{Error: &pb.RpcObjectListSetIsFavoriteResponseError{Code: pb.RpcObjectListSetIsFavoriteResponseError_NULL}}

	t.Run("set is_favorite runs the favorite RPC after the edit commits, not a detail write", func(t *testing.T) {
		fx := newV2Fixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectListSetIsFavorite(mock.Anything, &pb.RpcObjectListSetIsFavoriteRequest{
			ObjectIds: []string{"obj1"}, IsFavorite: true,
		}).Return(favoriteOk).Once()

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, 1, result.DiffStats.PropertiesChanged, "the receipt counts the flag")
		assert.False(t, (*captured).CombinedDetails().Has(bundle.RelationKeyIsFavorite),
			"favorites are links from the home object, not a detail persisted on the page")
	})

	t.Run("unset is_favorite runs the RPC with false", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectListSetIsFavorite(mock.Anything, &pb.RpcObjectListSetIsFavoriteRequest{
			ObjectIds: []string{"obj1"}, IsFavorite: false,
		}).Return(favoriteOk).Once()

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","unset":["is_favorite"]}`), "", false, true)

		require.NoError(t, err)
	})

	t.Run("a dry run does not favorite", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", true, true)

		require.NoError(t, err)
		assert.True(t, result.DryRun)
	})

	t.Run("a non-boolean flag is refused at the field", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":"yes"}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ops[0].set.is_favorite", apiErr.Issues[0].Path)
	})

	t.Run("an RPC failure is an error, not a silent 200", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectListSetIsFavorite(mock.Anything, mock.Anything).Return(&pb.RpcObjectListSetIsFavoriteResponse{
			Error: &pb.RpcObjectListSetIsFavoriteResponseError{Code: pb.RpcObjectListSetIsFavoriteResponseError_UNKNOWN_ERROR, Description: "home is read-only"},
		}).Once()

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"is_favorite":true}}`), "", false, true)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "home is read-only")
	})
}

func TestV2WritesSayWhatTheyLeaveInvisible(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t) // "chore" recommends severity
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-spice"),
			bundle.RelationKeyRelationKey:  domain.String("6a8f2c1d9e4b7a3f5c2d8e77"),
			bundle.RelationKeyApiObjectKey: domain.String("spiciness"),
			bundle.RelationKeyName:         domain.String("Spiciness"),
		})
		return fx
	}

	t.Run("a value on a property the type does not list is stored with a warning naming the repair", func(t *testing.T) {
		fx := setup(t)
		captured := fx.expectMutate(editRead(t, editChoreDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"spiciness":"mild"}}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, "mild", (*captured).CombinedDetails().GetString(domain.RelationKey("6a8f2c1d9e4b7a3f5c2d8e77")), "stored")
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "ops[0].set.spiciness", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, `property "spiciness" is not on type "chore"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetOpSchema("add_property")}, result.Warnings[0].SeeAlso)
	})

	t.Run("a listed property, a bundled one, and one the object already carries warn nothing", func(t *testing.T) {
		fx := setup(t)
		fx.expectMutate(editRead(t, `{"formatVersion":"2.0","id":"obj1","type":"chore","properties":{"name":"Doc","spiciness":"hot"}}`), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"severity":["High"],"description":"d","spiciness":"mild"}}`), "", false, true)

		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a query with no filter says it lists the whole type", func(t *testing.T) {
		fx := setup(t)
		fx.expectCreate("q1")
		fx.expectEtagRead("q1")

		result, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Chores needing attention", Type: "chore"}, false, true)

		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Contains(t, result.Warnings[0].Message, `lists every object of type "chore"`)
		assert.Equal(t, []v2model.Ref{v2model.RefGetSchema("filters")}, result.Warnings[0].SeeAlso)
	})

	t.Run("a filtered query, by string or by view, warns nothing", func(t *testing.T) {
		for name, req := range map[string]v2model.CreateQueryRequest{
			"string": {Name: "Q", Type: "chore", Filter: `severity = "High"`},
			"view":   {Name: "Q", Type: "chore", Views: json.RawMessage(`[{"name":"V","filters":[{"property":"severity","condition":"equal","value":["High"]}]}]`)},
		} {
			t.Run(name, func(t *testing.T) {
				fx := setup(t)
				fx.expectCreate("q1")
				fx.expectEtagRead("q1")

				result, err := fx.CreateQuery(ctx, testSpaceId, req, false, true)

				require.NoError(t, err)
				assert.Empty(t, result.Warnings)
			})
		}
	})

	t.Run("a view-key refusal spells the served key, never the stored one", func(t *testing.T) {
		fx := setup(t)

		_, err := fx.CreateQuery(ctx, testSpaceId, v2model.CreateQueryRequest{Name: "Q", Type: "chore",
			Sorts: json.RawMessage(`[{"property":"spiciness"}]`)}, true, true)

		issue := issueAt(t, err, "/sorts/0/property")
		assert.Contains(t, issue.Message, `type "chore" has no property "spiciness"`)
		assert.NotContains(t, issue.Message, "6a8f2c1d9e4b7a3f5c2d8e77")
		assert.NotContains(t, issue.Hint, "6a8f2c1d9e4b7a3f5c2d8e77")
	})
}

func TestV2ReceiptsCountWhatChanged(t *testing.T) {
	ctx := context.Background()

	t.Run("item ops count the members they actually changed", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutate(editRead(t, editCollectionDoc), "headB") // holds memberA

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"add_items","items":["memberA","memberB","memberC"]}`, `{"op":"remove_items","items":["memberA","memberZ"]}`), "", false, true)

		require.NoError(t, err)
		assert.Equal(t, 2, result.DiffStats.ItemsAdded, "memberA was already present")
		assert.Equal(t, 1, result.DiffStats.ItemsRemoved, "memberZ was never there")
	})

	t.Run("a created collection says how many members it holds", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("m1"), bundle.RelationKeyName: domain.String("One")},
			{bundle.RelationKeyId: domain.String("m2"), bundle.RelationKeyName: domain.String("Two")},
		})
		fx.expectCreate("newCollection")
		fx.expectEtagRead("newCollection")

		result, err := fx.CreateCollection(ctx, testSpaceId, v2model.CreateCollectionRequest{Name: "L", Items: []string{"m1", "m2"}}, false)

		require.NoError(t, err)
		assert.Equal(t, 2, result.Items)
	})
}

func TestV2ReadWarningsAreNotExportBookkeeping(t *testing.T) {
	assert.True(t, readWarningIsNoise(anyblockjson.Issue{Message: `legend value: "backlinks" is internal: export strips it, so import does not accept it — so no legend entry is written for "backlinks"; the term is spelled verbatim`}))
	assert.True(t, readWarningIsNoise(anyblockjson.Issue{Message: `the type has no stored key, so type_internal_key is not written; the type is spelled verbatim`}))
	assert.False(t, readWarningIsNoise(anyblockjson.Issue{Message: `block b1: nesting depth 40 exceeds the bound 32 — indent clamped`}))
}
