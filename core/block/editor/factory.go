package editor

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/accountobject"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/core/files/reconciler"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-editor")

type ObjectDeleter interface {
	DeleteObjectByFullID(id domain.FullID) (err error)
}

type accountService interface {
	AccountID() string
	PersonalSpaceID() string
	MyParticipantId(spaceId string) string
	Keys() *accountdata.AccountKeys
}

type deviceService interface {
	SaveDeviceInfo(info smartblock.ApplyInfo) error
}

type ObjectFactory struct {
	bookmarkService     bookmark.BookmarkService
	fileBlockService    file.BlockService
	layoutConverter     converter.LayoutConverter
	objectStore         objectstore.ObjectStore
	sourceService       source.Service
	tempDirProvider     core.TempDirProvider
	fileService         files.Service
	config              *config.Config
	picker              cache.ObjectGetter
	eventSender         event.Sender
	restrictionService  restriction.Service
	indexer             smartblock.Indexer
	spaceService        spaceService
	accountService      accountService
	fileObjectService   fileobject.Service
	processService      process.Service
	fileUploaderService fileuploader.Service
	fileReconciler      reconciler.Reconciler
	objectDeleter       ObjectDeleter
	deviceService       deviceService
	spaceIdResolver     idresolver.Resolver
	commonFile          fileservice.FileService
	dbProvider          anystoreprovider.Provider
}

func NewObjectFactory() *ObjectFactory {
	return &ObjectFactory{}
}

func (f *ObjectFactory) Init(a *app.App) (err error) {
	f.config = app.MustComponent[*config.Config](a)
	f.picker = app.MustComponent[cache.ObjectGetter](a)
	f.indexer = app.MustComponent[smartblock.Indexer](a)
	f.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	f.fileService = app.MustComponent[files.Service](a)
	f.eventSender = app.MustComponent[event.Sender](a)
	f.spaceService = app.MustComponent[spaceService](a)
	f.sourceService = app.MustComponent[source.Service](a)
	f.objectDeleter = app.MustComponent[ObjectDeleter](a)
	f.deviceService = app.MustComponent[deviceService](a)
	f.accountService = app.MustComponent[accountService](a)
	f.processService = app.MustComponent[process.Service](a)
	f.fileReconciler = app.MustComponent[reconciler.Reconciler](a)
	f.bookmarkService = app.MustComponent[bookmark.BookmarkService](a)
	f.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	f.layoutConverter = app.MustComponent[converter.LayoutConverter](a)
	f.fileBlockService = app.MustComponent[file.BlockService](a)
	f.fileObjectService = app.MustComponent[fileobject.Service](a)
	f.restrictionService = app.MustComponent[restriction.Service](a)
	f.fileUploaderService = app.MustComponent[fileuploader.Service](a)
	f.objectDeleter = app.MustComponent[ObjectDeleter](a)
	f.fileReconciler = app.MustComponent[reconciler.Reconciler](a)
	f.deviceService = app.MustComponent[deviceService](a)
	f.spaceIdResolver = app.MustComponent[idresolver.Resolver](a)
	f.commonFile = app.MustComponent[fileservice.FileService](a)
	f.dbProvider = app.MustComponent[anystoreprovider.Provider](a)
	return nil
}

const CName = "objectFactory"

func (f *ObjectFactory) Name() (name string) {
	return CName
}

