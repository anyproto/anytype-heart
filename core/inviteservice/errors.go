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
