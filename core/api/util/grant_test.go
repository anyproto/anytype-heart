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
			name:  "allSpaces rides the conversion",
			proto: &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_ReadWrite},
			want:  &ApiGrant{AllSpaces: true, Perms: GrantPermsReadWrite},
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

func TestApiGrantAllSpaces(t *testing.T) {
	t.Run("allSpaces covers any space id, including one created later", func(t *testing.T) {
		// given: dynamic semantics — the grant is not a snapshot of the
		// spaces that existed at approval
		grant := &ApiGrant{AllSpaces: true, Perms: GrantPermsRead}

		// then
		assert.True(t, grant.AllowsSpace("space1"))
		assert.True(t, grant.AllowsSpace("space-created-after-approval"))
		assert.False(t, grant.AllowsSpace(""), "an empty space id stays denied")
	})
}

func TestApiGrantIsUnrestricted(t *testing.T) {
	tests := []struct {
		name  string
		grant *ApiGrant
		want  bool
	}{
		{
			name:  "allSpaces readwrite is the one unrestricted shape",
			grant: &ApiGrant{AllSpaces: true, Perms: GrantPermsReadWrite},
			want:  true,
		},
		{
			name:  "allSpaces read-only is restricted",
			grant: &ApiGrant{AllSpaces: true, Perms: GrantPermsRead},
			want:  false,
		},
		{
			name:  "a space-listed readwrite grant is restricted",
			grant: &ApiGrant{Spaces: []string{"space1"}, Perms: GrantPermsReadWrite},
			want:  false,
		},
		{
			name:  "nil is not unrestricted — the legacy branch is the caller's",
			grant: nil,
			want:  false,
		},
		{
			name:  "an unknown perms value fails closed",
			grant: &ApiGrant{AllSpaces: true, Perms: "admin"},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.grant.IsUnrestricted())
		})
	}
}

func TestSpaceGrantRefusal(t *testing.T) {
	const techSpaceId = "techSpace1"
	tests := []struct {
		name    string
		grant   *ApiGrant
		spaceId string
		want    string // "" = admitted; else a substring of the refusal
	}{
		{
			name:    "nil grant is admitted unchanged",
			grant:   nil,
			spaceId: techSpaceId,
			want:    "",
		},
		{
			name:    "allSpaces admits an ordinary space",
			grant:   &ApiGrant{AllSpaces: true, Perms: GrantPermsRead},
			spaceId: "space1",
			want:    "",
		},
		{
			name: "allSpaces never covers the tech space",
			// the tech space holds account machinery, not user content; the
			// refusal says so instead of reading as a heart bug to a user
			// who just granted "all spaces"
			grant:   &ApiGrant{AllSpaces: true, Perms: GrantPermsReadWrite},
			spaceId: techSpaceId,
			want:    "never covered by an all-spaces grant",
		},
		{
			name:    "an explicit listing still opens the tech space",
			grant:   &ApiGrant{Spaces: []string{"space1", techSpaceId}, Perms: GrantPermsReadWrite},
			spaceId: techSpaceId,
			want:    "",
		},
		{
			name:    "a narrow grant refuses a non-listed space, naming the grant",
			grant:   &ApiGrant{Spaces: []string{"space1"}, Perms: GrantPermsRead},
			spaceId: "space2",
			want:    "granted: spaces [space1] with read access",
		},
		{
			name:    "an allSpaces refusal renders the grant as all spaces, not an empty list",
			grant:   &ApiGrant{AllSpaces: true, Perms: GrantPermsRead},
			spaceId: "",
			want:    "granted: all spaces with read access",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SpaceGrantRefusal(tt.grant, tt.spaceId, techSpaceId)
			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.want)
			}
		})
	}
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
