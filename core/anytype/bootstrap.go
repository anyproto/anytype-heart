package anytype

import (
	"context"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/net/dialer"
	"github.com/anytypeio/any-sync/net/pool"
	"github.com/anytypeio/any-sync/net/secureservice"
	"github.com/anytypeio/any-sync/net/streampool"
	"github.com/anytypeio/any-sync/nodeconf"
	"github.com/anytypeio/go-anytype-middleware/core/account"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	decorator "github.com/anytypeio/go-anytype-middleware/core/block/bookmark/bookmarkimporter"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/export"
	importer "github.com/anytypeio/go-anytype-middleware/core/block/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/object"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/core/debug"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"github.com/anytypeio/go-anytype-middleware/core/kanban"
	"github.com/anytypeio/go-anytype-middleware/core/recordsbatcher"
	"github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/core/status"
	"github.com/anytypeio/go-anytype-middleware/core/subscription"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore/clientds"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/profilefinder"
	walletUtil "github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/clientserver"
	"github.com/anytypeio/go-anytype-middleware/space/debug/clientdebugrpc"
	"github.com/anytypeio/go-anytype-middleware/space/localdiscovery"
	"github.com/anytypeio/go-anytype-middleware/space/peermanager"
	"github.com/anytypeio/go-anytype-middleware/space/peerstore"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/builtinobjects"
	"github.com/anytypeio/go-anytype-middleware/util/builtintemplate"
	"github.com/anytypeio/go-anytype-middleware/util/linkpreview"
	"github.com/anytypeio/go-anytype-middleware/util/unsplash"
	"os"
)

func StartAccountRecoverApp(ctx context.Context, eventSender event.Sender, accountPrivKey walletUtil.Keypair) (a *app.App, err error) {
	a = new(app.App)
	device, err := walletUtil.NewRandomKeypair(walletUtil.KeypairTypeDevice)
	if err != nil {
		return nil, err
	}

	a.Register(wallet.NewWithRepoPathAndKeys("", accountPrivKey, device)).
		Register(config.New(
			config.WithStagingCafe(os.Getenv("ANYTYPE_STAGING") == "1"),
			config.DisableFileConfig(true), // do not load/save config to file because we don't have a libp2p node and repo in this mode
		),
		).
		Register(cafe.New()).
		Register(profilefinder.New()).
		Register(eventSender)

	if err = a.Start(ctx); err != nil {
		return
	}

	return a, nil
}

func BootstrapConfig(newAccount bool, isStaging bool) *config.Config {
	return config.New(
		config.WithStagingCafe(isStaging),
		config.WithDebugAddr(os.Getenv("ANYTYPE_DEBUG_ADDR")),
		config.WithNewAccount(newAccount),
	)
}

func BootstrapWallet(rootPath, accountId string) wallet.Wallet {
	return wallet.NewWithAccountRepo(rootPath, accountId)
}

func StartNewApp(ctx context.Context, components ...app.Component) (a *app.App, err error) {
	a = new(app.App)
	Bootstrap(a, components...)
	metrics.SharedClient.SetAppVersion(a.Version())
	metrics.SharedClient.Run()
	if err = a.Start(ctx); err != nil {
		metrics.SharedClient.Close()
		a = nil
		return
	}

	return
}

func Bootstrap(a *app.App, components ...app.Component) {
	for _, c := range components {
		a.Register(c)
	}
	a.Register(clientds.New()).
		Register(nodeconf.New()).
		Register(peerstore.New()).
		Register(storage.New()).
		Register(secureservice.New()).
		Register(dialer.New()).
		Register(pool.New()).
		Register(streampool.New()).
		Register(clientserver.New()).
		Register(commonspace.New()).
		Register(rpcstore.New()).
		Register(fileservice.New()).
		Register(filestorage.New()).
		Register(localdiscovery.New()).
		Register(space.New()).
		Register(peermanager.New()).
		Register(typeprovider.New()).
		Register(relation.New()).
		Register(ftsearch.New()).
		Register(objectstore.New()).
		Register(filestore.New()).
		Register(recordsbatcher.New()).
		Register(files.New()).
		Register(cafe.New()).
		Register(account.New()).
		Register(configfetcher.New()).
		Register(process.New()).
		Register(source.New()).
		Register(core.New()).
		Register(builtintemplate.New()).
		Register(status.New()).
		Register(block.New()).
		Register(doc.New()).
		Register(indexer.New()).
		Register(history.New()).
		Register(gateway.New()).
		Register(export.New()).
		Register(linkpreview.New()).
		Register(unsplash.New()).
		Register(restriction.New()).
		Register(debug.New()).
		Register(clientdebugrpc.New()).
		Register(subscription.New()).
		Register(builtinobjects.New()).
		Register(bookmark.New()).
		Register(session.New()).
		Register(importer.New()).
		Register(decorator.New()).
		Register(object.NewCreator()).
		Register(kanban.New()).
		Register(editor.NewObjectFactory())
	return
}