func (f *ObjectFactory) InitObject(space smartblock.Space, id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := f.sourceService.NewSource(initCtx.Ctx, space, id, initCtx.BuildOpts)
	if err != nil {
		return
	}

	var ot objecttree.ObjectTree
	if p, ok := sc.(source.ObjectTreeProvider); ok {
		ot = p.Tree()
	}
	defer func() {
		if err != nil && ot != nil {
			ot.Close()
		}
	}()

	sb, err = f.New(space, sc.Type())
	if err != nil {
		return nil, fmt.Errorf("new smartblock: %w", err)
	}

	if ot != nil {
		// using lock from object tree
		sb.SetLocker(ot)
	}

	initCtx.Source = sc
	// adding locks as a temporary measure to find the place where we have races in our code
	sb.Lock()
	defer sb.Unlock()
	err = sb.Init(initCtx)
	if err != nil {
		return nil, fmt.Errorf("init smartblock: %w", err)
	}

	migration.RunMigrations(sb, initCtx)
	return sb, sb.Apply(initCtx.State, smartblock.NoHistory, smartblock.NoEvent, smartblock.NoRestrictions, smartblock.SkipIfNoChanges, smartblock.KeepInternalFlags, smartblock.IgnoreNoPermissions)
}

func (f *ObjectFactory) produceSmartblock(space smartblock.Space) (smartblock.SmartBlock, spaceindex.Store) {
	store := f.objectStore.SpaceIndex(space.Id())
	return smartblock.New(
		space,
		f.accountService.MyParticipantId(space.Id()),
		f.restrictionService,
		store,
		f.objectStore,
		f.indexer,
		f.eventSender,
		f.spaceIdResolver,
	), store
}

func (f *ObjectFactory) New(space smartblock.Space, sbType coresb.SmartBlockType) (smartblock.SmartBlock, error) {
	sb, spaceIndex := f.produceSmartblock(space)
	switch sbType {
	case coresb.SmartBlockTypePage,
		coresb.SmartBlockTypeDate,
		coresb.SmartBlockTypeBundledRelation,
		coresb.SmartBlockTypeBundledObjectType,
		coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption,
		coresb.SmartBlockTypeChatObject:
		return f.newPage(space.Id(), sb), nil
	case coresb.SmartBlockTypeObjectType:
		return f.newObjectType(space.Id(), sb), nil
	case coresb.SmartBlockTypeArchive:
		return NewArchive(sb, spaceIndex), nil
	case coresb.SmartBlockTypeHome:
		return NewDashboard(sb, spaceIndex, f.layoutConverter), nil
	case coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeAnytypeProfile:
		return f.newProfile(space.Id(), sb), nil
	case coresb.SmartBlockTypeFileObject:
		return f.newFile(space.Id(), sb), nil
	case coresb.SmartBlockTypeTemplate,
		coresb.SmartBlockTypeBundledTemplate:
		return f.newTemplate(space.Id(), sb), nil
	case coresb.SmartBlockTypeWorkspace:
		return f.newWorkspace(sb, spaceIndex), nil
	case coresb.SmartBlockTypeSpaceView:
		return f.newSpaceView(sb), nil
	case coresb.SmartBlockTypeMissingObject:
		return NewMissingObject(sb), nil
	case coresb.SmartBlockTypeWidget:
		return NewWidgetObject(sb, spaceIndex, f.layoutConverter), nil
	case coresb.SmartBlockTypeNotificationObject:
		return NewNotificationObject(sb), nil
	case coresb.SmartBlockTypeSubObject:
		return nil, fmt.Errorf("subobject not supported via factory")
	case coresb.SmartBlockTypeParticipant:
		return f.newParticipant(space.Id(), sb, spaceIndex), nil
	case coresb.SmartBlockTypeDevicesObject:
		return NewDevicesObject(sb, f.deviceService), nil
	case coresb.SmartBlockTypeChatDerivedObject:
		db, err := f.dbProvider.GetCrdtDb(space.Id())
		if err != nil {
			return nil, fmt.Errorf("get crdt db: %w", err)
		}
		return chatobject.New(sb, f.accountService, f.eventSender, db, spaceIndex), nil
	case coresb.SmartBlockTypeAccountObject:
		db, err := f.dbProvider.GetCrdtDb(space.Id())
		if err != nil {
			return nil, fmt.Errorf("get crdt db: %w", err)
		}
		return accountobject.New(sb, f.accountService.Keys(), spaceIndex, f.layoutConverter, f.fileObjectService, db, f.config), nil
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sbType)
	}
}
