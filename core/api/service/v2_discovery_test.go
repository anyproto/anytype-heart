package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestV2ListSpaces(t *testing.T) {
	t.Run("space views become minimal rows", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("spaceView1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
				bundle.RelationKeyName:           domain.String("Work"),
			},
			{
				bundle.RelationKeyId:             domain.String("spaceView2"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:  domain.String("space2"),
				bundle.RelationKeyName:           domain.String("Personal"),
			},
		})
		want := []apimodel.V2SpaceRow{
			{Id: "space1", Name: "Work"},
			{Id: "space2", Name: "Personal"},
		}

		// when
		rows, total, hasMore, err := fx.ListSpaces(context.Background(), 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("empty tech space lists nothing", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)

		// when
		rows, total, _, err := fx.ListSpaces(context.Background(), 0, 25)

		// then
		require.NoError(t, err)
		assert.Empty(t, rows)
		assert.Zero(t, total)
	})
}

func TestV2EnsureSpace(t *testing.T) {
	// C2: an unknown space_id must be rejected with 404 before any per-space
	// objectstore access, so a bogus id cannot mint an unbounded store index.
	t.Run("unknown space is rejected 404 without touching the store", func(t *testing.T) {
		// given: fixture registers only testSpaceId
		fx := newV2Fixture(t)

		// when: a space that has no spaceView
		_, _, _, err := fx.ListObjects(context.Background(), "bogus-space", nil, 0, 25)

		// then
		var apiErr *apimodel.V2Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Equal(t, apimodel.V2CodeNotFound, apiErr.Code)
	})

	t.Run("a registered space passes the guard", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when: testSpaceId is registered by the fixture
		_, total, _, err := fx.ListObjects(context.Background(), testSpaceId, nil, 0, 25)

		// then: no space error (empty result is fine)
		require.NoError(t, err)
		assert.Zero(t, total)
	})
}

func TestV2ListMembers(t *testing.T) {
	t.Run("active members become rows with roles", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                     domain.String("_participant_a"),
				bundle.RelationKeyName:                   domain.String("Alice"),
				bundle.RelationKeyResolvedLayout:         domain.Int64(int64(model.ObjectType_participant)),
				bundle.RelationKeyParticipantStatus:      domain.Int64(int64(model.ParticipantStatus_Active)),
				bundle.RelationKeyParticipantPermissions: domain.Int64(int64(model.ParticipantPermissions_Owner)),
				bundle.RelationKeyIdentity:               domain.String("idA"),
			},
			{
				bundle.RelationKeyId:                     domain.String("_participant_b"),
				bundle.RelationKeyName:                   domain.String("Bob"),
				bundle.RelationKeyResolvedLayout:         domain.Int64(int64(model.ObjectType_participant)),
				bundle.RelationKeyParticipantStatus:      domain.Int64(int64(model.ParticipantStatus_Joining)),
				bundle.RelationKeyParticipantPermissions: domain.Int64(int64(model.ParticipantPermissions_Reader)),
			},
		})
		want := []apimodel.V2MemberRow{
			{Id: "_participant_a", Name: "Alice", Role: "owner", Identity: "idA"},
		}

		// when
		rows, total, hasMore, err := fx.ListMembers(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows, "joining members are not listed")
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
	})
}

func TestV2ListTypes(t *testing.T) {
	t.Run("types become key+name rows", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-task"),
				bundle.RelationKeyName:           domain.String("Task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:             domain.String("type-hidden"),
				bundle.RelationKeyName:           domain.String("Hidden"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-hidden"),
				bundle.RelationKeyIsHidden:       domain.Bool(true),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
		})
		want := []apimodel.V2TypeRow{{Key: "task", Name: "Task"}}

		// when
		rows, total, hasMore, err := fx.ListTypes(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows, "hidden types are not listed")
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
	})
}

func TestV2GetType(t *testing.T) {
	t.Run("resolves the key and reads the type document", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-task"),
				bundle.RelationKeyName:           domain.String("Task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
		})
		read := testObjectRead()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-task").Return(read, nil)

		// when
		body, etag, err := fx.GetType(context.Background(), testSpaceId, "task")

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, etag)
		assert.NotEmpty(t, body)
	})

	t.Run("unknown key is a 404 steering to the types list", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, _, err := fx.GetType(context.Background(), testSpaceId, "nope")

		// then
		var v2Err *apimodel.V2Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Contains(t, v2Err.Message, "/types")
	})
}

func TestV2GetTypeSchema(t *testing.T) {
	// given
	fx := newV2Fixture(t)

	// when
	err := fx.GetTypeSchema(context.Background(), testSpaceId, "task")

	// then
	var v2Err *apimodel.V2Error
	require.ErrorAs(t, err, &v2Err)
	assert.Equal(t, 501, v2Err.Status)
	assert.Equal(t, apimodel.V2CodeNotImplemented, v2Err.Code)
}

func TestV2ListProperties(t *testing.T) {
	t.Run("properties become key+name+format rows", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-priority"),
				bundle.RelationKeyRelationKey:    domain.String("priority"),
				bundle.RelationKeyName:           domain.String("Priority"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
		})
		want := []apimodel.V2PropertyRow{{Key: "priority", Name: "Priority", Format: "select"}}

		// when
		rows, total, hasMore, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
	})
}

func TestV2ListPropertyOptions(t *testing.T) {
	addOptions := func(fx *v2Fixture, t *testing.T) {
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel-priority"),
				bundle.RelationKeyRelationKey:    domain.String("priority"),
				bundle.RelationKeyName:           domain.String("Priority"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:                  domain.String("opt-high"),
				bundle.RelationKeyRelationKey:         domain.String("priority"),
				bundle.RelationKeyName:                domain.String("High"),
				bundle.RelationKeyRelationOptionColor: domain.String("red"),
				bundle.RelationKeyResolvedLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				bundle.RelationKeyId:             domain.String("opt-low"),
				bundle.RelationKeyRelationKey:    domain.String("priority"),
				bundle.RelationKeyName:           domain.String("Low"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
			},
		})
	}

	t.Run("options become name+color rows", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		addOptions(fx, t)
		want := []apimodel.V2OptionRow{{Name: "High", Color: "red"}, {Name: "Low"}}

		// when
		rows, total, hasMore, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "priority", "", 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 2, total)
		assert.False(t, hasMore)
	})

	t.Run("prefix filters case-insensitively", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		addOptions(fx, t)

		// when
		rows, total, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "priority", "hi", 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "High", rows[0].Name)
		assert.Equal(t, 1, total)
	})

	t.Run("unknown property is a 404 steering to the properties list", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, _, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "nope", "", 0, 25)

		// then
		var v2Err *apimodel.V2Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Contains(t, v2Err.Message, "/properties")
	})
}
