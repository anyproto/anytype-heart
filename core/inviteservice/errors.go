package inviteservice

import (
	"errors"
	"fmt"
)

var (
	ErrInviteNotExists  = errors.New("invite not exists")
	ErrInviteBadContent = errors.New("invite bad content")
	ErrInviteGet        = errors.New("get invite")
	ErrInviteGenerate   = errors.New("generate invite")
	ErrInviteRemove     = errors.New("remove invite")
	ErrPersonalSpace    = errors.New("sharing of personal space is forbidden")
	// ErrInviteNotShareable is returned when an invite anyone can join with is asked to be shared within
	// the space while it grants more than read access. Such a link needs no approval from anyone:
	// whoever holds it is in, with the permissions it carries. A writer's link stays in the owner's
	// account, so that it is theirs alone to hand out.
	ErrInviteNotShareable = errors.New("invite cannot be shared within the space")
)

func removeInviteError(msg string, err error) error {
	return wrapError(msg, err, ErrInviteRemove)
}

func generateInviteError(msg string, err error) error {
	return wrapError(msg, err, ErrInviteGenerate)
}

func getInviteError(msg string, err error) error {
	return wrapError(msg, err, ErrInviteGet)
}

func badContentError(msg string, err error) error {
	return wrapError(msg, err, ErrInviteBadContent)
}

func wrapError(msg string, err, typedErr error) error {
	return fmt.Errorf("%s: %w, %w", msg, err, typedErr)
}
