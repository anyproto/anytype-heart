package v2service

import (
	"context"
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

// whoamiCtx builds the request context the way ensureAuthenticated does:
// key info and grant on the same carriers the enforcement path reads.
func whoamiCtx(info util.ApiKeyInfo, grant *util.ApiGrant) context.Context {
	ctx := util.CtxWithApiKeyInfo(context.Background(), info)
	return util.CtxWithApiGrant(ctx, grant)
}

// registerNamedSpace adds a live named space view to the fixture's tech
// space, the record ListSpaces (and therefore whoami's name resolution)
// reads.
func (fx *v2Fixture) registerNamedSpace(t *testing.T, spaceId, name string) {
	fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("spaceView_" + spaceId),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
		bundle.RelationKeyName:           domain.String(name),
	}})
}

func TestWhoamiService(t *testing.T) {
	keyInfo := util.ApiKeyInfo{
		Id:        "hash1",
		Name:      "Claude Desktop",
		CreatedAt: 1700000000,
		Scope:     model.AccountAuth_JsonAPI,
	}

	t.Run("legacy key: explicit scoped false, empty spaces array, null permission", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		createdAt := "2023-11-14T22:13:20Z"
		want := v2model.WhoamiResponse{
			Key:       v2model.WhoamiKey{Id: "hash1", Name: "Claude Desktop", CreatedAt: &createdAt},
			Scope:     "jsonApi",
			Grant:     v2model.WhoamiGrant{Scoped: false, Permission: nil, Spaces: []v2model.WhoamiGrantSpace{}},
			Api:       v2model.WhoamiApi{Version: util.ApiVersion},
			KeyStatus: util.KeyStatusLegacy,
			Notice:    util.LegacyKeyNotice,
		}

		// when
		got, err := fx.Whoami(whoamiCtx(keyInfo, nil))

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.NotNil(t, got.Grant.Spaces, "spaces must be [], never null — null reads fail-open")
	})

	t.Run("scoped key: names from the same grant-intersected list GET /v2/spaces serves", func(t *testing.T) {
		// given: two live named spaces, one granted; plus a granted space id
		// with no live view — the grant record stays authoritative for WHICH
		// spaces appear, the live list only contributes names
		fx := newV2FixtureBare(t)
		fx.registerNamedSpace(t, "spaceA", "Work")
		fx.registerNamedSpace(t, "spaceB", "Personal")
		grant := &util.ApiGrant{Spaces: []string{"spaceA", "ghostSpace"}, Perms: util.GrantPermsRead}
		perms := util.GrantPermsRead
		want := v2model.WhoamiGrant{
			Scoped:     true,
			Permission: &perms,
			Spaces: []v2model.WhoamiGrantSpace{
				{Id: "spaceA", Name: "Work", Permission: util.GrantPermsRead},
				{Id: "ghostSpace", Name: "", Permission: util.GrantPermsRead},
			},
		}

		// when
		got, err := fx.Whoami(whoamiCtx(keyInfo, grant))

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got.Grant)
		assert.Equal(t, util.KeyStatusScoped, got.KeyStatus)
		assert.Empty(t, got.Notice, "the notice is legacy-only")
		for _, space := range got.Grant.Spaces {
			assert.NotEqual(t, "Personal", space.Name, "a non-granted space's name must never appear")
		}
	})

	t.Run("zero timestamps render null, set ones RFC 3339 UTC", func(t *testing.T) {
		// given
		fx := newV2FixtureBare(t)
		info := util.ApiKeyInfo{Id: "hash2", Name: "cli", ExpiresAt: 1900000000, Scope: model.AccountAuth_Full}

		// when
		got, err := fx.Whoami(whoamiCtx(info, nil))

		// then
		require.NoError(t, err)
		assert.Nil(t, got.Key.CreatedAt)
		require.NotNil(t, got.Key.ExpiresAt)
		assert.Equal(t, "2030-03-17T17:46:40Z", *got.Key.ExpiresAt)
		assert.Equal(t, "full", got.Scope)
	})

	t.Run("the notice addresses only JSON-API keys", func(t *testing.T) {
		// given: a Full credential is nil-grant FOREVER — a grant is only
		// ever valid on JsonAPI scope (wallet.ValidateAppLinkGrant) — so the
		// re-issue advice is impossible to follow and must not be given; the
		// status itself stays legacy (grant presence decides, unconditionally)
		fx := newV2FixtureBare(t)
		info := util.ApiKeyInfo{Id: "hashFull", Name: "desktop", Scope: model.AccountAuth_Full}

		// when
		got, err := fx.Whoami(whoamiCtx(info, nil))

		// then
		require.NoError(t, err)
		assert.Equal(t, util.KeyStatusLegacy, got.KeyStatus)
		assert.Empty(t, got.Notice, "a non-JSON-API key cannot be re-issued as scoped — no advice")
	})

	t.Run("name resolution itself flows through the grant-intersected list", func(t *testing.T) {
		// given: two LIVE named spaces, one granted. This pins the MECHANISM,
		// not just Whoami's output: resolveGrantedSpaceNames returns exactly
		// what the grant-intersected ListSpaces yields, with no second filter
		// in front of this assertion — so if resolution ever bypasses the
		// ctx-grant intersection (a stripped grant, a widened limit), the
		// non-granted live name lands in the map and fails here even though
		// Whoami's outer loop over grant.Spaces would still mask it there.
		fx := newV2FixtureBare(t)
		fx.registerNamedSpace(t, "spaceA", "Work")
		fx.registerNamedSpace(t, "spaceB", "Personal")
		grant := &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsRead}
		want := map[string]string{"spaceA": "Work"}

		// when
		got, _, err := fx.resolveGrantedSpaceNames(util.CtxWithApiGrant(context.Background(), grant), grant)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a request without an authenticated session fails closed", func(t *testing.T) {
		fx := newV2FixtureBare(t)
		_, err := fx.Whoami(context.Background())
		require.Error(t, err)
	})
}
