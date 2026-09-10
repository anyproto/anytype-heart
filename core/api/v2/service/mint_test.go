package v2service

import (
	"context"
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

// The (a) identity layer (ADDRESSING §7.5) and its union collision check
// (§7.5a-6, §7.6-3): every v2 create mints a BSON internal key, the caller's
// key lives only in the apiObjectKey slug, and the slug is checked at mint
// against bundled keys, bundled-derived slugs, live stored keys and live
// stored slugs — with corpses vacated (§8-OQ2), which is what makes
// delete-then-recreate mint cleanly.

func v2ErrWithIssue(t *testing.T, err error) *v2model.Error {
	t.Helper()
	var apiErr *v2model.Error
	require.ErrorAs(t, err, &apiErr)
	return apiErr
}

func TestV2PropertyMintCollisionCheck(t *testing.T) {
	t.Run("a caller key cannot shadow a bundled slug", func(t *testing.T) {
		// given: "due_date" is bundled dueDate's derived slug. Before the
		// union check, propertyKeyExists("due_date") missed (the store has
		// dueDate, the bundle has dueDate) and the create pinned due_date as
		// a stored relation key — shadowing the bundled slug forever.
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "due_date", Name: "My due date", Format: "date"}, false)

		// then: loud refusal naming the bundled holder — no create RPC
		apiErr := v2ErrWithIssue(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "bundled property")
	})

	t.Run("normalization makes camel and snake collide", func(t *testing.T) {
		// dueDate2 and due_date2 are one slug after snake-at-mint — the
		// §7.5a-6 example: the sequential second create is refused. (The
		// joint spelling is due_date_2: strcase separates trailing digits,
		// which both spellings normalize into.)
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-dd2"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e001"),
			bundle.RelationKeyApiObjectKey: domain.String("due_date_2"),
			bundle.RelationKeyName:         domain.String("Due date 2"),
		})

		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "dueDate2", Name: "Another", Format: "date"}, false)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"due_date_2"`)

		// the snake spelling of the same key is the same refusal
		_, err = fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "due_date2", Name: "Another", Format: "date"}, false)
		v2ErrWithIssue(t, err)
	})

	t.Run("a name-derived slug is guarded too", func(t *testing.T) {
		// no key given: the slug derives from the name and the union check
		// still runs — a property NAMED "Due date" steers to bundled
		// due_date instead of silently minting a shadowing slug
		fx := newV2Fixture(t)

		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Name: "Due date", Format: "date"}, false)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "due_date")
	})

	t.Run("corpses vacate the namespace: delete-then-recreate mints cleanly", func(t *testing.T) {
		// given: an uninstalled (UI-deleted) and an archived (v2-deleted)
		// relation both holding "corpse_key"-ish identities. Before the
		// rework the uninstalled corpse made the guard refuse "already
		// exists" (steering to PATCH an object the user deleted), and the
		// archived one passed the guard only to die on ErrTreeExists at
		// PutTree — because the caller's key WAS the derived tree.
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("rel-uninstalled"),
			bundle.RelationKeyRelationKey:   domain.String("corpse_key"),
			bundle.RelationKeyName:          domain.String("UI-deleted"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-archived"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e002"),
			bundle.RelationKeyApiObjectKey: domain.String("corpse_key"),
			bundle.RelationKeyName:         domain.String("v2-deleted"),
			bundle.RelationKeyIsArchived:   domain.Bool(true),
		})
		var captured *pb.RpcObjectCreateRelationRequest
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				captured = req
				return &pb.RpcObjectCreateRelationResponse{
					ObjectId: "rel-fresh", Key: "6a7663db61fab21cd4b9e003",
					Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
				}
			})

		// when
		result, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "corpse_key", Name: "Recreated", Format: "text"}, false)

		// then: a clean create — fresh BSON identity (no relationKey in the
		// payload, so no derived tree to collide with), slug re-taken
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Empty(t, pbtypes.GetString(captured.Details, bundle.RelationKeyRelationKey.String()),
			"a caller key in the payload would re-derive the corpse's tree and die on ErrTreeExists")
		assert.Equal(t, "corpse_key", pbtypes.GetString(captured.Details, bundle.RelationKeyApiObjectKey.String()))
		assert.Equal(t, "corpse_key", result.Key)
	})

	t.Run("a live custom slug refuses the twin", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-live"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e004"),
			bundle.RelationKeyApiObjectKey: domain.String("priority_level"),
			bundle.RelationKeyName:         domain.String("Priority level"),
		})

		_, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "priorityLevel", Name: "Priority level 2", Format: "text"}, false)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"priority_level"`)
	})

	t.Run("propertyKeyExists is corpse-blind and live-sighted", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("rel-c"),
			bundle.RelationKeyRelationKey:   domain.String("corpseKey"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-l"),
			bundle.RelationKeyRelationKey: domain.String("liveKey"),
		})
		assert.False(t, fx.propertyKeyExists(testSpaceId, "corpseKey"))
		assert.True(t, fx.propertyKeyExists(testSpaceId, "liveKey"))
		assert.True(t, fx.propertyKeyExists(testSpaceId, "dueDate"), "bundled keys always exist")
	})
}

