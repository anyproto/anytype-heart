package idresolver

import (
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
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
	objectStore objectstore.ObjectStore
	sync.Mutex
}

func (r *resolver) Init(a *app.App) (err error) {
	r.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return
}

func (r *resolver) Name() (name string) {
	return CName
}

func (r *resolver) ResolveSpaceID(objectID string) (string, error) {
	return r.objectStore.GetSpaceId(objectID)
}
