package v2service

// grant_test.go pins the service-layer half of the space-grant enforcement:
// these tests call the service DIRECTLY with a grant-carrying context — no
// route middleware in front — so they prove the backstop (ensureSpace,
// GetSpace, CreateSpace) and the fan-out intersection (ListSpaces,
// spaceRefs/GlobalSearchObjects) deny on their own, defense in depth.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// grantCtx builds a request context carrying a grant, the way the server's
// ensureAuthenticated does.
func grantCtx(perms string, spaces ...string) context.Context {
	return util.CtxWithApiGrant(context.Background(), &util.ApiGrant{Spaces: spaces, Perms: perms})
}

func requireSpaceNotGranted(t *testing.T, err error) {
	t.Helper()
	var v2Err *v2model.Error
	require.ErrorAs(t, err, &v2Err)
	assert.Equal(t, http.StatusForbidden, v2Err.Status)
	assert.Equal(t, v2model.CodeSpaceNotGranted, v2Err.Code)
	assert.Contains(t, v2Err.Message, "granted:")
}

func TestEnsureSpaceGrantBackstop(t *testing.T) {
	t.Run("a non-granted space is denied even without the route middleware", func(t *testing.T) {
		// given: the service reached directly, as if a future route forgot
		// the gate — the ensureSpace backstop must fail closed on its own
		fx := newV2Fixture(t)

		// when
		_, _, _, err := fx.ListTypes(grantCtx(util.GrantPermsReadWrite, "someOtherSpace"), testSpaceId, 0, 25)

		// then
		requireSpaceNotGranted(t, err)
	})

	t.Run("a granted space passes through ensureSpace", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, _, _, err := fx.ListTypes(grantCtx(util.GrantPermsRead, testSpaceId), testSpaceId, 0, 25)
		require.NoError(t, err)
	})

	t.Run("the tech space is denied by the backstop unless explicitly granted", func(t *testing.T) {
		// ensureSpace admits the tech space as an ordinary space id AFTER
		// the grant check — the ordering is what keeps a scoped key out of
		// the tech space when the route middleware is bypassed
		fx := newV2Fixture(t)

		_, _, _, err := fx.ListTypes(grantCtx(util.GrantPermsReadWrite, testSpaceId), objectstore.TestTechSpaceId, 0, 25)
		requireSpaceNotGranted(t, err)

		_, _, _, err = fx.ListTypes(grantCtx(util.GrantPermsReadWrite, objectstore.TestTechSpaceId), objectstore.TestTechSpaceId, 0, 25)
		require.NoError(t, err)
	})

	t.Run("an empty granted-space list denies every space", func(t *testing.T) {
		// empty must be impossible at persist time; if ever encountered it
		// must NEVER be read as unscoped
		fx := newV2Fixture(t)
		_, _, _, err := fx.ListTypes(grantCtx(util.GrantPermsReadWrite), testSpaceId, 0, 25)
		requireSpaceNotGranted(t, err)
	})

	t.Run("a nil grant (legacy key) passes unchanged", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, _, _, err := fx.ListTypes(context.Background(), testSpaceId, 0, 25)
		require.NoError(t, err)
	})

	t.Run("GetSpace consults the grant directly (it bypasses ensureSpace)", func(t *testing.T) {
		fx := newV2Fixture(t)

		_, err := fx.GetSpace(grantCtx(util.GrantPermsRead, "someOtherSpace"), testSpaceId)
		requireSpaceNotGranted(t, err)

		space, err := fx.GetSpace(grantCtx(util.GrantPermsRead, testSpaceId), testSpaceId)
		require.NoError(t, err)
		assert.Equal(t, testSpaceId, space.Id)
	})

	t.Run("a read-only grant is refused by the verb backstop even without the route middleware", func(t *testing.T) {
		// the write entry points consult ensureWriteGranted themselves
		// (ensureSpaceWrite / ensureChatWrite / UpdateSpace's direct pair):
		// a future write route that forgets the middleware still cannot
		// mutate with a read-only key
		fx := newV2Fixture(t)
		ctx := grantCtx(util.GrantPermsRead, testSpaceId)
		tests := []struct {
			name string
			call func() error
		}{
			{"CreateObject", func() error { _, err := fx.CreateObject(ctx, testSpaceId, []byte(`{}`), false); return err }},
			{"PatchObject", func() error { _, err := fx.PatchObject(ctx, testSpaceId, "obj1", []byte(`{}`), "", false); return err }},
			{"CreateType", func() error { _, err := fx.CreateType(ctx, testSpaceId, []byte(`{}`), false); return err }},
			{"DeleteProperty", func() error { _, err := fx.DeleteProperty(ctx, testSpaceId, "status", false); return err }},
			{"CreateSet", func() error { _, err := fx.CreateSet(ctx, testSpaceId, v2model.CreateSetRequest{}, false); return err }},
			{"UploadFile", func() error { _, err := fx.UploadFile(ctx, testSpaceId, "", "", false); return err }},
			{"CreateChat", func() error {
				_, err := fx.CreateChat(ctx, testSpaceId, v2model.CreateChatRequest{Name: "c"}, false)
				return err
			}},
			// the read-watermark advance is a WRITE, and the verb check runs
			// BEFORE the chat lookup — no chat fixture needed
			{"ReadChat", func() error {
				_, err := fx.ReadChat(ctx, testSpaceId, "chat1", v2model.ChatReadRequest{}, false)
				return err
			}},
			{"UpdateSpace", func() error {
				_, err := fx.UpdateSpace(ctx, testSpaceId, v2model.UpdateSpaceRequest{}, false)
				return err
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.call()

				var v2Err *v2model.Error
				require.ErrorAs(t, err, &v2Err)
				assert.Equal(t, http.StatusForbidden, v2Err.Status)
				assert.Equal(t, v2model.CodeWriteNotGranted, v2Err.Code)
				assert.Contains(t, v2Err.Message, "read-only")
			})
		}
	})

	t.Run("space_not_granted wins over write_not_granted on the write backstop", func(t *testing.T) {
		// route-gate precedence: the space check runs first, so a read-only
		// key writing into a NON-granted space is told about the space
		fx := newV2Fixture(t)

		_, err := fx.CreateObject(grantCtx(util.GrantPermsRead, "someOtherSpace"), testSpaceId, []byte(`{}`), false)

		requireSpaceNotGranted(t, err)
	})

	t.Run("a readwrite grant passes the verb backstop", func(t *testing.T) {
		// the next failure is the ordinary validation error — the check fell
		// through instead of short-circuiting readwrite keys
		fx := newV2Fixture(t)

		_, err := fx.CreateChat(grantCtx(util.GrantPermsReadWrite, testSpaceId),
			testSpaceId, v2model.CreateChatRequest{}, false)

		var v2Err *v2model.Error
		require.ErrorAs(t, err, &v2Err)
		assert.Equal(t, v2model.CodeValidationFailed, v2Err.Code)
	})

	t.Run("CreateSpace refuses every granted key at the service layer too", func(t *testing.T) {
		// a key that can mint spaces it then owns is not meaningfully
		// scoped — the route gate denies POST /v2/spaces, and this is the
		// backstop for a path that skips it
		fx := newV2Fixture(t)

		_, err := fx.CreateSpace(grantCtx(util.GrantPermsReadWrite, testSpaceId),
			v2model.CreateSpaceRequest{Name: "New"}, false)

		var v2Err *v2model.Error
		require.True(t, errors.As(err, &v2Err))
		assert.Equal(t, v2model.CodeSpaceNotGranted, v2Err.Code)
		assert.Contains(t, v2Err.Message, "cannot create spaces")
	})
}

