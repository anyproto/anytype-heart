package v2service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

// The §7.5a-5 input chain: after the (a) mint rework a v2-created property
// or type has a BSON stored key and its slug is the only readable address —
// every place the API takes a key must resolve the slug (exact stored key
// first, live slug second, bundled table third, fold fourth; ambiguity is a
// loud 400, never a guess).

const (
	slugPropKey   = "6a7663db61fab21cd4b9e101" // stored key of the slug-addressed text property
	slugSelectKey = "6a7663db61fab21cd4b9e102" // stored key of the slug-addressed select property
	slugTypeKey   = "6a7663db61fab21cd4b9e103" // internal key of the slug-addressed type
)

func slugSpaceFixture(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-manual"),
		bundle.RelationKeyRelationKey:  domain.String(slugPropKey),
		bundle.RelationKeyApiObjectKey: domain.String("manual_property"),
		bundle.RelationKeyName:         domain.String("Manual property"),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-mood"),
		bundle.RelationKeyRelationKey:    domain.String(slugSelectKey),
		bundle.RelationKeyApiObjectKey:   domain.String("mood_level"),
		bundle.RelationKeyName:           domain.String("Mood level"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("type-meeting"),
		bundle.RelationKeyUniqueKey:    domain.String("ot-" + slugTypeKey),
		bundle.RelationKeyApiObjectKey: domain.String("meeting_note"),
		bundle.RelationKeyName:         domain.String("Meeting note"),
	})
	return fx
}

func TestV2SlugAddressedRoutes(t *testing.T) {
	t.Run("PATCH properties by slug lands on the BSON-keyed relation", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSetDetailsRequest) bool {
			return req.ContextId == "rel-manual"
		})).Return(&pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL}})
		name := "Renamed"

		// when
		result, err := fx.UpdateProperty(context.Background(), testSpaceId, "manual_property",
			v2model.UpdatePropertyRequest{Name: &name}, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "rel-manual", result.Id)
	})

	t.Run("options listing by slug lists the stored-key-bound options", func(t *testing.T) {
		// given: options bind to the STORED key, so the slug must resolve to
		// it before the store lookup
		fx := slugSpaceFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("opt-happy"),
			bundle.RelationKeyRelationKey:    domain.String(slugSelectKey),
			bundle.RelationKeyName:           domain.String("Happy"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		}})

		// when
		rows, _, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "mood_level", "", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "Happy", rows[0].Name)
	})

	t.Run("DELETE types by slug archives the BSON-keyed type", func(t *testing.T) {
		fx := slugSpaceFixture(t)
		fx.mwMock.EXPECT().ObjectSetIsArchived(mock.Anything, &pb.RpcObjectSetIsArchivedRequest{
			ContextId: "type-meeting", IsArchived: true,
		}).Return(&pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: pb.RpcObjectSetIsArchivedResponseError_NULL}})

		result, err := fx.DeleteType(context.Background(), testSpaceId, "meeting_note", false)

		require.NoError(t, err)
		assert.Equal(t, "type-meeting", result.Id)
	})

	t.Run("an ambiguous slug is a loud 400 listing both holders", func(t *testing.T) {
		// twin slugs — the (a) strategy's accepted concurrency artifact —
		// must never resolve by store order (the D2 lesson at the key layer)
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-manual-twin"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e104"),
			bundle.RelationKeyApiObjectKey: domain.String("manual_property"),
			bundle.RelationKeyName:         domain.String("Manual property twin"),
		})
		name := "X"

		_, err := fx.UpdateProperty(context.Background(), testSpaceId, "manual_property",
			v2model.UpdatePropertyRequest{Name: &name}, false)

		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "Manual property twin")
	})
}

