package anytype

import (
	"context"
	"os"
	"regexp"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/coordinator/nodeconfsource"
	"github.com/anyproto/any-sync/metric"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/debugserver"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/net/secureservice"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/net/transport/quic"
	"github.com/anyproto/any-sync/net/transport/yamux"
	"github.com/anyproto/any-sync/node/nodeclient"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/nodeconfstore"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/any-sync/nameservice/nameserviceclient"
	"github.com/anyproto/any-sync/paymentservice/paymentserviceclient"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/comptester"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/backlinks"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	decorator "github.com/anyproto/anytype-heart/core/block/bookmark/bookmarkimporter"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/export"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/object/objectgraph"
	"github.com/anyproto/anytype-heart/core/block/object/treemanager"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	templateservice "github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/configfetcher"
	"github.com/anyproto/anytype-heart/core/debug"
	"github.com/anyproto/anytype-heart/core/debug/profiler"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/history"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/core/indexer"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/core/invitestore"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/core/payments"
	paymentscache "github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/core/recordsbatcher"
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
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/coordinatorclient"
	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
	"github.com/anyproto/anytype-heart/space/spacecore/credentialprovider"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peermanager"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spacecore/syncstatusprovider"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/space/spacefactory"
	"github.com/anyproto/anytype-heart/space/virtualspaceservice"
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

func BootstrapConfig(newAccount bool, isStaging bool) *config.Config {
	return config.New(
		config.WithDebugAddr(os.Getenv("ANYTYPE_DEBUG_ADDR")),
		config.WithNewAccount(newAccount),
	)
}

func BootstrapWallet(rootPath string, derivationResult crypto.DerivationResult) wallet.Wallet {
	return wallet.NewWithAccountRepo(rootPath, derivationResult)
}

func bootstrapComponents() []app.Component {
	const fileWatcherUpdateInterval = 5 * time.Second

	return []app.Component{
		clientds.New(),
		debugstat.New(),
		ftsearch.New(),
		objectstore.New(),
		backlinks.New(),
		filestore.New(),
		// Services
		nodeconfsource.New(),
		nodeconfstore.New(),
		nodeconf.New(),
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
		quic.New(),
		clientserver.New(),
		streampool.New(),
		coordinatorclient.New(),
		nodeclient.New(),
		credentialprovider.New(),
		commonspace.New(),
		aclclient.NewAclJoiningClient(),
		virtualspaceservice.New(),
		spacecore.New(),
		idresolver.New(),
		localdiscovery.New(),
		peermanager.New(),
		typeprovider.New(),
		fileuploader.New(),
		rpcstore.New(),
		fileservice.New(),
		filestorage.New(),
		files.New(),
		fileacl.New(),
		source.New(),
		spacefactory.New(),
		space.New(),
		deletioncontroller.New(),
		invitestore.New(),
		filesync.New(),
		fileobject.New(200*time.Millisecond, 2*time.Second),
		inviteservice.New(),
		acl.New(),
		builtintemplate.New(),
		converter.NewLayoutConverter(),
		recordsbatcher.New(),
		configfetcher.New(),
		process.New(),
		core.New(),
		core.NewTempDirService(),
		treemanager.New(),
		block.New(),
		indexer.New(),
		syncstatus.New(fileWatcherUpdateInterval),
		history.New(),
		gateway.New(),
		export.New(),
		linkpreview.New(),
		unsplash.New(),
		restriction.New(),
		debug.New(),
		collection.New(),
		subscription.New(),
		builtinobjects.New(),
		bookmark.New(),
		importer.New(),
		decorator.New(),
		objectcreator.NewCreator(),
		kanban.New(),
		editor.NewObjectFactory(),
		objectgraph.NewBuilder(),
		account.New(),
		profiler.New(),
		identity.New(30*time.Second, 10*time.Second),
		templateservice.New(),
		notifications.New(),
		paymentserviceclient.New(),
		nameservice.New(),
		nameserviceclient.New(),
		payments.New(),
		paymentscache.New(),
	}
}

func StartNewApp(ctx context.Context, clientWithVersion string, components ...app.Component) (a *app.App, err error) {
	a = new(app.App)
	complexAppVersion := appVersion(a, clientWithVersion)
	a.SetVersionName(complexAppVersion)
	logging.SetVersion(complexAppVersion)
	components = append(components, bootstrapComponents()...)
	Bootstrap(a, components...)
	metrics.Service.SetAppVersion(a.VersionName())
	metrics.Service.Run()
	startTime := time.Now()
	if err = a.Start(ctx); err != nil {
		metrics.Service.Close()
		a = nil
		return
	}
	totalSpent := time.Since(startTime)
	l := log.With(zap.Int64("total", totalSpent.Milliseconds()))
	stat := a.StartStat()
	event := &metrics.AppStart{
		TotalMs:   stat.SpentMsTotal,
		PerCompMs: stat.SpentMsPerComp,
		Extra:     map[string]interface{}{},
	}

	if v, ok := ctx.Value(metrics.CtxKeyRPC).(string); ok {
		event.Request = v
		l = l.With(zap.String("rpc", v))
	}

	for comp, spent := range stat.SpentMsPerComp {
		if spent == 0 {
			continue
		}
		l = l.With(zap.Int64(comp, spent))
	}

	l.With(zap.Int64("totalRun", stat.SpentMsTotal))
	a.IterateComponents(func(comp app.Component) {
		if c, ok := comp.(ComponentLogFieldsGetter); ok {
			for _, field := range c.GetLogFields() {
				field.Key = comp.Name() + "_" + field.Key
				l = l.With(field)
				if field.String != "" {
					event.Extra[field.Key] = field.String
				} else {
					event.Extra[field.Key] = field.Integer
				}

			}
		}
	})

	if metrics.Enabled {
		metrics.Service.Send(event)
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

func BootstrapTester(a *app.App, mode comptester.TestMode, failAt int, components ...app.Component) {
	tester := comptester.New(comptester.TestModeFailOnInit, 0)
	for _, c := range components {
		a.Register(comptester.New(c))
	}
}

func Bootstrap(a *app.App, components ...app.Component) {
	for _, c := range components {
		a.Register(c)
	}
}

func MiddlewareVersion() string {
	return vcs.GetVCSInfo().Version()
}

type ComponentLogFieldsGetter interface {
	// GetLogFields returns additional useful fields for logs to debug long app start/stop duration or something else in the future
	// You don't need to provide the component name in the field's Key, because it will be added automatically
	GetLogFields() []zap.Field
}
