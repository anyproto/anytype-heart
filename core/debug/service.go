package debug

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
)

const CName = "debug"

func New() Debug {
	return new(debug)
}

type Debug interface {
	app.Component
	DumpTree(blockId, path string) (filename string, err error)
}

type debug struct {
	core  core.Service
	store objectstore.ObjectStore
}

func (d *debug) Init(a *app.App) (err error) {
	d.core = a.MustComponent(core.CName).(core.Service)
	d.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	return nil
}

func (d *debug) Name() (name string) {
	return CName
}

func (d *debug) DumpTree(blockId, path string) (filename string, err error) {
	block, err := d.core.GetBlock(blockId)
	if err != nil {
		return
	}
	builder := &treeBuilder{b: block, s: d.store}
	return builder.Build(path)
}
