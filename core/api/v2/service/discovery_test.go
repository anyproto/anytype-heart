package v2service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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
		want := []v2model.SpaceRow{
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

	t.Run("rows carry the description — no GET-one hop to disambiguate", func(t *testing.T) {
		// given: description sits in the same tech-space record for free;
		// withholding it forced 1+N reads on "list my spaces, pick one"
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceS", "Work", "The local-first wiki")

		// when
		rows, _, _, err := fx.ListSpaces(context.Background(), 0, 25)

		// then
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "The local-first wiki", rows[0].Description)
	})

	t.Run("deleted and non-active spaces are filtered out (the live predicate)", func(t *testing.T) {
		// given: a live space and a deleted one — v1's GetSpace filters both
		// status axes; a dead row is indistinguishable from a live one and an
		// agent picking it would write into a space that can never load
		fx := newV2FixtureBare(t)
		fx.registerSpaceView(t, "spaceLive", "Live", "")
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                 domain.String("spaceView_dead"),
			bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:      domain.String("deadSpace"),
			bundle.RelationKeyName:               domain.String("Deleted space"),
			bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceDeleted)),
			bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(model.SpaceStatus_Missing)),
		}})

		// when
		rows, total, _, err := fx.ListSpaces(context.Background(), 0, 25)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, rows, 1)
		assert.Equal(t, "spaceLive", rows[0].Id)
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
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusNotFound, apiErr.Status)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
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

	t.Run("a deleted space is refused by every nested discovery list", func(t *testing.T) {
		const deadSpace = "deadSpace"
		calls := map[string]func(*v2Fixture) error{
			"objects": func(fx *v2Fixture) error {
				_, _, _, err := fx.ListObjects(context.Background(), deadSpace, nil, 0, 25)
				return err
			},
			"members": func(fx *v2Fixture) error {
				_, _, _, err := fx.ListMembers(context.Background(), deadSpace, 0, 25)
				return err
			},
			"types": func(fx *v2Fixture) error {
				_, _, _, err := fx.ListTypes(context.Background(), deadSpace, 0, 25)
				return err
			},
			"properties": func(fx *v2Fixture) error {
				_, _, _, err := fx.ListProperties(context.Background(), deadSpace, 0, 25)
				return err
			},
		}
		for name, call := range calls {
			t.Run(name, func(t *testing.T) {
				fx := newV2FixtureBare(t)
				fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
					bundle.RelationKeyId:                 domain.String("spaceView_" + deadSpace),
					bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
					bundle.RelationKeyTargetSpaceId:      domain.String(deadSpace),
					bundle.RelationKeyName:               domain.String("Deleted space"),
					bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceDeleted)),
					bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(model.SpaceStatus_Missing)),
				}})

				apiErr := v2Err(t, call(fx))
				assert.Equal(t, http.StatusNotFound, apiErr.Status)
				assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
				assert.Equal(t, `space "deadSpace" is not available (deleted, left, or still joining) — list live spaces with GET /v2/spaces`, apiErr.Message)
			})
		}
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
		want := []v2model.MemberRow{
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

func TestV2GetMemberMe(t *testing.T) {
	meId := domain.NewParticipantId(testSpaceId, testAccountId)

	t.Run("returns the caller's member row from the store", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:                     domain.String(meId),
				bundle.RelationKeyName:                   domain.String("Me Myself"),
				bundle.RelationKeyResolvedLayout:         domain.Int64(int64(model.ObjectType_participant)),
				bundle.RelationKeyParticipantPermissions: domain.Int64(int64(model.ParticipantPermissions_Owner)),
			},
		})
		want := v2model.MemberRow{Id: meId, Name: "Me Myself", Role: "owner", Identity: testAccountId}

		// when
		row, err := fx.GetMemberMe(context.Background(), testSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, row)
	})

	t.Run("serves the deterministic id before the participant object is indexed", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		want := v2model.MemberRow{Id: meId, Identity: testAccountId}

		// when
		row, err := fx.GetMemberMe(context.Background(), testSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, row, "the id is what assignee values need — served without a store row")
	})

	t.Run("no account identity is a 404 steering to the members list", func(t *testing.T) {
		// given: a service constructed without an account id (degraded mode)
		fx := newV2Fixture(t)
		svc := NewService(fx.mwMock, fx.readerMock, fx.creatorMock, fx.mutatorMock, nil, nil, fx.objectStore, objectstore.TestTechSpaceId, "")

		// when
		_, err := svc.GetMemberMe(context.Background(), testSpaceId)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
		assert.Contains(t, apiErr.Message, "GET /v2/spaces/{space_id}/members")
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
		want := []v2model.TypeRow{{Key: "task", Name: "Task"}}

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
		body, etag, err := fx.GetType(context.Background(), testSpaceId, "task", ObjectQuery{})

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, etag)
		assert.NotEmpty(t, body)
	})

	t.Run("?ids= rides through to the type read — the export shape is one query parameter away", func(t *testing.T) {
		// given: a type document with minted-shape block ids; GetType used to
		// hardcode ObjectQuery{}, so §8.25's "the export shape is one query
		// parameter away" was false for types
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-task"),
				bundle.RelationKeyName:           domain.String("Task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
		})
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-task").Return(testObjectReadLongIds(), nil).Times(2)

		// when / then: default = labels, full = the stored ids
		compact, _, err := fx.GetType(context.Background(), testSpaceId, "task", ObjectQuery{})
		require.NoError(t, err)
		assert.Contains(t, string(compact), `"id":"bbbb1"`, "the default type read is the edit shape")

		full, _, err := fx.GetType(context.Background(), testSpaceId, "task", ObjectQuery{Ids: V2IdsFull})
		require.NoError(t, err)
		assert.Contains(t, string(full), `"id":"`+testMintedParentId+`"`, "?ids=full serves the stored ids")
	})

	t.Run("unknown key is a 404 listing the space's type keys with did-you-mean", func(t *testing.T) {
		// given: the candidate-less form of this tip was a dead end — a
		// benchmarked small model did not retry at all (§8.21); the message
		// must carry the actual keys and the nearest match, like the
		// property-key path always did
		fx := newV2Fixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-task"),
				bundle.RelationKeyName:           domain.String("Task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:             domain.String("type-page"),
				bundle.RelationKeyName:           domain.String("Page"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-page"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
		})

		// the benchmark's literal miss — the Title-Case guess — now resolves
		// through the §7.5a-3 fold layer (exact-first, fold as fallback):
		// zero retries instead of one repaired retry (§8.21)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "type-page").Return(testObjectRead(), nil)
		_, _, err := fx.GetType(context.Background(), testSpaceId, "Page", ObjectQuery{})
		require.NoError(t, err)

		// when: a genuine miss (no fold candidate) keeps the keyed 404
		_, _, err = fx.GetType(context.Background(), testSpaceId, "Pages", ObjectQuery{})

		// then
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Contains(t, v2Err.Message, "known type keys: page, task")
		assert.Contains(t, v2Err.Message, "did you mean page?")
	})

	t.Run("unknown key in an empty space says so", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, _, err := fx.GetType(context.Background(), testSpaceId, "nope", ObjectQuery{})

		// then
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Contains(t, v2Err.Message, "the space has no type keys yet")
	})
}

