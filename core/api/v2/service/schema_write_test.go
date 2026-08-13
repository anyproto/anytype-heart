package v2service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// addTaskType registers a custom type "chore" with one recommended property
// (the select property from addSelectProperty) in the test space.
func (fx *v2Fixture) addTaskType(t *testing.T) {
	fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:                           domain.String("type-chore"),
		bundle.RelationKeyName:                         domain.String("Chore"),
		bundle.RelationKeyUniqueKey:                    domain.String("ot-chore"),
		bundle.RelationKeyResolvedLayout:               domain.Int64(int64(model.ObjectType_objectType)),
		bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-severity"}),
	}})
}

func TestV2CreateType(t *testing.T) {
	t.Run("type document creates missing properties atomically", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t) // "severity" exists — must NOT be re-created
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationRequest) bool {
			// the (a) identity layer: the document's key becomes the slug,
			// NEVER the stored relation key (a BSON is minted downstream)
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == "" &&
				pbtypes.GetString(req.Details, bundle.RelationKeyApiObjectKey.String()) == "spiciness" &&
				pbtypes.GetString(req.Details, bundle.RelationKeyName.String()) == "Spiciness" &&
				pbtypes.GetInt64(req.Details, bundle.RelationKeyRelationFormat.String()) == int64(model.RelationFormat_number)
		})).Return(&pb.RpcObjectCreateRelationResponse{
			ObjectId: "rel-spiciness", Key: "6a7663db61fab21cd4b9e745",
			Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
		})
		var createdDetails *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				createdDetails = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-workout",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-workout")

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId, []byte(`{
			"kind":"objectType","key":"workout",
			"properties":{"name":"Workout","recommendedLayout":"todo"},
			"typeProperties":[
				{"key":"severity","section":"featured"},
				{"key":"spiciness","name":"Spiciness","format":"number"}
			]}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "type-workout", result.Id)
		assert.Equal(t, "workout", result.Key)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1, "only the unknown key is created")
		assert.Equal(t, "spiciness", result.Created.Properties[0].Key)

		require.NotNil(t, createdDetails)
		details := createdDetails.Details
		assert.Empty(t, pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String()),
			"the document key must not become the uniqueKey — objectcreator mints a BSON (ADDRESSING §7.5)")
		assert.Equal(t, "workout", pbtypes.GetString(details, bundle.RelationKeyApiObjectKey.String()))
		assert.Equal(t, "Workout", pbtypes.GetString(details, bundle.RelationKeyName.String()))
		assert.Equal(t, int64(model.ObjectType_todo), pbtypes.GetInt64(details, bundle.RelationKeyRecommendedLayout.String()),
			"the layout NAME maps to the stored enum")
		assert.Equal(t, []string{"rel-severity"}, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
		assert.Equal(t, []string{"rel-spiciness"}, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()))
		assert.Empty(t, pbtypes.GetString(details, "id"), "the minted document id never travels into the RPC")
	})

	t.Run("all-featured typeProperties keep the featured list (regular seeded)", func(t *testing.T) {
		// FillRecommendedRelations detects "already filled" by the FIRST
		// entry of recommendedRelations; an empty regular section would send
		// it down the layout-defaults path and clobber the featured list.
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-createdDate"),
				bundle.RelationKeyRelationKey:    domain.String("createdDate"),
				bundle.RelationKeyName:           domain.String("Created date"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel-creator"),
				bundle.RelationKeyRelationKey:    domain.String("creator"),
				bundle.RelationKeyName:           domain.String("Creator"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel-links"),
				bundle.RelationKeyRelationKey:    domain.String("links"),
				bundle.RelationKeyName:           domain.String("Links"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
		})
		var createdDetails *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				createdDetails = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-workout",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-workout")

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(`{
			"kind":"objectType","key":"workout",
			"typeProperties":[{"key":"severity","section":"featured"}]}`), false)

		// then
		require.NoError(t, err)
		details := createdDetails.Details
		assert.Equal(t, []string{"rel-severity"}, pbtypes.GetStringList(details, bundle.RelationKeyRecommendedFeaturedRelations.String()),
			"the document's featured list survives")
		assert.Equal(t, []string{"rel-createdDate", "rel-creator", "rel-links"},
			pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String()),
			"the empty regular section is seeded with the system defaults")
	})

	t.Run("dry run reports would-be properties without creating", func(t *testing.T) {
		// given: no RPC expectations — any call fails the test
		fx := newV2Fixture(t)

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId, []byte(`{
			"kind":"objectType","key":"workout",
			"typeProperties":[{"key":"spiciness","format":"number"}]}`), true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, "spiciness", result.Created.Properties[0].Key)
	})

	t.Run("a pasted GET-type read body creates — etag and warnings are stripped", func(t *testing.T) {
		// this pass taught GetType to serve etag (the ?ids= threading), but
		// CreateType never got normalizeCreateBody — so POST types 400ed on
		// the etag of its own read while POST objects stripped the same
		// field. Dry run: no RPC expectations, any call fails the test.
		fx := newV2Fixture(t)

		result, err := fx.CreateType(context.Background(), testSpaceId, []byte(`{
			"kind":"objectType","key":"workout","etag":"abcd1234",
			"warnings":[{"message":"from the read"}],
			"typeProperties":[{"key":"spiciness","format":"number"}]}`), true)

		require.NoError(t, err, "a GET-type body must create without hand-stripping envelope fields")
		assert.True(t, result.DryRun)
	})

	t.Run("a ?block= subtree read is refused by name, not as an unknown field", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.CreateType(context.Background(), testSpaceId, []byte(`{
			"kind":"objectType","key":"workout","subtree":true,
			"typeProperties":[{"key":"spiciness","format":"number"}]}`), true)

		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/subtree", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "subtree read", "the refusal names the partial-read marker")
	})

	t.Run("bundled type key is rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"objectType","key":"task"}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/key", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "bundled")
	})

	t.Run("existing type key is rejected with PATCH steering", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"objectType","key":"chore"}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Hint, "PATCH /v2/spaces/"+testSpaceId+"/types/chore")
	})

	t.Run("blocks on a type document are rejected explicitly", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"objectType","key":"workout","blocks":[{"type":"dataview"}]}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/blocks", apiErr.Issues[0].Path)
	})

	t.Run("wrong kind is rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"page","key":"workout"}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
	})
}

func TestV2UpdateType(t *testing.T) {
	t.Run("patch updates details and rebuilds recommended lists", func(t *testing.T) {
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
		result, err := fx.UpdateType(context.Background(), testSpaceId, "chore", []byte(`{
			"properties":{"name":"Chores"},
			"typeProperties":[{"key":"severity","section":"featured"}]}`), false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "type-chore", result.Id)
		require.NotNil(t, setDetails)
		assert.Equal(t, "type-chore", setDetails.ContextId)
		byKey := map[string]*types.Value{}
		for _, d := range setDetails.Details {
			byKey[d.Key] = d.Value
		}
		require.Contains(t, byKey, "name")
		assert.Equal(t, "Chores", byKey["name"].GetStringValue())
		require.Contains(t, byKey, bundle.RelationKeyRecommendedFeaturedRelations.String())
		require.Contains(t, byKey, bundle.RelationKeyRecommendedRelations.String())
	})

	// The wire spelling the swagger, the served type schema, the hint text and
	// SPEC all teach. `name` folds identically in both vocabularies, which is
	// why the subtest above stayed green while two of this surface's four
	// fields were dead: the map is keyed by the wire spelling and the value was
	// read back with the stored one. Any fixture here must use a key the two
	// vocabularies SPELL DIFFERENTLY.
	t.Run("the slug spellings the surface teaches actually work", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			body      string
			detailKey string
			want      *types.Value
		}{
			{"icon_emoji", `{"properties":{"icon_emoji":"✅"}}`, "iconEmoji", pbtypes.String("✅")},
			{"recommended_layout", `{"properties":{"recommended_layout":"todo"}}`, "recommendedLayout", pbtypes.Int64(int64(model.ObjectType_todo))},
			{"the camelCase spelling still works", `{"properties":{"iconEmoji":"✅"}}`, "iconEmoji", pbtypes.String("✅")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// given
				fx := newV2Fixture(t)
				fx.addTaskType(t)
				var setDetails *pb.RpcObjectSetDetailsRequest
				fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
					RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
						setDetails = req
						return &pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}}
					})
				fx.expectEtagRead("type-chore")

				// when
				_, err := fx.UpdateType(context.Background(), testSpaceId, "chore", []byte(tc.body), false)

				// then
				require.NoError(t, err)
				require.NotNil(t, setDetails)
				require.Len(t, setDetails.Details, 1)
				assert.Equal(t, tc.detailKey, setDetails.Details[0].Key, "the STORED spelling reaches the store")
				assert.Equal(t, tc.want, setDetails.Details[0].Value)
			})
		}
	})

	t.Run("a rejection names the caller's own spelling", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addTaskType(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "chore",
			[]byte(`{"properties":{"recommended_layout":"frobnicate"}}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/properties/recommended_layout", apiErr.Issues[0].Path,
			"an issue path naming a key the request never sent is unactionable")
	})

	t.Run("non-updatable property key is rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "chore",
			[]byte(`{"properties":{"uniqueKey":"ot-hack"}}`), false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/properties/uniqueKey", apiErr.Issues[0].Path)
	})

	t.Run("unknown type is a 404", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "ghost", []byte(`{}`), false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
	})
}

