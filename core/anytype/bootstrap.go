package anytype

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/export"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

func StartNewApp(components ...app.Component) (a *app.App, err error) {
	a = new(app.App)
	Bootstrap(a, components...)
	if err = a.Start(); err != nil {
		return
	}
	return
}

func Bootstrap(a *app.App, components ...app.Component) {
	for _, c := range components {
		a.Register(c)
	}
	a.Register(indexer.New()).
		Register(status.New()).
		Register(meta.New()).
		Register(block.New()).
		Register(process.New()).
		Register(history.New()).
		Register(gateway.New()).
		Register(export.New()).
		Register(linkpreview.New())
	return
}
