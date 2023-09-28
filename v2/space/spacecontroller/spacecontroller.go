package spacecontroller

import (
	"context"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

type (
	DeletionParams struct{}
	LocalStatus    struct{}
	RemoteStatus   struct{}
)

type SpaceStatus struct {
	Local  LocalStatus
	Remote RemoteStatus
}

type MandatoryIds struct {
}

type SpaceController interface {
	Id() string
	Delete(ctx context.Context, params DeletionParams) error
	RevertDeletion(ctx context.Context) error
	WaitLoad(ctx context.Context) error
	GetStatus() (SpaceStatus, error)
	MandatoryIds() (MandatoryIds, error)
	CreateObject(ctx context.Context, id string) (editorsb.SmartBlock, error)
	DeleteObject(ctx context.Context, id string) error
	GetObject(ctx context.Context, id string) (editorsb.SmartBlock, error)
}
