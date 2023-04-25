package editor

import (
	"fmt"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/migration"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/files"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
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
	relationService      relation2.Service
	sbtProvider          typeprovider.SmartBlockTypeProvider
	sendEvent            func(e *pb.Event)
	sourceService        source.Service
	tempDirProvider      core.TempDirProvider
	templateCloner       templateCloner
	fileService          files.IService

	smartblockFactory smartblockFactory
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
	f.relationService = app.MustComponent[relation2.Service](a)
	f.sourceService = app.MustComponent[source.Service](a)
	f.sendEvent = app.MustComponent[event.Sender](a).Send
	f.templateCloner = app.MustComponent[templateCloner](a)
	f.fileService = app.MustComponent[files.IService](a)

	f.smartblockFactory = smartblockFactory{
		anytype:            f.anytype,
		fileService:        f.fileService,
		indexer:            app.MustComponent[smartblock.Indexer](a),
		objectStore:        f.objectStore,
		relationService:    f.relationService,
		restrictionService: app.MustComponent[restriction.Service](a),
	}

	return nil
}

const CName = "objectFactory"

func (f *ObjectFactory) Name() (name string) {
	return CName
}

func (f *ObjectFactory) InitObject(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := f.sourceService.NewSource(id, initCtx.SpaceID, initCtx.BuildTreeOpts)
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

	basicEditor := basic.NewBasic(sb, f.objectStore, f.relationService, f.layoutConverter)
	if len(initCtx.ObjectTypeUrls) > 0 && len(sb.ObjectTypes()) == 0 {
		err = basicEditor.SetObjectTypesInState(initCtx.State, initCtx.ObjectTypeUrls)
		if err != nil {
			return nil, fmt.Errorf("set object types in state: %w", err)
		}
	}

	err = migration.RunMigrations(sb, initCtx)
	if err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return
}

func (f *ObjectFactory) New(sbType model.SmartBlockType) (smartblock.SmartBlock, error) {
	sb := f.smartblockFactory.Produce()
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
			f.fileBlockService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
			f.smartblockFactory,
			f.templateCloner,
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
