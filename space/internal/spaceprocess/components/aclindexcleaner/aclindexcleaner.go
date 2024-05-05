package aclindexcleaner

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
)

const CName = "client.components.aclindexcleaner"

type AclIndexCleaner interface {
	app.ComponentRunnable
}

func New() AclIndexCleaner {
	return &aclIndexCleaner{}
}

type aclIndexCleaner struct {
	indexer dependencies.SpaceIndexer
	status  spacestatus.SpaceStatus
}

func (a *aclIndexCleaner) Init(ap *app.App) (err error) {
	a.indexer = app.MustComponent[dependencies.SpaceIndexer](ap)
	a.status = app.MustComponent[spacestatus.SpaceStatus](ap)
	return nil
}

func (a *aclIndexCleaner) Name() (name string) {
	return CName
}

func (a *aclIndexCleaner) Run(ctx context.Context) (err error) {
	return a.indexer.RemoveAclIndexes(a.status.SpaceId())
}

func (a *aclIndexCleaner) Close(ctx context.Context) (err error) {
	return nil
}
