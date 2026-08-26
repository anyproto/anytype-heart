package v2service

import (
	"context"
	"encoding/json"
	"testing"

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

// The review's M5 bypass was a canonicalization DIVERGENCE: the prewarm
// (pre-lock) lacked the fold the in-lock pass had, so a folded spelling was
// invisible to the create-missing bound and live inside the lock. Both
// passes now walk resolvePropertyInput; this table pins their equivalence
// over every spelling class, so the next divergence fails here first.
func TestCanonicalizationEquivalence(t *testing.T) {
	fx := slugSpaceFixture(t) // manual_property/mood_level (BSON keys), meeting_note type
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-twin1"),
		bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e201"),
		bundle.RelationKeyApiObjectKey: domain.String("twin_key"),
		bundle.RelationKeyName:         domain.String("Twin one"),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-twin2"),
		bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e202"),
		bundle.RelationKeyApiObjectKey: domain.String("twin_key"),
		bundle.RelationKeyName:         domain.String("Twin two"),
	})

	cases := []struct {
		name  string
		input string
		// canonical is what BOTH passes must produce; ambiguous/miss inputs
		// canonicalize to themselves (each pass then refuses/skips loudly
		// on its own path)
		canonical string
	}{
		{"stored key verbatim", slugPropKey, slugPropKey},
		{"bundled key verbatim", "dueDate", "dueDate"},
		{"live slug", "manual_property", slugPropKey},
		{"folded live slug", "manualProperty", slugPropKey},
		{"folded bundled key", "due_date", "dueDate"},
		{"ambiguous slug stays verbatim", "twin_key", "twin_key"},
		{"miss stays verbatim", "no_such_key", "no_such_key"},
	}

	resolvers := fx.newCreatingResolvers(context.Background(), testSpaceId, true)
	entries, err := fx.liveProperties(testSpaceId)
	require.NoError(t, err)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// the prewarm's pass
			assert.Equal(t, tc.canonical, resolvers.canonicalPropertyKey(tc.input), "prewarm pass")

			// the in-lock pass (canonicalizeSetPropertyKeys' canon core):
			// resolve through the same chain and compare
			entry, ok, ambiguous := fx.resolvePropertyInput(tc.input, entries)
			inLock := tc.input
			if len(ambiguous) == 0 && ok && entry.Key != tc.input {
				inLock = entry.Key
			}
			assert.Equal(t, tc.canonical, inLock, "in-lock pass")
		})
	}
}

func TestV2MintShadowingClosed(t *testing.T) {
	// a LEGACY relation stored as my_key — the one fixture shape that
	// exposes the stored-key shadow the typeProperties union check missed
	legacy := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-legacy"),
			bundle.RelationKeyRelationKey:    domain.String("my_key"),
			bundle.RelationKeyName:           domain.String("Legacy"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
		})
		return fx
	}

	t.Run("a camel respelling resolves to the legacy stored key, never a twin mint", func(t *testing.T) {
		// myKey folds onto my_key: the chain resolves it — no create RPC
		// (none is expected; firing one fails the mock)
		fx := legacy(t)
		var captured *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				captured = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-a",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-a")

		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Gizmo"},"type_settings":{"api_key":"gizmo","property_definitions":[{"property":"myKey","section":"featured"}]}}`), false)

		require.NoError(t, err)
		assert.Nil(t, result.Created, "nothing minted — the fold resolved to the legacy relation")
		require.NotNil(t, captured)
		assert.Equal(t, []string{"rel-legacy"},
			pbtypes.GetStringList(captured.Details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
	})

	t.Run("a spelling whose fold misses but whose slug collides is refused", func(t *testing.T) {
		// "My Key" folds to "my key" (the space survives folding) so the
		// chain misses — but its minted slug my_key would shadow the legacy
		// stored key. The union check on the SLUG spelling refuses loudly.
		fx := legacy(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Gizmo 2"},"type_settings":{"api_key":"gizmo2","property_definitions":[{"property":"My Key","name":"My Key","format":"text"}]}}`), false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `"my_key" is already taken`)
	})

	t.Run("two spellings of one key in one request are refused, not twin-minted", func(t *testing.T) {
		// the mint cache is keyed by document key and the live snapshot
		// predates this request's own mints — without mintedSlugs both
		// spellings minted, both stamped warranty_until, permanently
		// ambiguous, returned 200
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateRelationResponse{
				ObjectId: "rel-w1", Key: "6a7663db61fab21cd4b9e203",
				Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
			}).Once() // the FIRST spelling mints; a second create fails the mock

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Warrantied"},"type_settings":{"api_key":"warrantied","property_definitions":[{"property":"warranty_until","name":"W","format":"date"},{"property":"warrantyUntil","name":"W2","format":"date"}]}}`), false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "two spellings of one key")
	})

	t.Run("a fold-colliding property mint is refused", func(t *testing.T) {
		// minting moodlevel beside mood_level would make the folded
		// spelling permanently ambiguous for every caller
		fx := slugSpaceFixture(t)

		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "moodlevel", Name: "Mood 2", Format: "text"}, false)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "taken")
	})
}

func TestV2HiddenHoldersVacateTheSlugNamespace(t *testing.T) {
	// a hidden holder is invisible and undeletable to the caller — it must
	// not make a visible holder's slug a permanent 400 or downgrade its row
	fx := slugSpaceFixture(t)
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("type-hidden"),
		bundle.RelationKeyUniqueKey:    domain.String("ot-6a7663db61fab21cd4b9e204"),
		bundle.RelationKeyApiObjectKey: domain.String("meeting_note"),
		bundle.RelationKeyName:         domain.String("Hidden twin"),
		bundle.RelationKeyIsHidden:     domain.Bool(true),
	})

	t.Run("the visible holder still resolves by slug", func(t *testing.T) {
		fx.mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId: "type-meeting", IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL}})

		result, err := fx.DeleteType(context.Background(), testSpaceId, "meeting_note", false)

		require.NoError(t, err)
		assert.Equal(t, "type-meeting", result.Id)
	})

	t.Run("the visible holder's row keeps its slug", func(t *testing.T) {
		keys, err := fx.typeKeysById(testSpaceId)
		require.NoError(t, err)
		assert.Equal(t, "meeting_note", keys["type-meeting"])
	})
}

func TestV2CanonicalizeDocumentKeysDeterministicError(t *testing.T) {
	// two spellings collapsing onto one key must name the same path on
	// every run (the rewrite loop used to range an unsorted map)
	fx := slugSpaceFixture(t)
	body := []byte(`{"version":1,"properties":{"manual_property":"a","` + slugPropKey + `":"b","manualProperty":"c"}}`)
	var firstPath string
	for i := 0; i < 8; i++ {
		_, _, err := fx.canonicalizeDocumentKeys(testSpaceId, body)
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		require.NotEmpty(t, apiErr.Issues)
		if i == 0 {
			firstPath = apiErr.Issues[0].Path
			continue
		}
		assert.Equal(t, firstPath, apiErr.Issues[0].Path, "run %d", i)
	}
}

// sanity: the JSON in the twin-spelling test parses (guards the test body
// itself against quoting slips)
func TestCanonicalTestBodiesParse(t *testing.T) {
	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"objectType","key":"warrantied","properties":{"name":"Warrantied"},
			"type_settings":{"property_definitions":[{"property":"warranty_until","name":"W","format":"date"},{"property":"warrantyUntil","name":"W2","format":"date"}]}}`), &v))
}
