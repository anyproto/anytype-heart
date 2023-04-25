package editor

import (
	"fmt"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	relation2 "github.com/anytypeio/go-anytype-middleware/core/relation"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type ObjectFactory struct {
	anytype              core.Service
	bookmarkBlockService bookmark.BlockService
	bookmarkService      bookmark.BookmarkService
	detailsModifier      DetailsModifier
	fileBlockService     file.BlockService
	importerCtrl         _import.Services
	objectCreator        _import.ObjectCreator
	objectStore          objectstore.ObjectStore
	relationService      relation2.Service
	sourceService        source.Service
	sendEvent            func(e *pb.Event)

	app *app.App
}

func NewObjectFactory() *ObjectFactory {
	return &ObjectFactory{}
}

func (f *ObjectFactory) Init(a *app.App) (err error) {
	f.anytype = app.MustComponent[core.Service](a)
	f.bookmarkBlockService = app.MustComponent[bookmark.BlockService](a)
	f.bookmarkService = app.MustComponent[bookmark.BookmarkService](a)
	f.detailsModifier = app.MustComponent[DetailsModifier](a)
	f.fileBlockService = app.MustComponent[file.BlockService](a)
	f.importerCtrl = app.MustComponent[_import.Services](a)
	f.objectCreator = app.MustComponent[_import.ObjectCreator](a)
	f.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	f.relationService = app.MustComponent[relation2.Service](a)
	f.sourceService = app.MustComponent[source.Service](a)
	f.sendEvent = app.MustComponent[event.Sender](a).Send

	f.app = a
	return nil
}

const CName = "objectFactory"

func (f *ObjectFactory) Name() (name string) {
	return CName
}

func (f *ObjectFactory) InitObject(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	var ot objecttree.ObjectTree
	if initCtx != nil {
		ot = initCtx.ObjectTree
	}

	defer func() {
		if err != nil && ot != nil {
			initCtx.ObjectTree.Close()
		}
	}()

	sc, err := f.sourceService.NewSource(id, ot)
	if err != nil {
		return
	}
	sb = f.New(sc.Type())

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
	return
}

func (f *ObjectFactory) New(sbType model.SmartBlockType) smartblock.SmartBlock {
	switch sbType {
	case model.SmartBlockType_Page, model.SmartBlockType_Date:
		return NewPage(
			f.objectStore,
			f.importerCtrl,
			f.objectCreator,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
		)
	case model.SmartBlockType_Archive:
		return NewArchive(
			f.detailsModifier,
			f.objectStore,
		)
	case model.SmartBlockType_Home:
		return NewDashboard(
			f.detailsModifier,
			f.objectStore,
			f.importerCtrl,
			f.objectCreator,
			f.anytype,
		)
	case model.SmartBlockType_Set:
		return NewSet(
			f.anytype,
			f.objectStore,
			f.relationService,
		)
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		return NewProfile(
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.sendEvent,
		)
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		return NewObjectType(
			f.anytype,
			f.objectStore,
			f.relationService,
		)
	case model.SmartBlockType_BundledRelation:
		return NewSet(
			f.anytype,
			f.objectStore,
			f.relationService,
		)
	case model.SmartBlockType_SubObject:
		return NewSubObject(
			f.objectStore,
			f.fileBlockService,
			f.anytype,
			f.relationService,
		)
	case model.SmartBlockType_File:
		return NewFiles()
	case model.SmartBlockType_MarketplaceType:
		return NewMarketplaceType(
			f.anytype,
			f.objectStore,
			f.relationService,
		)
	case model.SmartBlockType_MarketplaceRelation:
		return NewMarketplaceRelation(
			f.anytype,
			f.objectStore,
			f.relationService,
		)
	case model.SmartBlockType_MarketplaceTemplate:
		return NewMarketplaceTemplate(
			f.anytype,
			f.objectStore,
			f.relationService,
		)
	case model.SmartBlockType_Template:
		return NewTemplate(
			f.objectStore,
			f.importerCtrl,
			f.objectCreator,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
		)
	case model.SmartBlockType_BundledTemplate:
		return NewTemplate(
			f.objectStore,
			f.importerCtrl,
			f.objectCreator,
			f.anytype,
			f.fileBlockService,
			f.bookmarkBlockService,
			f.bookmarkService,
			f.relationService,
		)
	case model.SmartBlockType_Breadcrumbs:
		return NewBreadcrumbs()
	case model.SmartBlockType_Workspace:
		return NewWorkspace(
			f.objectStore,
			f.anytype,
			f.relationService,
			f.sourceService,
			f.detailsModifier,
			f.fileBlockService,
		)
	case model.SmartBlockType_Widget:
		return NewWidgetObject()
	default:
		panic(fmt.Errorf("unexpected smartblock type: %v", sbType))
	}
}
