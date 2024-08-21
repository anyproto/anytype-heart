package compatibility

import (
	"testing"

	"github.com/anyproto/any-sync/net/secureservice"
	"github.com/stretchr/testify/assert"
)

func TestCompatibilityChecker_AddPeerVersion(t *testing.T) {
	t.Run("add peer version", func(t *testing.T) {
		// given
		checker := &compatibilityChecker{peersToVersions: map[string]uint32{}}

		// when
		checker.AddPeerVersion("id", 1)

		// then
		assert.Equal(t, uint32(1), checker.peersToVersions["id"])
	})
}

func TestCompatibilityChecker_IsVersionCompatibleWithPeers(t *testing.T) {
	t.Run("no versions", func(t *testing.T) {
		// given
		checker := &compatibilityChecker{peersToVersions: map[string]uint32{}}

		// when
		compatibleWithPeers := checker.IsVersionCompatibleWithPeers()

		// then
		assert.True(t, compatibleWithPeers)
	})
	t.Run("versions compatible", func(t *testing.T) {
		// given
		checker := &compatibilityChecker{peersToVersions: map[string]uint32{}}

		// when
		checker.AddPeerVersion("id", secureservice.ProtoVersion)
		compatibleWithPeers := checker.IsVersionCompatibleWithPeers()

		// then
		assert.True(t, compatibleWithPeers)
	})
	t.Run("versions is not compatible", func(t *testing.T) {
		// given
		checker := &compatibilityChecker{peersToVersions: map[string]uint32{}}

		// when
		checker.AddPeerVersion("id", 1)
		compatibleWithPeers := checker.IsVersionCompatibleWithPeers()

		// then
		assert.False(t, compatibleWithPeers)
	})
}