func TestV2TypeMintCollisionCheck(t *testing.T) {
	t.Run("a document key cannot shadow a bundled type slug", func(t *testing.T) {
		// "object_type" is bundled objectType's derived slug; the old guard
		// checked only the exact bundled key, so object_type passed and
		// minted uniqueKey ot-object_type — a permanent shadow
		fx := newV2Fixture(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Shadow"},"type_settings":{"api_key":"object_type"}}`), false, true)

		apiErr := v2ErrWithIssue(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "bundled type")
	})

	t.Run("type corpses vacate the namespace", func(t *testing.T) {
		// given: a UI-deleted type holding ot-corpsetype — the old guard saw
		// it and refused "already exists", steering PATCH at a corpse
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("type-corpse"),
			bundle.RelationKeyUniqueKey:     domain.String("ot-corpsetype"),
			bundle.RelationKeyName:          domain.String("Old"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		var captured *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				captured = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-fresh",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-fresh")

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Recreated"},"type_settings":{"api_key":"corpsetype"}}`), false, true)

		// then: a clean create with fresh BSON identity
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Empty(t, pbtypes.GetString(captured.Details, bundle.RelationKeyUniqueKey.String()),
			"the corpse's key in uniqueKey would re-derive its tree")
		assert.Equal(t, "corpsetype", pbtypes.GetString(captured.Details, bundle.RelationKeyApiObjectKey.String()))
		assert.Equal(t, "corpsetype", result.Key)
	})

	t.Run("a live type slug refuses the twin", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-live"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6a7663db61fab21cd4b9e005"),
			bundle.RelationKeyApiObjectKey: domain.String("meeting_note"),
			bundle.RelationKeyName:         domain.String("Meeting note"),
		})

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Meeting note 2"},"type_settings":{"api_key":"meetingNote"}}`), false, true)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, `"meeting_note"`)
	})
}

