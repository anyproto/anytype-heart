package anytype

import (
	"context"
	"os"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/coordinator/coordinatorclient"
	"github.com/anytypeio/any-sync/coordinator/nodeconfsource"
	"github.com/anytypeio/any-sync/net/dialer"
	"github.com/anytypeio/any-sync/net/pool"
	"github.com/anytypeio/any-sync/net/secureservice"
	"github.com/anytypeio/any-sync/net/streampool"
	"github.com/anytypeio/any-sync/nodeconf"
	"github.com/anytypeio/any-sync/nodeconf/nodeconfstore"
	"github.com/anytypeio/any-sync/util/crypto"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	decorator "github.com/anytypeio/go-anytype-middleware/core/block/bookmark/bookmarkimporter"
	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/export"
	importer "github.com/anytypeio/go-anytype-middleware/core/block/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/object/objectcreator"
	"github.com/anytypeio/go-anytype-middleware/core/block/object/objectgraph"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/configfetcher"
	"github.com/anytypeio/go-anytype-middleware/core/debug"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync/filesyncstatus"
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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/clientserver"
	"github.com/anytypeio/go-anytype-middleware/space/credentialprovider"
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
)

func BootstrapConfig(newAccount bool, isStaging bool, createBuiltinObjects, createBuiltinTemplates bool) *config.Config {
	return config.New(
		config.WithStagingCafe(isStaging),
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
	indexerService := indexer.New(blockService, spaceService)
	relationService := relation.New()
	coreService := core.New()
	graphRenderer := objectgraph.NewBuilder(sbtProvider, relationService, objectStore, coreService)
	fileSyncService := filesync.New()

	fileSyncUpdateInterval := 5 * time.Second
	fileSyncStatusRegistry := filesyncstatus.NewRegistry(fileSyncService, fileSyncUpdateInterval)

	linkedFilesStatusWatcher := status.NewLinkedFilesWatcher(spaceService, fileSyncStatusRegistry)
	subObjectsStatusWatcher := status.NewSubObjectsWatcher()
	statusUpdateReceiver := status.NewUpdateReceiver(coreService, linkedFilesStatusWatcher, subObjectsStatusWatcher, cfg, eventService.Send)
	objectStatusWatcher := status.NewSpaceObjectWatcher(spaceService, statusUpdateReceiver)

	fileSyncStatusWatcher := filesyncstatus.New(fileSyncStatusRegistry, statusUpdateReceiver, fileSyncUpdateInterval)
	fileStatusWatcher := status.NewFileWatcher(spaceService, fileSyncStatusWatcher)

	statusService := status.New(sbtProvider, coreService, fileStatusWatcher, objectStatusWatcher, subObjectsStatusWatcher, linkedFilesStatusWatcher)

	a.Register(clientds.New()).
		Register(nodeconfsource.New()).
		Register(nodeconfstore.New()).
		Register(nodeconf.New()).
		Register(peerstore.New()).
		Register(storage.New()).
		Register(secureservice.New()).
		Register(dialer.New()).
		Register(pool.New()).
		Register(streampool.New()).
		Register(clientserver.New()).
		Register(coordinatorclient.New()).
		Register(credentialprovider.New()).
		Register(commonspace.New()).
		Register(rpcstore.New()).
		Register(filestore.New()).
		Register(fileservice.New()).
		Register(filestorage.New()).
		Register(fileSyncService).
		Register(localdiscovery.New()).
		Register(spaceService).
		Register(peermanager.New()).
		Register(sbtProvider).
		Register(relationService).
		Register(ftsearch.New()).
		Register(objectStore).
		Register(recordsbatcher.New()).
		Register(files.New()).
		Register(cafe.New()).
		Register(configfetcher.New()).
		Register(process.New()).
		Register(source.New()).
		Register(coreService).
		Register(builtintemplate.New()).
		Register(blockService).
		Register(indexerService).
		Register(fileSyncStatusWatcher).
		Register(linkedFilesStatusWatcher).
		Register(statusService).
		Register(history.New()).
		Register(gateway.New()).
		Register(export.New(sbtProvider)).
		Register(linkpreview.New()).
		Register(unsplash.New(tempDirService)).
		Register(restriction.New(sbtProvider)).
		Register(debug.New()).
		Register(clientdebugrpc.New()).
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
