package acl

import (
	"errors"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/space"
)

func TestConvertError(t *testing.T) {
	t.Run("passthrough errors", func(t *testing.T) {
		var passthroughErrors = []error{
			space.ErrSpaceStorageMissig,
			space.ErrSpaceDeleted,
			inviteservice.ErrInviteGet,
			inviteservice.ErrInviteGenerate,
			inviteservice.ErrInviteRemove,
			inviteservice.ErrInviteBadContent,
		}
		for _, err := range passthroughErrors {
			newErr := convertErrorOrReturn(err, ErrInternal)
			require.Equal(t, err, newErr)
		}
	})
	t.Run("nil error", func(t *testing.T) {
		err := convertErrorOrReturn(nil, ErrInternal)
		require.NoError(t, err)
	})
	t.Run("other error", func(t *testing.T) {
		err := convertErrorOrReturn(fmt.Errorf("test"), ErrInternal)
		require.True(t, errors.Is(err, ErrInternal))
	})
	t.Run("specific error", func(t *testing.T) {
		err := coordinatorproto.ErrSpaceIsDeleted
		err = convertErrorOrReturn(err, ErrInternal)
		require.Equal(t, space.ErrSpaceDeleted, err)

		err = coordinatorproto.ErrSpaceNotExists
		err = convertErrorOrReturn(err, ErrInternal)
		require.Equal(t, space.ErrSpaceNotExists, err)

		err = coordinatorproto.ErrSpaceNotShareable
		err = convertErrorOrReturn(err, ErrInternal)
		require.Equal(t, ErrNotShareable, err)

		err = coordinatorproto.ErrSpaceLimitReached
		err = convertErrorOrReturn(err, ErrInternal)
		require.Equal(t, ErrLimitReached, err)

		err = list.ErrNoSuchRecord
		err = convertErrorOrReturn(err, ErrInternal)
		require.Equal(t, ErrRequestNotExists, err)

		err = list.ErrNoSuchAccount
		err = convertErrorOrReturn(err, ErrInternal)
		require.Equal(t, ErrNoSuchAccount, err)
	})
	t.Run("other error for converted", func(t *testing.T) {
		err := fmt.Errorf("test")
		newErr := convertedOrInternalError("error", err)
		require.True(t, errors.Is(newErr, ErrInternal))

		newErr = convertedOrAclRequestError(err)
		require.True(t, errors.Is(newErr, ErrAclRequestFailed))
	})
}

func TestWrapError(t *testing.T) {
	t.Run("wrap error", func(t *testing.T) {
		err := fmt.Errorf("test")
		newErr := wrapError("msg", err, ErrRequestNotExists)
		require.True(t, errors.Is(newErr, ErrRequestNotExists))
	})
	t.Run("wrap acl error", func(t *testing.T) {
		err := fmt.Errorf("test")
		newErr := wrapAclErr(err)
		require.True(t, errors.Is(newErr, ErrAclRequestFailed))
	})
}