func TestV2DeleteType(t *testing.T) {
	t.Run("delete archives the type object", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)
		fx.mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId: "type-chore", IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL}})

		// when
		result, err := fx.DeleteType(context.Background(), testSpaceId, "chore", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "type-chore", result.Id)
		assert.Equal(t, "chore", result.Key)
	})

	t.Run("dry run does not archive", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addTaskType(t)

		// when
		result, err := fx.DeleteType(context.Background(), testSpaceId, "chore", true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
	})
}

func TestV2CreateProperty(t *testing.T) {
	t.Run("create with options", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		const mintedKey = "6a7663db61fab21cd4b9e745"
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationRequest) bool {
			// the caller's key becomes the apiObjectKey slug; the stored
			// relation key is minted downstream (ADDRESSING §7.5)
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == "" &&
				pbtypes.GetString(req.Details, bundle.RelationKeyApiObjectKey.String()) == "vibe" &&
				pbtypes.GetInt64(req.Details, bundle.RelationKeyRelationFormat.String()) == int64(model.RelationFormat_status)
		})).Return(&pb.RpcObjectCreateRelationResponse{
			ObjectId: "rel-vibe", Key: mintedKey,
			Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
		})
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			// options bind to the STORED (minted) relation key
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == mintedKey &&
				pbtypes.GetString(req.Details, bundle.RelationKeyName.String()) == "Happy" &&
				pbtypes.GetString(req.Details, bundle.RelationKeyRelationOptionColor.String()) == "yellow"
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-happy",
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		})

		// when
		result, err := fx.CreateProperty(context.Background(), testSpaceId, v2model.CreatePropertyRequest{
			Key: "vibe", Name: "Mood", Format: "select",
			Options: []v2model.CreateOptionRequest{{Name: "Happy", Color: "yellow"}},
		}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "rel-vibe", result.Id)
		assert.Equal(t, "vibe", result.Key)
		require.NotNil(t, result.Created)
		assert.Equal(t, []v2model.CreatedOption{{Property: "vibe", Name: "Happy"}}, result.Created.Options)
	})

	t.Run("M6: the advertised bounds are enforced, path-addressed", func(t *testing.T) {
		// the property kind's schema advertises key pattern/length, name
		// length and the options cap — without enforcement a strict-mode
		// agent is told bounds the endpoint never checks; no create RPC is
		// expected, so slipping through fails the test twice over
		fx := newV2Fixture(t)

		// the review's reproduced input: a key the pattern forbids
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "my key!!", Name: "My key", Format: "text"}, false)
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/key", apiErr.Issues[0].Path)

		// a name over the advertised maxLength
		_, err = fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Name: strings.Repeat("x", maxV2NameLength+1), Format: "text"}, false)
		apiErr = v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)

		// more options than the advertised maxItems
		options := make([]v2model.CreateOptionRequest, maxV2PropertyOptions+1)
		for i := range options {
			options[i] = v2model.CreateOptionRequest{Name: fmt.Sprintf("o%d", i)}
		}
		_, err = fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Name: "Tags", Format: "multiSelect", Options: options}, false)
		apiErr = v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/options", apiErr.Issues[0].Path)
	})

	t.Run("unknown format names the allowed set", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Name: "X", Format: "picklist"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/format", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "multiSelect")
	})

	t.Run("options on a non-select format are rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateProperty(context.Background(), testSpaceId, v2model.CreatePropertyRequest{
			Name: "X", Format: "number", Options: []v2model.CreateOptionRequest{{Name: "One"}},
		}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/options", apiErr.Issues[0].Path)
	})

	t.Run("existing key is rejected", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)

		// when
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "severity", Name: "Again", Format: "select"}, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/key", apiErr.Issues[0].Path)
	})

	t.Run("dry run reports without creating", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		result, err := fx.CreateProperty(context.Background(), testSpaceId, v2model.CreatePropertyRequest{
			Key: "vibe", Name: "Mood", Format: "select",
			Options: []v2model.CreateOptionRequest{{Name: "Happy"}},
		}, true)

		// then
		require.NoError(t, err)
		assert.True(t, result.DryRun)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		require.Len(t, result.Created.Options, 1)
	})
}