func TestListSpacesGrantIntersection(t *testing.T) {
	t.Run("three live spaces, grant covers one", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		for _, space := range []struct{ id, name string }{
			{"spaceA", "Work"}, {"spaceB", "Personal"}, {"spaceC", "Diary"},
		} {
			fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:             domain.String("spaceView_" + space.id),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:  domain.String(space.id),
				bundle.RelationKeyName:           domain.String(space.name),
			}})
		}
		want := []v2model.SpaceRow{{Id: "spaceA", Name: "Work"}}

		// when
		rows, total, hasMore, err := fx.ListSpaces(grantCtx(util.GrantPermsRead, "spaceA"), 0, 25)

		// then: the grant intersects the space set — total counts only
		// granted spaces, so even the COUNT of others is not disclosed
		require.NoError(t, err)
		assert.Equal(t, want, rows)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
	})
}

func TestGlobalSearchGrantIntersection(t *testing.T) {
	// given: an object in each of two live spaces, grant covers space1 only
	setup := func(t *testing.T) *v2Fixture {
		fx := newV2Fixture(t)
		fx.registerSpace(t, "space2")
		for spaceId, objectId := range map[string]string{testSpaceId: "docA", "space2": "docB"} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:               domain.String(objectId),
				bundle.RelationKeyName:             domain.String("Doc in " + spaceId),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
			}})
		}
		return fx
	}

	t.Run("results come only from granted spaces, totals included", func(t *testing.T) {
		fx := setup(t)

		rows, total, hasMore, warnings, err := fx.GlobalSearchObjects(
			grantCtx(util.GrantPermsRead, testSpaceId), v2model.SearchRequest{}, 0, 25)

		require.NoError(t, err)
		assert.Equal(t, []string{"docA"}, rowIds(rows))
		assert.Equal(t, 1, total, "total must not count non-granted spaces")
		assert.False(t, hasMore)
		assert.Empty(t, warnings)
	})

	t.Run("a non-granted space never appears in warnings", func(t *testing.T) {
		// the intersection happens on the INPUT set (spaceRefs), not on the
		// output rows: the probe is a type that resolves ONLY in the granted
		// space, the exact shape that makes an intersected-away space emit
		// `space "space2" was skipped` if it enters the fan-out loop — which
		// would disclose that the space exists. Warnings must be EMPTY, not
		// merely free of the id: the skip warning names the space by name.
		fx := setup(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-chore"),
				bundle.RelationKeyName:           domain.String("Chore"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-chore"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:               domain.String("chore1"),
				bundle.RelationKeyName:             domain.String("A chore"),
				bundle.RelationKeyType:             domain.String("type-chore"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(2000),
			},
		})

		rows, total, _, warnings, err := fx.GlobalSearchObjects(
			grantCtx(util.GrantPermsRead, testSpaceId), v2model.SearchRequest{Type: "chore"}, 0, 25)

		require.NoError(t, err)
		assert.Equal(t, []string{"chore1"}, rowIds(rows))
		assert.Equal(t, 1, total)
		require.Empty(t, warnings, "a skip warning here could only name the non-granted space")
	})

	t.Run("a nil grant fans out over every space unchanged", func(t *testing.T) {
		fx := setup(t)

		rows, total, _, _, err := fx.GlobalSearchObjects(
			context.Background(), v2model.SearchRequest{}, 0, 25)

		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"docA", "docB"}, rowIds(rows))
		assert.Equal(t, 2, total)
	})
}
