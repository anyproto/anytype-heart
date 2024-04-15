package acl

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"

	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/space"
)

var (
	ErrRequestNotExists     = errors.New("request doesn't exist")
	ErrPersonalSpace        = errors.New("sharing of personal space is forbidden")
	ErrIncorrectPermissions = errors.New("incorrect permissions")
	ErrNoSuchAccount        = errors.New("no such user")
	ErrAclRequestFailed     = errors.New("acl request failed")
	ErrLimitReached         = errors.New("limit reached")
	ErrInternal             = errors.New("internal error")
)

var passthroughErrors = []error{
	space.ErrSpaceStorageMissig,
	space.ErrSpaceDeleted,
	inviteservice.ErrInviteGet,
	inviteservice.ErrInviteGenerate,
	inviteservice.ErrInviteRemove,
	inviteservice.ErrInviteBadContent,
}

func convertErrorOrReturn(err, otherErr error) error {
	if err == nil {
		return nil
	}
	for _, passthroughErr := range passthroughErrors {
		if errors.Is(err, passthroughErr) {
			return err
		}
	}
	switch {
	case errors.Is(err, coordinatorproto.ErrSpaceNotExists):
		return space.ErrSpaceNotExists
	case errors.Is(err, coordinatorproto.ErrSpaceIsDeleted):
		return space.ErrSpaceDeleted
	case errors.Is(err, coordinatorproto.ErrSpaceLimitReached):
		return ErrLimitReached
	case errors.Is(err, list.ErrNoSuchRecord):
		return ErrRequestNotExists
	case errors.Is(err, list.ErrNoSuchAccount):
		return ErrNoSuchAccount
	default:
		return otherErr
	}
}

func wrapError(msg string, err, typedErr error) error {
	return fmt.Errorf("%s: %w, %w", msg, err, typedErr)
}

func wrapAclErr(err error) error {
	return fmt.Errorf("%w: %w", ErrAclRequestFailed, err)
}

func convertedOrSpaceErr(err error) error {
	return convertedOrInternalError("failed to get space", err)
}

func convertedOrInternalError(msg string, err error) error {
	return convertErrorOrReturn(err, wrapError(msg, err, ErrInternal))
}

func convertedOrAclRequestError(err error) error {
	return convertErrorOrReturn(err, wrapAclErr(err))
}
