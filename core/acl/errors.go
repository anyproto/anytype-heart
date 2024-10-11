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
	ErrNotShareable         = errors.New("space is not shareable")
	ErrLimitReached         = errors.New("limit reached")
	ErrDifferentNetwork     = errors.New("different network")
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
		return wrapError("acl service error", err, space.ErrSpaceNotExists)
	case errors.Is(err, coordinatorproto.ErrSpaceIsDeleted):
		return wrapError("acl service error", err, space.ErrSpaceDeleted)
	case errors.Is(err, coordinatorproto.ErrSpaceLimitReached):
		return wrapError("acl service error", err, ErrLimitReached)
	case errors.Is(err, coordinatorproto.ErrSpaceNotShareable):
		return wrapError("acl service error", err, ErrNotShareable)
	case errors.Is(err, list.ErrNoSuchRecord):
		return wrapError("acl service error", err, ErrRequestNotExists)
	case errors.Is(err, list.ErrNoSuchAccount):
		return wrapError("acl service error", err, ErrNoSuchAccount)
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
