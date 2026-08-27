package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestApiGrantFromProto(t *testing.T) {
	tests := []struct {
		name  string
		proto *model.AccountAuthAppGrant
		want  *ApiGrant
	}{
		{
			name:  "nil proto is a nil grant (unscoped key)",
			proto: nil,
			want:  nil,
		},
		{
			name:  "read maps to read",
			proto: &model.AccountAuthAppGrant{SpaceIds: []string{"space1"}, Perm: model.AccountAuthAppGrant_Read},
			want:  &ApiGrant{Spaces: []string{"space1"}, Perms: GrantPermsRead},
		},
		{
			name:  "readwrite maps to readwrite",
			proto: &model.AccountAuthAppGrant{SpaceIds: []string{"space1", "space2"}, Perm: model.AccountAuthAppGrant_ReadWrite},
			want:  &ApiGrant{Spaces: []string{"space1", "space2"}, Perms: GrantPermsReadWrite},
		},
		{
			name: "an unknown perm enum maps to read, never readwrite",
			// a future enum value this binary does not know must not widen
			// into write access
			proto: &model.AccountAuthAppGrant{SpaceIds: []string{"space1"}, Perm: model.AccountAuthAppGrantPerm(99)},
			want:  &ApiGrant{Spaces: []string{"space1"}, Perms: GrantPermsRead},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ApiGrantFromProto(tt.proto))
		})
	}
}

func TestApiGrantAllowsSpace(t *testing.T) {
	tests := []struct {
		name    string
		grant   *ApiGrant
		spaceId string
		want    bool
	}{
		{
			name:    "granted space passes",
			grant:   &ApiGrant{Spaces: []string{"space1", "space2"}, Perms: GrantPermsRead},
			spaceId: "space2",
			want:    true,
		},
		{
			name:    "non-granted space is denied",
			grant:   &ApiGrant{Spaces: []string{"space1"}, Perms: GrantPermsRead},
			spaceId: "space2",
			want:    false,
		},
		{
			name: "an EMPTY space list denies every space — it is never all spaces",
			// persist-time validation rejects empty lists; if one is ever
			// encountered anyway it must deny, not widen
			grant:   &ApiGrant{Spaces: []string{}, Perms: GrantPermsReadWrite},
			spaceId: "space1",
			want:    false,
		},
		{
			name: "a nil receiver denies (callers branch on nil BEFORE calling)",
			// a caller that forgets the nil-means-unscoped branch fails
			// closed, not open
			grant:   nil,
			spaceId: "space1",
			want:    false,
		},
		{
			name:    "the empty space id is denied",
			grant:   &ApiGrant{Spaces: []string{""}, Perms: GrantPermsRead},
			spaceId: "",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.grant.AllowsSpace(tt.spaceId))
		})
	}
}

func TestApiGrantCanWrite(t *testing.T) {
	assert.True(t, (&ApiGrant{Spaces: []string{"s"}, Perms: GrantPermsReadWrite}).CanWrite())
	assert.False(t, (&ApiGrant{Spaces: []string{"s"}, Perms: GrantPermsRead}).CanWrite())
	// unknown or empty perms are read at most — fail closed
	assert.False(t, (&ApiGrant{Spaces: []string{"s"}, Perms: "admin"}).CanWrite())
	assert.False(t, (&ApiGrant{Spaces: []string{"s"}}).CanWrite())
	assert.False(t, (*ApiGrant)(nil).CanWrite())
}

func TestApiGrantCtxRoundTrip(t *testing.T) {
	t.Run("grant rides the context", func(t *testing.T) {
		// given
		want := &ApiGrant{Spaces: []string{"space1"}, Perms: GrantPermsRead}

		// when
		ctx := CtxWithApiGrant(context.Background(), want)

		// then
		require.Equal(t, want, ApiGrantFromCtx(ctx))
	})

	t.Run("absent grant reads as nil (unscoped)", func(t *testing.T) {
		assert.Nil(t, ApiGrantFromCtx(context.Background()))
	})

	t.Run("a stored nil grant reads as nil", func(t *testing.T) {
		ctx := CtxWithApiGrant(context.Background(), nil)
		assert.Nil(t, ApiGrantFromCtx(ctx))
	})
}

func TestBearerChallenges(t *testing.T) {
	// the header values are wire surface MCP clients parse — pin them
	assert.Equal(t, `Bearer realm="anytype"`, BearerChallenge())
	assert.Equal(t, `Bearer realm="anytype", error="invalid_token"`, BearerChallengeInvalidToken())
	assert.Equal(t, `Bearer error="insufficient_scope"`, BearerChallengeInsufficientScope(""))
	assert.Equal(t, `Bearer error="insufficient_scope", scope="space:space1:readwrite"`,
		BearerChallengeInsufficientScope(SpaceScope("space1", GrantPermsReadWrite)))
}
