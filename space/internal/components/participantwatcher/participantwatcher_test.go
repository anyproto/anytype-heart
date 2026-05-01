package participantwatcher

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestConvertPermissions_Admin(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Admin, convertPermissions(list.AclPermissionsAdmin))
}

func TestConvertPermissions_Owner(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Owner, convertPermissions(list.AclPermissionsOwner))
}

func TestConvertPermissions_Writer(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Writer, convertPermissions(list.AclPermissionsWriter))
}

func TestConvertPermissions_Reader(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_Reader, convertPermissions(list.AclPermissionsReader))
}

func TestConvertPermissions_None(t *testing.T) {
	require.Equal(t, model.ParticipantPermissions_NoPermissions, convertPermissions(list.AclPermissionsNone))
}
