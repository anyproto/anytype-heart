package anytype

import (
	"context"
	"github.com/anyproto/any-sync/metric"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"github.com/anyproto/anytype-heart/space/syncstatusprovider"
	"os"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/nodeconfsource"
	"github.com/anyproto/any-sync/net/secureservice"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/nodeconfstore"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	decorator "github.com/anyproto/anytype-heart/core/block/bookmark/bookmarkimporter"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/export"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/object/objectgraph"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/core/debug"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/history"
	"github.com/anyproto/anytype-heart/core/indexer"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/core/recordsbatcher"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/credentialprovider"
	"github.com/anyproto/anytype-heart/space/localdiscovery"
	"github.com/anyproto/anytype-heart/space/peermanager"
	"github.com/anyproto/anytype-heart/space/peerstore"
	"github.com/anyproto/anytype-heart/space/storage"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/builtintemplate"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/unsplash"
)

func BootstrapConfig(newAccount bool, isStaging bool, createBuiltinObjects, createBuiltinTemplates bool) *config.Config {
	return config.New(
		config.WithDebugAddr(os.Getenv("ANYTYPE_DEBUG_ADDR")),
		config.WithNewAccount(newAccount),
		config.WithCreateBuiltinObjects(createBuiltinObjects),
		config.WithCreateBuiltinTemplates(createBuiltinTemplates),
	)
}

func BootstrapWallet(rootPath string, derivationResult crypto.DerivationResult) wallet.Wallet {
	return wallet.NewWithAccountRepo(rootPath, derivationResult)
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
	walletService := a.Component(wallet.CName).(wallet.Wallet)
	eventService := a.Component(event.CName).(event.Sender)
	cfg := a.Component(config.CName).(*config.Config)

	tempDirService := core.NewTempDirService(walletService)
	spaceService := space.New()
	sbtProvider := typeprovider.New(spaceService)
	objectStore := objectstore.New(sbtProvider)
	objectCreator := objectcreator.NewCreator(sbtProvider)
	layoutConverter := converter.NewLayoutConverter(objectStore, sbtProvider)
	blockService := block.New(tempDirService, sbtProvider, layoutConverter)
	collectionService := collection.New(blockService, objectStore, objectCreator, blockService)
	relationService := relation.New()
	coreService := core.New()
	graphRenderer := objectgraph.NewBuilder(sbtProvider, relationService, objectStore, coreService)
	fileSyncService := filesync.New(eventService.Send)
	fileStore := filestore.New()

	datastoreProvider := clientds.New()

	const fileWatcherUpdateInterval = 5 * time.Second
	syncStatusService := syncstatus.New(
		sbtProvider,
		datastoreProvider,
		spaceService,
		coreService,
		fileSyncService,
		fileStore,
		blockService,
		cfg,
		eventService.Send,
		fileWatcherUpdateInterval,
	)

	fileService := files.New(syncStatusService, objectStore)

	indexerService := indexer.New(blockService, spaceService, fileService)

	a.Register(datastoreProvider).
		Register(nodeconfsource.New()).
		Register(nodeconfstore.New()).
		Register(nodeconf.New()).
		Register(peerstore.New()).
		Register(syncstatusprovider.New()).
		Register(storage.New()).
		Register(secureservice.New()).
		Register(metric.New()).
		Register(server.New()).
		Register(pool.New()).
		Register(peerservice.New()).
		Register(yamux.New()).
		Register(streampool.New()).
		Register(coordinatorclient.New()).
		Register(credentialprovider.New()).
		Register(commonspace.New()).
		Register(rpcstore.New()).
		Register(fileStore).
		Register(fileservice.New()).
		Register(filestorage.New(eventService.Send)).
		Register(fileSyncService).
		Register(localdiscovery.New()).
		Register(spaceService).
		Register(peermanager.New()).
		Register(sbtProvider).
		Register(relationService).
		Register(ftsearch.New()).
		Register(objectStore).
		Register(recordsbatcher.New()).
		Register(fileService).
		Register(configfetcher.New()).
		Register(process.New()).
		Register(source.New()).
		Register(coreService).
		Register(builtintemplate.New()).
		Register(blockService).
		Register(indexerService).
		Register(syncStatusService).
		Register(history.New()).
		Register(gateway.New()).
		Register(export.New(sbtProvider)).
		Register(linkpreview.New()).
		Register(unsplash.New(tempDirService)).
		Register(restriction.New(sbtProvider, objectStore)).
		Register(debug.New()).
		Register(collectionService).
		Register(subscription.New(collectionService, sbtProvider)).
		Register(builtinobjects.New(sbtProvider)).
		Register(bookmark.New(tempDirService)).
		Register(session.New()).
		Register(importer.New(tempDirService, sbtProvider)).
		Register(decorator.New()).
		Register(objectCreator).
		Register(kanban.New()).
		Register(editor.NewObjectFactory(tempDirService, sbtProvider, layoutConverter)).
		Register(graphRenderer)
	return
}