func TestV2PropertyIdResolutionChain(t *testing.T) {
	t.Run("a document key resolves through the space slug namespace", func(t *testing.T) {
		// given: a v2-created property — BSON stored key, slug
		// priority_level. A typeProperties entry naming priority_level must
		// resolve to it (chain step 2), never mint a twin.
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-priority"),
			bundle.RelationKeyRelationKey:    domain.String("6a7663db61fab21cd4b9e006"),
			bundle.RelationKeyApiObjectKey:   domain.String("priority_level"),
			bundle.RelationKeyName:           domain.String("Priority level"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
		})
		var captured *pb.RpcObjectCreateObjectTypeRequest
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
				captured = req
				return &pb.RpcObjectCreateObjectTypeResponse{
					ObjectId: "type-x",
					Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
				}
			})
		fx.expectEtagRead("type-x")

		// when: no ObjectCreateRelation expectation — a create RPC fails the
		// mock, which is the point
		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Sprint"},"type_settings":{"api_key":"sprint","property_definitions":[{"property":"priority_level","section":"featured"}]}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Created, "nothing minted — the slug resolved")
		require.NotNil(t, captured)
		assert.Equal(t, []string{"rel-priority"},
			pbtypes.GetStringList(captured.Details, bundle.RelationKeyRecommendedFeaturedRelations.String()))
	})

	t.Run("a bundled slug installs the bundled relation, never a twin", func(t *testing.T) {
		// chain step 3: due_date names bundled dueDate. The create RPC must
		// carry relationKey dueDate (the derived install path — convergence
		// is the install mechanism, §2.4-1), NOT pin due_date as a new key.
		fx := newV2Fixture(t)
		var captured *pb.RpcObjectCreateRelationRequest
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				captured = req
				return &pb.RpcObjectCreateRelationResponse{
					ObjectId: "rel-duedate", Key: "dueDate",
					Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
				}
			})
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateObjectTypeResponse{
				ObjectId: "type-y",
				Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
			})
		fx.expectEtagRead("type-y")

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Errand"},"type_settings":{"api_key":"errand","property_definitions":[{"property":"due_date","section":"featured"}]}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Equal(t, "dueDate", pbtypes.GetString(captured.Details, bundle.RelationKeyRelationKey.String()),
			"the bundled derived table rewrites the slug to the bundled key so install convergence serves it")
	})

	t.Run("an ambiguous slug fails loud, never by store order", func(t *testing.T) {
		// two live holders of one slug (the concurrent-create artifact the
		// (a) strategy accepts as its cost): resolution lists both and
		// refuses — the D2 lesson applied to the key layer
		fx := newV2Fixture(t)
		for i, id := range []string{"rel-twin1", "rel-twin2"} {
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String(id),
				bundle.RelationKeyRelationKey:  domain.String([]string{"6a7663db61fab21cd4b9e007", "6a7663db61fab21cd4b9e008"}[i]),
				bundle.RelationKeyApiObjectKey: domain.String("twin_key"),
				bundle.RelationKeyName:         domain.String("Twin"),
			})
		}

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Twin user"},"type_settings":{"api_key":"twinuser","property_definitions":[{"property":"twin_key"}]}}`), false, true)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambiguous")
		assert.Contains(t, err.Error(), "rel-twin1")
		assert.Contains(t, err.Error(), "rel-twin2")
	})

	t.Run("a declared format conflicting with the resolved property is a loud 400", func(t *testing.T) {
		// the format check SPEC §2a promises at the wiring (§2.3-5): before
		// this, PropertyId returned the existing relation with the declared
		// format silently ignored — the entry's objects then held
		// wrong-shaped values
		fx := newV2Fixture(t)
		fx.addSelectProperty(t) // "severity", format select

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Incident"},"type_settings":{"api_key":"incident","property_definitions":[{"property":"severity","format":"text"}]}}`), false, true)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/type_properties/0/format", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `"select"`)
	})

	t.Run("a declared format conflicting with a bundled property is a loud 400", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Errand 2"},"type_settings":{"api_key":"errand2","property_definitions":[{"property":"due_date","format":"text"}]}}`), false, true)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/type_properties/0/format", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, `"date"`)
	})

	t.Run("a matching declared format passes the check", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateObjectTypeResponse{
				ObjectId: "type-ok",
				Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
			})
		fx.expectEtagRead("type-ok")

		_, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Incident 2"},"type_settings":{"api_key":"incident2","property_definitions":[{"property":"severity","format":"select"}]}}`), false, true)

		require.NoError(t, err)
	})

	t.Run("the format check guards the PATCH typeProperties channel too", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addSelectProperty(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-edit"),
			bundle.RelationKeyUniqueKey: domain.String("ot-editable"),
			bundle.RelationKeyName:      domain.String("Editable"),
		})

		_, err := fx.UpdateType(context.Background(), testSpaceId, "editable",
			[]byte(`{"type_settings":{"property_definitions":[{"property":"severity","format":"number"}]}}`), false, true)

		apiErr := v2ErrWithIssue(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/type_properties/0/format", apiErr.Issues[0].Path)
	})

	t.Run("a custom key still creates on a full miss, slug stamped", func(t *testing.T) {
		fx := newV2Fixture(t)
		var captured *pb.RpcObjectCreateRelationRequest
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				captured = req
				return &pb.RpcObjectCreateRelationResponse{
					ObjectId: "rel-new", Key: "6a7663db61fab21cd4b9e009",
					Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
				}
			})
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateObjectTypeResponse{
				ObjectId: "type-z",
				Error:    &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
			})
		fx.expectEtagRead("type-z")

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Gadget"},"type_settings":{"api_key":"gadget","property_definitions":[{"property":"warranty_until","name":"Warranty until","format":"date"}]}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Empty(t, pbtypes.GetString(captured.Details, bundle.RelationKeyRelationKey.String()))
		assert.Equal(t, "warranty_until", pbtypes.GetString(captured.Details, bundle.RelationKeyApiObjectKey.String()))
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, "warranty_until", result.Created.Properties[0].Key)
	})
}

