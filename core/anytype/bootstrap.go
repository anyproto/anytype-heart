package anytype

import (
	"context"
	"os"
	"regexp"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/nodeconfsource"
	"github.com/anyproto/any-sync/metric"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/debugserver"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/net/secureservice"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/nodeconfstore"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"go.uber.org/zap"

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
	"github.com/anyproto/anytype-heart/space/clientserver"
	"github.com/anyproto/anytype-heart/space/credentialprovider"
	"github.com/anyproto/anytype-heart/space/localdiscovery"
	"github.com/anyproto/anytype-heart/space/peermanager"
	"github.com/anyproto/anytype-heart/space/peerstore"
	"github.com/anyproto/anytype-heart/space/storage"
	"github.com/anyproto/anytype-heart/space/syncstatusprovider"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/builtintemplate"
	"github.com/anyproto/anytype-heart/util/linkpreview"
	"github.com/anyproto/anytype-heart/util/unsplash"
	"github.com/anyproto/anytype-heart/util/vcs"
)

var (
	log          = logging.LoggerNotSugared("anytype-app")
	WarningAfter = time.Second * 1
)

func BootstrapConfig(newAccount bool, isStaging bool, createBuiltinTemplates bool) *config.Config {
	return config.New(
		config.WithDebugAddr(os.Getenv("ANYTYPE_DEBUG_ADDR")),
		config.WithNewAccount(newAccount),
		config.WithCreateBuiltinTemplates(createBuiltinTemplates),
	)
}

func BootstrapWallet(rootPath string, derivationResult crypto.DerivationResult) wallet.Wallet {
	return wallet.NewWithAccountRepo(rootPath, derivationResult)
}

func StartNewApp(ctx context.Context, clientWithVersion string, components ...app.Component) (a *app.App, err error) {
	a = new(app.App)
	a.SetVersionName(appVersion(a, clientWithVersion))
	components = Bootstrap(a, components...)
	metrics.SharedClient.SetAppVersion(a.Version())
	metrics.SharedClient.Run()
	startTime := time.Now()
	if err = a.Start(ctx); err != nil {
		metrics.SharedClient.Close()
		a = nil
		return
	}
	totalSpent := time.Since(startTime)
	l := log.With(zap.Int64("total", totalSpent.Milliseconds()))
	if v, ok := ctx.Value(metrics.CtxKeyRPC).(string); ok {
		l = l.With(zap.String("rpc", v))
	}

	for comp, spent := range a.StartStat().SpentMsPerComp {
		if spent == 0 {
			continue
		}
		l = l.With(zap.Int64(comp, spent))
	}
	l.With(zap.Int64("totalRun", a.StartStat().SpentMsTotal))
	for _, comp := range components {
		if c, ok := comp.(ComponentLogFieldsGetter); ok {
			for _, field := range c.GetLogFields() {
				field.Key = comp.Name() + "_" + field.Key
				l = l.With(field)
			}
		}
	}
	if totalSpent > WarningAfter {
		l.Warn("app started")
	} else {
		l.Debug("app started")
	}
	return
}

func appVersion(a *app.App, clientWithVersion string) string {
	clientWithVersion = regexp.MustCompile(`(@|\/)+`).ReplaceAllString(clientWithVersion, "_")
	middleVersion := MiddlewareVersion()
	anySyncVersion := a.AnySyncVersion()
	return clientWithVersion + "/middle:" + middleVersion + "/any-sync:" + anySyncVersion
}

func Bootstrap(a *app.App, components ...app.Component) []app.Component {
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
	nodeConf := nodeconf.New()

	const fileWatcherUpdateInterval = 5 * time.Second
	syncStatusService := syncstatus.New(
		sbtProvider,
		datastoreProvider,
		spaceService,
		coreService,
		fileSyncService,
		nodeConf,
		fileStore,
		blockService,
		cfg,
		eventService.Send,
		fileWatcherUpdateInterval,
	)
	fileSyncService.OnUpload(syncStatusService.OnFileUpload)

	fileService := files.New(syncStatusService, objectStore)

	indexerService := indexer.New(blockService, spaceService, fileService)

	// we already registred some required components
	skipRegister := len(components)

	components = append(components, []app.Component{
		datastoreProvider,
		nodeconfsource.New(),
		nodeconfstore.New(),
		nodeConf,
		peerstore.New(),
		syncstatusprovider.New(),
		storage.New(),
		secureservice.New(),
		metric.New(),
		server.New(),
		debugserver.New(),
		pool.New(),
		peerservice.New(),
		yamux.New(),
		clientserver.New(),
		streampool.New(),
		coordinatorclient.New(),
		credentialprovider.New(),
		commonspace.New(),
		rpcstore.New(),
		fileStore,
		fileservice.New(),
		filestorage.New(eventService.Send),
		fileSyncService,
		localdiscovery.New(),
		spaceService,
		peermanager.New(),
		sbtProvider,
		relationService,
		ftsearch.New(),
		objectStore,
		recordsbatcher.New(),
		fileService,
		configfetcher.New(),
		process.New(),
		source.New(),
		coreService,
		builtintemplate.New(),
		blockService,
		indexerService,
		syncStatusService,
		history.New(),
		gateway.New(),
		export.New(sbtProvider),
		linkpreview.New(),
		unsplash.New(tempDirService),
		restriction.New(sbtProvider, objectStore),
		debug.New(),
		collectionService,
		subscription.New(collectionService, sbtProvider),
		builtinobjects.New(tempDirService),
		bookmark.New(tempDirService),
		session.New(),
		importer.New(tempDirService, sbtProvider),
		decorator.New(),
		objectCreator,
		kanban.New(),
		editor.NewObjectFactory(tempDirService, sbtProvider, layoutConverter),
		graphRenderer}...)

	for _, component := range components[skipRegister:] {
		a.Register(component)
	}
	return components
}

func MiddlewareVersion() string {
	return vcs.GetVCSInfo().Version()
}

type ComponentLogFieldsGetter interface {
	// GetLogFields returns additional useful fields for logs to debug long app start/stop duration or something else in the future
	// You don't need to provide the component name in the field's Key, because it will be added automatically
	GetLogFields() []zap.Field
}
