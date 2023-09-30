package space

import (
	"context"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

type (
	LocalStatus  struct{}
	RemoteStatus struct{}
)

type SpaceStatus struct {
	Local  LocalStatus
	Remote RemoteStatus
}

type MandatoryIds struct {
}

type Space interface {
	Id() string
	GetStatus() (SpaceStatus, error)
	MandatoryIds() (MandatoryIds, error)
	CreateObject(ctx context.Context, id string) (editorsb.SmartBlock, error)
	DeleteObject(ctx context.Context, id string) error
	GetObject(ctx context.Context, id string) (editorsb.SmartBlock, error)
}

// space view -> space Id
// (...)
//
// logic space status -> AclInvite loop || Load loop
//

// SpaceStatusManager -> ...