// TestCreateReturnsTheStoredKeyNotTheProposal pins the one thing a 201 owes:
// the key it returns must be a key the key routes accept. v2's pre-check and
// the heart's mint check DIFFERENT namespaces on purpose — the mint counts
// hidden holders (a hidden holder still occupies a stored slug in data, and
// minting a second entity onto it is what creates the ambiguity), v2's
// request namespace excludes them (propertyEntry.Hidden). So the mint can
// suffix a slug v2 just found free, or give up and store none. Returning the
// PROPOSAL made `201 {"key":"manual_property"}` followed by
// `GET …/properties/manual_property` → 404.
func TestCreateReturnsTheStoredKeyNotTheProposal(t *testing.T) {
	t.Run("a property mint that suffixed is reported as suffixed", func(t *testing.T) {
		// given — a HIDDEN holder of `manual_property`: v2's pre-check does
		// not see it, so the create proceeds; the mint does, so it suffixes
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-hidden"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e201"),
			bundle.RelationKeyApiObjectKey: domain.String("manual_property"),
			bundle.RelationKeyName:         domain.String("Hidden holder"),
			bundle.RelationKeyIsHidden:     domain.Bool(true),
		})
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateRelationResponse{
				ObjectId: "rel-new", Key: "6a7663db61fab21cd4b9e202",
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyApiObjectKey.String(): pbtypes.String("manual_property_2"),
				}},
				Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
			})

		// when
		result, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "manual_property", Name: "Manual property", Format: "text"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "manual_property_2", result.Key,
			"the stored slug, never the proposal — the proposal 404s")
	})

	t.Run("a property mint that stored no slug reports the internal key", func(t *testing.T) {
		// given — the walk gave up (maxApiKeySuffix): apiObjectKey is empty
		// and the minted BSON is the only address there is
		fx := newV2Fixture(t)
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateRelationResponse{
				ObjectId: "rel-new", Key: "6a7663db61fab21cd4b9e203",
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyApiObjectKey.String(): pbtypes.String(""),
				}},
				Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
			})

		// when
		result, err := fx.CreateProperty(context.Background(), testSpaceId,
			v2model.CreatePropertyRequest{Key: "manual_property", Name: "Manual property", Format: "text"}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "6a7663db61fab21cd4b9e203", result.Key)
	})

	t.Run("the type create path has the same shape", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-hidden"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6a7663db61fab21cd4b9e204"),
			bundle.RelationKeyApiObjectKey: domain.String("invoice"),
			bundle.RelationKeyName:         domain.String("Hidden type"),
			bundle.RelationKeyIsHidden:     domain.Bool(true),
		})
		fx.mwMock.EXPECT().ObjectCreateObjectType(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateObjectTypeResponse{
				ObjectId: "type-new",
				Details: &types.Struct{Fields: map[string]*types.Value{
					bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-6a7663db61fab21cd4b9e205"),
					bundle.RelationKeyApiObjectKey.String(): pbtypes.String("invoice_2"),
				}},
				Error: &pb.RpcObjectCreateObjectTypeResponseError{Code: pb.RpcObjectCreateObjectTypeResponseError_NULL},
			})
		fx.expectEtagRead("type-new")

		// when
		result, err := fx.CreateType(context.Background(), testSpaceId,
			[]byte(`{"kind":"object_type","properties":{"name":"Invoice"},"type_settings":{"api_key":"invoice"}}`), false, true)

		// then
		require.NoError(t, err)
		assert.Equal(t, "invoice_2", result.Key)
	})
}
