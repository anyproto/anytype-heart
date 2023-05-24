package editor

import (
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
)

var log = logging.Logger("anytype-mw-editor")

type ObjectFactory struct {
	anytype              core.Service
	bookmarkBlockService bookmark.BlockService
	bookmarkService      bookmark.BookmarkService
	detailsModifier      DetailsModifier
	fileBlockService     file.BlockService
	layoutConverter      converter.LayoutConverter
	objectStore          objectstore.ObjectStore
	relationService      relation.Service
	sbtProvider          typeprovider.SmartBlockTypeProvider
	sendEvent            func(e *pb.Event)
	sourceService        source.Service
	tempDirProvider      core.TempDirProvider
	templateCloner       templateCloner
	fileService          files.Service
	config               *config.Config

	subObjectFactory subObjectFactory
}

func NewObjectFactory(
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
	layoutConverter converter.LayoutConverter,
) *ObjectFactory {
	return &ObjectFactory{
		tempDirProvider: tempDirProvider,
		sbtProvider:     sbtProvider,
		layoutConverter: layoutConverter,
	}
}

func (f *ObjectFactory) Init(a *app.App) (err error) {
	f.anytype = app.MustComponent[core.Service](a)
	f.bookmarkBlockService = app.MustComponent[bookmark.BlockService](a)
	f.bookmarkService = app.MustComponent[bookmark.BookmarkService](a)
	f.detailsModifier = app.MustComponent[DetailsModifier](a)
	f.fileBlockService = app.MustComponent[file.BlockService](a)
	f.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	f.relationService = app.MustComponent[relation.Service](a)
	f.sourceService = app.MustComponent[source.Service](a)
	f.sendEvent = app.MustComponent[event.Sender](a).Send
	f.templateCloner = app.MustComponent[templateCloner](a)
	f.fileService = app.MustComponent[files.Service](a)
	f.config = app.MustComponent[*config.Config](a)

	f.subObjectFactory = subObjectFactory{
		coreService:        f.anytype,
		fileBlockService:   f.fileBlockService,
		fileService:        f.fileService,
		indexer:            app.MustComponent[smartblock.Indexer](a),
		layoutConverter:    f.layoutConverter,
		objectStore:        f.objectStore,
		relationService:    f.relationService,
		restrictionService: app.MustComponent[restriction.Service](a),
		sbtProvider:        f.sbtProvider,
		sourceService:      f.sourceService,
		tempDirProvider:    f.tempDirProvider,
	}

	return nil
}

const CName = "objectFactory"

func (f *ObjectFactory) Name() (name string) {
	return CName
}

func (f *ObjectFactory) InitObject(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := f.sourceService.NewSource(id, initCtx.SpaceID, initCtx.BuildOpts)
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

	sb, err = f.New(sc.Type())
	if err != nil {
		return nil, fmt.Errorf("new smartblock: %w", err)
	}

	if ot != nil {
		setter, ok := sb.Inner().(smartblock.LockerSetter)
		if !ok {
			err = fmt.Errorf("should be able to provide lock from the outside")
			return
		}
		// using lock from object tree
		setter.SetLocker(ot)
	}

	// we probably don't need any locks here, because the object is initialized synchronously
	if initCtx == nil {
		initCtx = &smartblock.InitContext{}
	}
	initCtx.Source = sc
	err = sb.Init(initCtx)
	if err != nil {
		return nil, fmt.Errorf("init smartblock: %w", err)
	}

	err = migration.RunMigrations(sb, initCtx)
	if err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return
}

func (f *ObjectFactory) New(sbType model.SmartBlockType) (smartblock.SmartBlock, error) {
	sb := f.subObjectFactory.produceSmartblock()
	switch sbType {
	case model.SmartBlockType_Page, model.SmartBlockType_Date, model.SmartBlockType_BundledRelation, model.SmartBlockType_BundledObjectType:
		return NewPage(
			sb,
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
			f.fileService,
		), nil
	case model.SmartBlockType_Archive:
		return NewArchive(
			sb,
			f.detailsModifier,
			f.objectStore,
		), nil
	case model.SmartBlockType_Home:
		return NewDashboard(
			sb,
			f.detailsModifier,
			f.objectStore,
			f.relationService,
			f.anytype,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		return NewProfile(
			sb,
			f.objectStore,
			f.relationService,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.sendEvent,
			f.tempDirProvider,
			f.layoutConverter,
			f.fileService,
		), nil
	case model.SmartBlockType_File:
		return NewFiles(sb), nil
	case model.SmartBlockType_Template:
		return NewTemplate(
			sb,
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
			f.fileService,
		), nil
	case model.SmartBlockType_BundledTemplate:
		return NewTemplate(
			sb,
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
			f.fileService,
		), nil
	case model.SmartBlockType_Workspace:
		return NewWorkspace(
			sb,
			f.objectStore,
			f.anytype,
			f.relationService,
			f.sourceService,
			f.detailsModifier,
			f.sbtProvider,
			f.layoutConverter,
			f.subObjectFactory,
			f.templateCloner,
			f.config,
		), nil
	case model.SmartBlockType_MissingObject:
		return NewMissingObject(sb), nil
	case model.SmartBlockType_Widget:
		return NewWidgetObject(sb, f.objectStore, f.relationService, f.layoutConverter), nil
	case model.SmartBlockType_SubObject:
		return nil, fmt.Errorf("subobject not supported via factory")
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sbType)
	}
}
