package spaceinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestSpaceLocalInfo_Equal(t *testing.T) {
	stored := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyTargetSpaceId:        domain.String("spaceId"),
		bundle.RelationKeySpaceLocalStatus:     domain.Int64(int64(LocalStatusOk)),
		bundle.RelationKeySpaceRemoteStatus:    domain.Int64(int64(RemoteStatusOk)),
		bundle.RelationKeySpaceShareableStatus: domain.Int64(int64(ShareableStatusShareable)),
		bundle.RelationKeyWritersLimit:         domain.Int64(10),
		bundle.RelationKeyReadersLimit:         domain.Int64(20),
	})

	t.Run("equal when all set fields match", func(t *testing.T) {
		info := NewSpaceLocalInfo("spaceId")
		info.SetLocalStatus(LocalStatusOk).
			SetRemoteStatus(RemoteStatusOk).
			SetShareableStatus(ShareableStatusShareable).
			SetWriteLimit(10).
			SetReadLimit(20)
		assert.True(t, info.Equal(stored))
	})

	t.Run("equal when only a subset of fields is set and matches", func(t *testing.T) {
		info := NewSpaceLocalInfo("spaceId")
		info.SetLocalStatus(LocalStatusOk)
		assert.True(t, info.Equal(stored))
	})

	t.Run("not equal when a set field differs", func(t *testing.T) {
		info := NewSpaceLocalInfo("spaceId")
		info.SetLocalStatus(LocalStatusLoading)
		assert.False(t, info.Equal(stored))
	})

	t.Run("not equal when read limit differs", func(t *testing.T) {
		info := NewSpaceLocalInfo("spaceId")
		info.SetReadLimit(99)
		assert.False(t, info.Equal(stored))
	})

	t.Run("not equal on space id mismatch", func(t *testing.T) {
		info := NewSpaceLocalInfo("otherSpace")
		info.SetLocalStatus(LocalStatusOk)
		assert.False(t, info.Equal(stored))
	})

	t.Run("not equal on nil details", func(t *testing.T) {
		info := NewSpaceLocalInfo("spaceId")
		assert.False(t, info.Equal(nil))
	})

	t.Run("equal against empty details when nothing but matching space id", func(t *testing.T) {
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyTargetSpaceId: domain.String("spaceId"),
		})
		info := NewSpaceLocalInfo("spaceId")
		assert.True(t, info.Equal(details))
	})
}

func TestSpacePersistentInfo_Equal(t *testing.T) {
	stored := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyTargetSpaceId:      domain.String("spaceId"),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(AccountStatusActive)),
		bundle.RelationKeyLatestAclHeadId:    domain.String("aclHead"),
		bundle.RelationKeyName:               domain.String("My space"),
	})

	t.Run("equal when set fields match", func(t *testing.T) {
		info := NewSpacePersistentInfo("spaceId")
		info.SetAccountStatus(AccountStatusActive).SetAclHeadId("aclHead")
		info.Name = "My space"
		assert.True(t, info.Equal(stored))
	})

	t.Run("equal when only account status set and matches", func(t *testing.T) {
		info := NewSpacePersistentInfo("spaceId")
		info.SetAccountStatus(AccountStatusActive)
		assert.True(t, info.Equal(stored))
	})

	t.Run("not equal when account status differs", func(t *testing.T) {
		info := NewSpacePersistentInfo("spaceId")
		info.SetAccountStatus(AccountStatusDeleted)
		assert.False(t, info.Equal(stored))
	})

	t.Run("not equal when acl head differs", func(t *testing.T) {
		info := NewSpacePersistentInfo("spaceId")
		info.SetAclHeadId("otherHead")
		assert.False(t, info.Equal(stored))
	})

	t.Run("not equal on space id mismatch", func(t *testing.T) {
		info := NewSpacePersistentInfo("otherSpace")
		info.SetAccountStatus(AccountStatusActive)
		assert.False(t, info.Equal(stored))
	})

	t.Run("not equal on nil details", func(t *testing.T) {
		info := NewSpacePersistentInfo("spaceId")
		assert.False(t, info.Equal(nil))
	})
}