func TestV2UpdateDeleteProperty(t *testing.T) {
	t.Run("update renames the property", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		newName := "Urgency"
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSetDetailsRequest) bool {
			return req.ContextId == "rel-severity" && len(req.Details) == 1 && req.Details[0].Key == "name"
		})).Return(&pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}})

		// when
		result, err := fx.UpdateProperty(context.Background(), testSpaceId, "severity",
			v2model.UpdatePropertyRequest{Name: &newName}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "rel-severity", result.Id)
	})

	t.Run("a case-variant key resolves through the fold layer", func(t *testing.T) {
		// §7.5a-3: the Title-Case guess folds onto the stored key and the
		// PATCH lands on the intended property
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSetDetailsRequest) bool {
			return req.ContextId == "rel-severity"
		})).Return(&pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}})
		name := "X"

		result, err := fx.UpdateProperty(context.Background(), testSpaceId, "Severity",
			v2model.UpdatePropertyRequest{Name: &name}, false)

		require.NoError(t, err)
		assert.Equal(t, "rel-severity", result.Id)
	})

	t.Run("unknown property is a 404 listing the space's keys", func(t *testing.T) {
		// given: the not-found family lists candidates (§8.21) — a
		// candidate-less tip left a benchmarked small model with nothing to
		// act on
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		name := "X"

		// when: a genuine miss — no fold candidate either
		_, err := fx.UpdateProperty(context.Background(), testSpaceId, "sev",
			v2model.UpdatePropertyRequest{Name: &name}, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Contains(t, apiErr.Message, "known property keys: severity")
		assert.Contains(t, apiErr.Message, "did you mean severity?")
	})

	t.Run("delete archives the property object", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId: "rel-severity", IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL}})

		// when
		result, err := fx.DeleteProperty(context.Background(), testSpaceId, "severity", false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "rel-severity", result.Id)
	})
}
