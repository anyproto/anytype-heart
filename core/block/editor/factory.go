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
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

type ObjectFactory struct {
	anytype              core.Service
	bookmarkBlockService bookmark.BlockService
	bookmarkService      bookmark.BookmarkService
	detailsModifier      DetailsModifier
	fileBlockService     file.BlockService
	objectStore          objectstore.ObjectStore
	relationService      relation2.Service
	sourceService        source.Service
	sendEvent            func(e *pb.Event)
	tempDirProvider      core.TempDirProvider
	collectionService    CollectionService
	sbtProvider          typeprovider.SmartBlockTypeProvider
	layoutConverter      converter.LayoutConverter

	app *app.App
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
	f.collectionService = app.MustComponent[CollectionService](a)

	f.app = a
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
	initCtx.App = f.app
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
	switch sbType {
	case model.SmartBlockType_Page, model.SmartBlockType_Date:
		return NewPage(
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_Archive:
		return NewArchive(
			f.detailsModifier,
			f.objectStore,
		), nil
	case model.SmartBlockType_Home:
		return NewDashboard(
			f.detailsModifier,
			f.objectStore,
			f.relationService,
			f.anytype,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_Set:
		return NewSet(
			f.anytype,
			f.objectStore,
			f.relationService,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_Collection:
		return NewCollection(
			f.anytype,
			f.objectStore,
			f.relationService,
			f.collectionService,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		return NewProfile(
			f.objectStore,
			f.relationService,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.sendEvent,
			f.tempDirProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		return NewObjectType(
			f.anytype,
			f.objectStore,
			f.relationService,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_BundledRelation:
		return NewSet(
			f.anytype,
			f.objectStore,
			f.relationService,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_File:
		return NewFiles(), nil
	case model.SmartBlockType_Template:
		return NewTemplate(
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_BundledTemplate:
		return NewTemplate(
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_Breadcrumbs:
		return NewBreadcrumbs(), nil
	case model.SmartBlockType_Workspace:
		return NewWorkspace(
			f.objectStore,
			f.anytype,
			f.relationService,
			f.sourceService,
			f.detailsModifier,
			f.fileBlockService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
		), nil
	case model.SmartBlockType_MissingObject:
		return NewMissingObject(), nil
	case model.SmartBlockType_Widget:
		return NewWidgetObject(f.objectStore, f.relationService, f.layoutConverter), nil
	case model.SmartBlockType_SubObject:
		return nil, fmt.Errorf("subobject not supported via factory")
	case model.SmartBlockType_MarketplaceType:
		return nil, fmt.Errorf("marketplace type is deprecated")
	case model.SmartBlockType_MarketplaceRelation:
		return nil, fmt.Errorf("marketplace type is deprecated")
	case model.SmartBlockType_MarketplaceTemplate:
		return nil, fmt.Errorf("marketplace type is deprecated")
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sbType)
	}
}
