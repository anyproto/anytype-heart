package idresolver

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

const CName = "block.object.resolver"

type Resolver interface {
	app.Component
	ResolveSpaceID(objectID string) (string, error)
}

func New() Resolver {
	return &resolver{}
}

type resolver struct {
	storage storage.ClientStorage
	sync.Mutex
}

func (r *resolver) Init(a *app.App) (err error) {
	r.storage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	return
}

func (r *resolver) Name() (name string) {
	return CName
}

func (r *resolver) ResolveSpaceID(objectID string) (string, error) {
	// TODO: [MR] remove it, because maybe we don't need it
	if addr.IsBundledId(objectID) {
		return addr.AnytypeMarketplaceWorkspace, nil
	}
	return r.storage.GetSpaceID(objectID)
}
