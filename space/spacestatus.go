package space

import (
	"errors"
	"time"

	"github.com/anytypeio/any-sync/coordinator/coordinatorproto"
)

type Status int32

var (
	ErrSpaceDeleteUnexpected = errors.New("unexpected error while deleting space")
	ErrSpaceIsDeleted        = errors.New("space is deleted")
	ErrSpaceIsCreated        = errors.New("space is created")
	ErrSpaceDeletionPending  = errors.New("space deletion is pending")
)

const (
	SpaceStatusCreated Status = iota
	SpaceStatusPendingDeletion
	SpaceStatusDeletionStarted
	SpaceStatusDeleted
)

type StatusPayload struct {
	Status       Status
	DeletionDate time.Time
}

func newSpaceStatus(payload *coordinatorproto.SpaceStatusPayload) StatusPayload {
	return StatusPayload{
		Status:       Status(payload.Status),
		DeletionDate: time.Unix(payload.DeletionTimestamp, 0),
	}
}

func coordError(err error) error {
	switch err {
	case coordinatorproto.ErrSpaceDeletionPending:
		return ErrSpaceDeletionPending
	case coordinatorproto.ErrSpaceIsDeleted:
		return ErrSpaceIsDeleted
	case coordinatorproto.ErrSpaceIsCreated:
		return ErrSpaceIsCreated
	default:
		return ErrSpaceDeleteUnexpected
	}
}
