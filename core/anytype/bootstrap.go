package anytype

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/export"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/recordsbatcher"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/ipfslite"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/builtintemplate"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
)

func DefaultClientComponents(newAccount bool, rootPath, accountId string) ([]app.Component, error) {
	return []app.Component{
		config.New(func(c *config.Config) {
			c.NewAccount = newAccount
		}),

		wallet.NewWithAccountRepo(rootPath, accountId),
		clientds.New(),
		ftsearch.New(),
		localstore.New(),
		recordsbatcher.New(),
		indexer.New(),
		ipfslite.New(),
		files.New(),
		cafe.New(),
		threads.New(),
		core.New(),
		pin.New(),
	}, nil
}

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
	a.Register(status.New()).
		Register(meta.New()).
		Register(block.New()).
		Register(process.New()).
		Register(history.New()).
		Register(gateway.New()).
		Register(export.New()).
		Register(builtintemplate.New()).
		Register(linkpreview.New())
	return
}
