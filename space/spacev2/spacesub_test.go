package spacev2

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func TestStatusToInfo(t *testing.T) {
	// given
	status := spaceViewStatus{
		spaceId:       "space1",
		aclHeadId:     "aclHead1",
		guestKey:      "guestKey1",
		accountStatus: spaceinfo.AccountStatusJoining,
	}
	want := spaceinfo.NewSpacePersistentInfo("space1")
	want.SetAccountStatus(spaceinfo.AccountStatusJoining).
		SetAclHeadId("aclHead1").
		SetEncodedKey("guestKey1")

	// when
	got := statusToInfo(status)

	// then
	assert.Equal(t, want, got)
}
