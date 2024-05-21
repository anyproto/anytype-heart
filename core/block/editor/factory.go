package editor

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-editor")

type ObjectDeleter interface {
	DeleteObjectByFullID(id domain.FullID) (err error)
}

type accountService interface {
	PersonalSpaceID() string
	MyParticipantId(spaceId string) string
}

type ObjectFactory struct {
	bookmarkService     bookmark.BookmarkService
	fileBlockService    file.BlockService
	layoutConverter     converter.LayoutConverter
	objectStore         objectstore.ObjectStore
	sourceService       source.Service
	tempDirProvider     core.TempDirProvider
	fileStore           filestore.FileStore
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
	objectDeleter       ObjectDeleter
}

func NewObjectFactory() *ObjectFactory {
	return &ObjectFactory{}
}

func (f *ObjectFactory) Init(a *app.App) (err error) {
	f.bookmarkService = app.MustComponent[bookmark.BookmarkService](a)
	f.fileBlockService = app.MustComponent[file.BlockService](a)
	f.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	f.restrictionService = app.MustComponent[restriction.Service](a)
	f.sourceService = app.MustComponent[source.Service](a)
	f.fileService = app.MustComponent[files.Service](a)
	f.fileStore = app.MustComponent[filestore.FileStore](a)
	f.config = app.MustComponent[*config.Config](a)
	f.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	f.layoutConverter = app.MustComponent[converter.LayoutConverter](a)
	f.picker = app.MustComponent[cache.ObjectGetter](a)
	f.indexer = app.MustComponent[smartblock.Indexer](a)
	f.eventSender = app.MustComponent[event.Sender](a)
	f.spaceService = app.MustComponent[spaceService](a)
	f.accountService = app.MustComponent[accountService](a)
	f.fileObjectService = app.MustComponent[fileobject.Service](a)
	f.processService = app.MustComponent[process.Service](a)
	f.fileUploaderService = app.MustComponent[fileuploader.Service](a)
	f.objectDeleter = app.MustComponent[ObjectDeleter](a)
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

func (f *ObjectFactory) produceSmartblock(space smartblock.Space) smartblock.SmartBlock {
	return smartblock.New(
		space,
		f.accountService.MyParticipantId(space.Id()),
		f.fileStore,
		f.restrictionService,
		f.objectStore,
		f.indexer,
		f.eventSender,
	)
}

func (f *ObjectFactory) New(space smartblock.Space, sbType coresb.SmartBlockType) (smartblock.SmartBlock, error) {
	sb := f.produceSmartblock(space)
	switch sbType {
	case coresb.SmartBlockTypePage,
		coresb.SmartBlockTypeDate,
		coresb.SmartBlockTypeBundledRelation,
		coresb.SmartBlockTypeBundledObjectType,
		coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption:
		return f.newPage(sb), nil
	case coresb.SmartBlockTypeArchive:
		return NewArchive(sb, f.objectStore), nil
	case coresb.SmartBlockTypeHome:
		return NewDashboard(sb, f.objectStore, f.layoutConverter), nil
	case coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeAnytypeProfile:
		return f.newProfile(sb), nil
	case coresb.SmartBlockTypeFileObject:
		return f.newFile(sb), nil
	case coresb.SmartBlockTypeTemplate,
		coresb.SmartBlockTypeBundledTemplate:
		return f.newTemplate(sb), nil
	case coresb.SmartBlockTypeWorkspace:
		return f.newWorkspace(sb), nil
	case coresb.SmartBlockTypeSpaceView:
		return f.newSpaceView(sb), nil
	case coresb.SmartBlockTypeMissingObject:
		return NewMissingObject(sb), nil
	case coresb.SmartBlockTypeWidget:
		return NewWidgetObject(sb, f.objectStore, f.layoutConverter, f.accountService), nil
	case coresb.SmartBlockTypeNotificationObject:
		return NewNotificationObject(sb), nil
	case coresb.SmartBlockTypeSubObject:
		return nil, fmt.Errorf("subobject not supported via factory")
	case coresb.SmartBlockTypeParticipant:
		return f.newParticipant(sb), nil
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sbType)
	}
}