func TestV2SlugAddressedDocuments(t *testing.T) {
	t.Run("create canonicalizes slug property keys and the type slug", func(t *testing.T) {
		// given: a document naming the slug spellings — the snapshot must
		// carry the stored spellings (detail keys and the ot- URL are the
		// store's vocabulary, not the wire's)
		fx := slugSpaceFixture(t)
		captured := fx.expectCreate("obj-new")
		fx.expectEtagRead("obj-new")

		// when
		result, err := fx.CreateObject(context.Background(), testSpaceId, []byte(`{
			"version":1,"type":"meeting_note",
			"properties":{"name":"Standup","manual_property":"hello"}}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		snapshot := *captured
		assert.Equal(t, []string{"ot-" + slugTypeKey}, snapshot.ObjectTypes,
			"the type slug canonicalizes to the internal key before the ot- URL is derived")
		assert.Equal(t, "hello", pbtypes.GetString(snapshot.Details, slugPropKey),
			"the property slug canonicalizes to the stored key — details bind by stored key")
		assert.Empty(t, pbtypes.GetString(snapshot.Details, "manual_property"),
			"the slug spelling must not land as a detail key")
		_ = result
	})

	t.Run("two spellings of one property are a loud 400", func(t *testing.T) {
		fx := slugSpaceFixture(t)

		_, err := fx.CreateObject(context.Background(), testSpaceId, []byte(`{
			"version":1,"type":"meeting_note",
			"properties":{"manual_property":"a","`+slugPropKey+`":"b"}}`), false, true)

		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Message, "duplicate property key")
	})
}

func TestV2SlugAddressedOps(t *testing.T) {
	ctx := context.Background()

	t.Run("set_properties by slug writes the stored key", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"manual_property":"from-slug"}}`), "", false, true)

		// then
		require.NoError(t, err)
		st := *captured
		assert.Equal(t, "from-slug", st.CombinedDetails().GetString(domain.RelationKey(slugPropKey)),
			"the detail lands under the stored BSON key")
		assert.False(t, st.CombinedDetails().Has("manual_property"),
			"the slug spelling must not become a detail key")
	})

	t.Run("create-missing options bind to the stored key, not the slug", func(t *testing.T) {
		// given: a select property addressed by slug with a new option name —
		// the prewarm must canonicalize BEFORE the create RPC, or the option
		// is minted bound to the slug string and orphaned forever
		fx := slugSpaceFixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == slugSelectKey &&
				pbtypes.GetString(req.Details, bundle.RelationKeyName.String()) == "Brand new"
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-new",
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		})

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"mood_level":["Brand new"]}}`), "", false, true)

		// then
		require.NoError(t, err)
	})

	t.Run("the M5 bound sees folded spellings (the reviewed bypass repro)", func(t *testing.T) {
		// the review's repro: a PATCH naming 70 new options under a FOLDED
		// key spelling. Pre-fix, the prewarm (no fold) recorded zero
		// pending, guardCreateMissing short-circuited on len(pending)==0,
		// and all 70 were created inside the object lock — cap 64 lost.
		fx := slugSpaceFixture(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").
			Return(editRead(t, editBaseDoc), nil)
		// no ObjectCreateRelationOption and no mutator expectation: creating
		// anything, anywhere, fails the test — the bound must refuse first

		names := make([]string, 0, v2MaxCreatedOptionsPerPatch+6)
		for i := 0; i < v2MaxCreatedOptionsPerPatch+6; i++ {
			names = append(names, fmt.Sprintf(`"Opt %03d"`, i))
		}
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"moodLevel":[`+strings.Join(names, ",")+`]}}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "limit 64")
	})

	t.Run("a FOLDED slug spelling reaches the prewarm too (the M5 bypass)", func(t *testing.T) {
		// the reviewed regression: the prewarm lacked the fold the in-lock
		// pass had, so "moodLevel" was invisible pre-lock — the M5 bound
		// short-circuited on len(pending)==0 and the creates ran INSIDE the
		// object lock. The prewarm now walks the same chain: the option is
		// created pre-lock, bound to the stored key.
		fx := slugSpaceFixture(t)
		fx.expectMutate(editRead(t, editBaseDoc), "headB")
		fx.mwMock.EXPECT().ObjectCreateRelationOption(mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectCreateRelationOptionRequest) bool {
			return pbtypes.GetString(req.Details, bundle.RelationKeyRelationKey.String()) == slugSelectKey &&
				pbtypes.GetString(req.Details, bundle.RelationKeyName.String()) == "Folded new"
		})).Return(&pb.RpcObjectCreateRelationOptionResponse{
			ObjectId: "opt-folded",
			Error:    &pb.RpcObjectCreateRelationOptionResponseError{Code: pb.RpcObjectCreateRelationOptionResponseError_NULL},
		})

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"moodLevel":["Folded new"]}}`), "", false, true)

		require.NoError(t, err)
	})
}