func TestV2GetTypeSchema(t *testing.T) {
	t.Run("an accessible space gets the designed 501", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		err := fx.GetTypeSchema(context.Background(), testSpaceId, "task")

		// then
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 501, v2Err.Status)
		assert.Equal(t, v2model.CodeNotImplemented, v2Err.Code)
	})

	t.Run("an unknown space is a 404 before the 501", func(t *testing.T) {
		// The stub resolves the space first, like every space-scoped route —
		// ensureSpace is a resource guard, not a formality, and the 404 is the
		// one refusal here a caller can act on. The published document said
		// "every request answers 501" and declared no 404, which is the drift
		// this pins: the ordering is deliberate, so the contract states it.
		fx := newV2Fixture(t)

		err := fx.GetTypeSchema(context.Background(), "bafybeimissingspaceforv2typeschema", "task")

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Equal(t, v2model.CodeNotFound, v2Err.Code)
	})
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
		want := []v2model.PropertyRow{{Key: "priority", Name: "Priority", Format: "select"}}

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
		want := []v2model.OptionRow{{Name: "High", Color: "red"}, {Name: "Low"}}

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

	t.Run("a case-variant key resolves through the fold layer", func(t *testing.T) {
		// §7.5a-3: exact match first, fold (lowercase, _- stripped) as the
		// forgiving fallback — the Title-Case guess works without a retry
		fx := newV2Fixture(t)
		addOptions(fx, t)

		rows, _, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "Priority", "", 0, 25)

		require.NoError(t, err)
		require.Len(t, rows, 2)
	})

	t.Run("unknown property is a 404 listing the space's keys with did-you-mean", func(t *testing.T) {
		// given: the same candidate-listing contract as the type path (§8.21
		// — the family is fixed together, not one string)
		fx := newV2Fixture(t)
		addOptions(fx, t)

		// when: a genuine miss — no fold candidate either
		_, _, _, err := fx.ListPropertyOptions(context.Background(), testSpaceId, "prio", "", 0, 25)

		// then
		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, 404, v2Err.Status)
		assert.Contains(t, v2Err.Message, "known property keys: priority")
		assert.Contains(t, v2Err.Message, "did you mean priority?")
	})
}
