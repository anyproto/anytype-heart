package virtualspaceservice

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

const CName = "common.space.virtualspaceservice"

type VirtualSpaceService interface {
	app.ComponentRunnable
	RegisterVirtualSpace(spaceID string) (err error)
}

type virtualSpaceService struct {
	objectStore objectstore.ObjectStore
}

func (v *virtualSpaceService) Init(a *app.App) (err error) {
	v.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	return nil
}

func (v *virtualSpaceService) Name() string {
	return CName
}

func (v *virtualSpaceService) Run(ctx context.Context) (err error) {
	return v.cleanupVirtualSpaces(err)
}

func (v *virtualSpaceService) Close(ctx context.Context) (err error) {
	return v.cleanupVirtualSpaces(err)
}

func (v *virtualSpaceService) cleanupVirtualSpaces(err error) error {
	spaces, err := v.objectStore.ListVirtualSpaces()
	if err != nil {
		return err
	}
	for _, id := range spaces {
		err := v.objectStore.DeleteVirtualSpace(id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (v *virtualSpaceService) RegisterVirtualSpace(spaceID string) (err error) {
	return v.objectStore.SaveVirtualSpace(spaceID)
}

func New() VirtualSpaceService {
	return &virtualSpaceService{}
}
