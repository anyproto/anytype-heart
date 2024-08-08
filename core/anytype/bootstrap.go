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
	"github.com/anyproto/anytype-heart/core/device"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileacl"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/files/reconciler"
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
	"github.com/anyproto/anytype-heart/core/peerstatus"
	"github.com/anyproto/anytype-heart/core/recordsbatcher"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
	"github.com/anyproto/anytype-heart/pkg/lib/gateway"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/oldstore"
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

func StartNewApp(ctx context.Context, clientWithVersion string, components ...app.Component) (a *app.App, err error) {
	a = new(app.App)
	complexAppVersion := appVersion(a, clientWithVersion)
	a.SetVersionName(complexAppVersion)
	logging.SetVersion(complexAppVersion)
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

func Bootstrap(a *app.App, components ...app.Component) {
	for _, c := range components {
		a.Register(c)
	}

	a.
		// Data storages
		Register(clientds.New()).
		Register(debugstat.New()).
		// Register(ftsearch.BleveNew()).
		Register(ftsearch.TantivyNew()).
		Register(oldstore.New()).
		Register(objectstore.New()).
		Register(backlinks.New()).
		Register(filestore.New()).
		// Services
		Register(nodeconfsource.New()).
		Register(nodeconfstore.New()).
		Register(nodeconf.New()).
		Register(peerstore.New()).
		Register(storage.New()).
		Register(secureservice.New()).
		Register(metric.New()).
		Register(server.New()).
		Register(debugserver.New()).
		Register(pool.New()).
		Register(peerservice.New()).
		Register(yamux.New()).
		Register(quic.New()).
		Register(clientserver.New()).
		Register(streampool.New()).
		Register(coordinatorclient.New()).
		Register(nodeclient.New()).
		Register(credentialprovider.New()).
		Register(commonspace.New()).
		Register(aclclient.NewAclJoiningClient()).
		Register(virtualspaceservice.New()).
		Register(spacecore.New()).
		Register(idresolver.New()).
		Register(device.New()).
		Register(localdiscovery.New()).
		Register(peermanager.New()).
		Register(typeprovider.New()).
		Register(fileuploader.New()).
		Register(rpcstore.New()).
		Register(fileservice.New()).
		Register(filestorage.New()).
		Register(files.New()).
		Register(fileoffloader.New()).
		Register(fileacl.New()).
		Register(source.New()).
		Register(spacefactory.New()).
		Register(space.New()).
		Register(deletioncontroller.New()).
		Register(invitestore.New()).
		Register(filesync.New()).
		Register(reconciler.New()).
		Register(fileobject.New(200*time.Millisecond, 2*time.Second)).
		Register(inviteservice.New()).
		Register(acl.New()).
		Register(builtintemplate.New()).
		Register(converter.NewLayoutConverter()).
		Register(recordsbatcher.New()).
		Register(configfetcher.New()).
		Register(process.New()).
		Register(core.New()).
		Register(core.NewTempDirService()).
		Register(treemanager.New()).
		Register(block.New()).
		Register(indexer.New()).
		Register(detailsupdater.New()).
		Register(session.NewHookRunner()).
		Register(spacesyncstatus.NewSpaceSyncStatus()).
		Register(nodestatus.NewNodeStatus()).
		Register(syncstatus.New()).
		Register(history.New()).
		Register(gateway.New()).
		Register(export.New()).
		Register(linkpreview.New()).
		Register(unsplash.New()).
		Register(restriction.New()).
		Register(debug.New()).
		Register(collection.New()).
		Register(subscription.New()).
		Register(syncsubscriptions.New()).
		Register(builtinobjects.New()).
		Register(bookmark.New()).
		Register(importer.New()).
		Register(decorator.New()).
		Register(objectcreator.NewCreator()).
		Register(kanban.New()).
		Register(device.NewDevices()).
		Register(editor.NewObjectFactory()).
		Register(objectgraph.NewBuilder()).
		Register(account.New()).
		Register(profiler.New()).
		Register(identity.New(30*time.Second, 10*time.Second)).
		Register(templateservice.New()).
		Register(notifications.New(time.Second * 10)).
		Register(paymentserviceclient.New()).
		Register(nameservice.New()).
		Register(nameserviceclient.New()).
		Register(payments.New()).
		Register(paymentscache.New()).
		Register(peerstatus.New())
}

func MiddlewareVersion() string {
	return vcs.GetVCSInfo().Version()
}

type ComponentLogFieldsGetter interface {
	// GetLogFields returns additional useful fields for logs to debug long app start/stop duration or something else in the future
	// You don't need to provide the component name in the field's Key, because it will be added automatically
	GetLogFields() []zap.Field
}
