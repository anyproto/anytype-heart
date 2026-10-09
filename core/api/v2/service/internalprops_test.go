package v2service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// The hidden internal properties (`_score`, `syncStatus`, `id`, …) are app
// machinery: no type lists them and every object write refuses them. A
// caller's "Score" used to fold onto the fulltext `_score`, so create_type
// listed `_score` and every create_object after it was refused.

func (fx *v2Fixture) expectTypeCreate(id string) **pb.RpcObjectCreateObjectTypeRequest {
	var captured *pb.RpcObjectCreateObjectTypeRequest
	fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
			captured = req
			return &pb.RpcObjectCreateObjectTypeResponse{ObjectId: id,
				Error: &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL}}
		})
	fx.expectEtagRead(id)
	return &captured
}

func listedIds(req *pb.RpcObjectCreateObjectTypeRequest) []string {
	var ids []string
	for _, key := range []domain.RelationKey{
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedHiddenRelations,
		bundle.RelationKeyRecommendedFileRelations,
	} {
		ids = append(ids, pbtypes.GetStringList(req.Details, key.String())...)
	}
	return ids
}

func TestV2HiddenInternalProperties(t *testing.T) {
	t.Run("a caller's Score is a new property, not the fulltext score", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationRequest) bool {
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == "" &&
				pbtypes.GetString(req.Details, bundle.RelationKeyName.String()) == "Score"
		})).Return(&pb.RpcObjectCreateRelationResponse{
			ObjectId: "rel-score", Key: "6a7663db61fab21cd4b9e745",
			Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
		})
		captured := fx.expectTypeCreate("type-game")

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Game"},"type_settings":{"api_key":"game","property_definitions":[{"name":"Score","format":"number"}]}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, "score", result.Created.Properties[0].Key)
		assert.Empty(t, result.Warnings)
		assert.Contains(t, listedIds(*captured), "rel-score")
	})

	t.Run("a hidden internal key is dropped from the definitions with a warning", func(t *testing.T) {
		for _, term := range []string{"_score", "syncStatus", "Anytype ID"} {
			t.Run(term, func(t *testing.T) {
				// given: no ObjectCreateRelation expectation — installing the
				// bundled relation would fail the mock
				fx := newV2Fixture(t)
				fx.addSelectProperty(t)
				captured := fx.expectTypeCreate("type-game")

				// when
				result, err := fx.CreateType(context.Background(), testSpaceId,
					[]byte(`{"kind":"object_type","properties":{"name":"Game"},"type_settings":{"api_key":"game","property_definitions":[{"property":"severity","section":"featured"},{"property":"`+term+`"}]}}`), false, true)

				// then
				require.NoError(t, err)
				assert.Nil(t, result.Created)
				require.Len(t, result.Warnings, 1)
				assert.Equal(t, "/type_settings/property_definitions/1", result.Warnings[0].Path)
				assert.Contains(t, result.Warnings[0].Message, "internal property")
				assert.Equal(t, []string{"rel-severity"},
					pbtypes.GetStringList((*captured).Details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
				for _, id := range listedIds(*captured) {
					assert.NotContains(t, []string{"_br_score", "_brsyncStatus", "_brid"}, id)
				}
			})
		}
	})

	t.Run("a visible system property is still placed", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		captured := fx.expectTypeCreate("type-game")

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Game"},"type_settings":{"api_key":"game","property_definitions":[{"property":"Created by","section":"featured"}]}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
		assert.Equal(t, []string{bundle.RelationKeyCreator.BundledURL()},
			pbtypes.GetStringList((*captured).Details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
	})

	t.Run("a replaced list drops a new hidden internal entry", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)
		var setDetails *pb.RpcObjectSetDetailsRequest
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
				setDetails = req
				return &pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}
			})
		fx.expectEtagRead("type-chore")

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "chore", "", []byte(`{
			"type_settings":{"property_definitions":[{"property":"severity","section":"featured"},{"property":"_score"}]}}`), false, true)

		// then
		require.NoError(t, err)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "/type_settings/property_definitions/1", result.Warnings[0].Path)
		require.NotNil(t, setDetails)
		for _, d := range setDetails.Details {
			for _, v := range d.Value.GetListValue().GetValues() {
				assert.NotEqual(t, "_br_score", v.GetStringValue(), d.Key)
			}
		}
	})

	t.Run("a hidden internal entry the type already lists survives the echo", func(t *testing.T) {
		// given: the bundled template type lists templateIsBundled; a read
		// echoed back must not detach it
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-tib"),
			bundle.RelationKeyRelationKey: domain.String(bundle.RelationKeyTemplateIsBundled.String()),
			bundle.RelationKeyName:        domain.String("Bundled Template"),
			bundle.RelationKeyIsHidden:    domain.Bool(true),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                         domain.String("type-tpl"),
			bundle.RelationKeyName:                       domain.String("Tpl"),
			bundle.RelationKeyUniqueKey:                  domain.String("ot-tpl"),
			bundle.RelationKeyRecommendedHiddenRelations: domain.StringList([]string{"rel-tib"}),
		})
		var setDetails *pb.RpcObjectSetDetailsRequest
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
				setDetails = req
				return &pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}
			})
		fx.expectEtagRead("type-tpl")

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "tpl", "", []byte(`{
			"type_settings":{"property_definitions":[{"property":"templateIsBundled","section":"hidden"}]}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Empty(t, result.Warnings)
		require.NotNil(t, setDetails)
		var hidden []string
		for _, d := range setDetails.Details {
			if d.Key == bundle.RelationKeyRecommendedHiddenRelations.String() {
				for _, v := range d.Value.GetListValue().GetValues() {
					hidden = append(hidden, v.GetStringValue())
				}
			}
		}
		assert.Equal(t, []string{"rel-tib"}, hidden)
	})

	t.Run("add_property refuses a hidden internal property", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "chore", "",
			opsBody(`{"op":"add_property","property":"_score"}`), false, true)

		// then
		var apiErr *v2model.Error
		require.True(t, errors.As(err, &apiErr), "got %v", err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.property", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "internal property")
	})

	t.Run("the fold no longer reaches a hidden internal key", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		entries, err := fx.liveProperties(testSpaceId)
		require.NoError(t, err)

		for _, tc := range []struct {
			input   string
			want    string
			resolve bool
		}{
			{"Score", "", false},
			{"score", "", false},
			{"_score", "_score", true}, // the stored key is still its own address
			{"ID", "id", true},         // id keeps its fold: its refusal names the envelope
			{"creator", "creator", true},
		} {
			// when
			entry, ok, _ := fx.resolvePropertyInput(tc.input, entries)

			// then
			assert.Equal(t, tc.resolve, ok, tc.input)
			if tc.resolve {
				assert.Equal(t, tc.want, entry.Key, tc.input)
			}
		}
	})
}
