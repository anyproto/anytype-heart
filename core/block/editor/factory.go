package editor

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/editor/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

var log = logging.Logger("anytype-mw-editor")

type spaceIndexer interface {
	smartblock.Indexer
	ReindexSpace(spaceID string) error
}

type personalIDProvider interface {
	PersonalSpaceID() string
}

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spaceID string, ids []string) ([]string, []*types.Struct, error)
}

type ObjectFactory struct {
	anytype             core.Service
	bookmarkService     bookmark.BookmarkService
	detailsModifier     DetailsModifier
	fileBlockService    file.BlockService
	layoutConverter     converter.LayoutConverter
	objectStore         objectstore.ObjectStore
	systemObjectService system_object.Service
	sbtProvider         typeprovider.SmartBlockTypeProvider
	sourceService       source.Service
	tempDirProvider     core.TempDirProvider
	templateCloner      templateCloner
	fileService         files.Service
	config              *config.Config
	picker              getblock.Picker
	eventSender         event.Sender
	restrictionService  restriction.Service
	indexer             spaceIndexer
	spaceService        spaceService
	objectDeriver       objectDeriver
}

func NewObjectFactory() *ObjectFactory {
	return &ObjectFactory{}
}

func (f *ObjectFactory) Init(a *app.App) (err error) {
	f.anytype = app.MustComponent[core.Service](a)
	f.bookmarkService = app.MustComponent[bookmark.BookmarkService](a)
	f.detailsModifier = app.MustComponent[DetailsModifier](a)
	f.fileBlockService = app.MustComponent[file.BlockService](a)
	f.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	f.systemObjectService = app.MustComponent[system_object.Service](a)
	f.restrictionService = app.MustComponent[restriction.Service](a)
	f.sourceService = app.MustComponent[source.Service](a)
	f.templateCloner = app.MustComponent[templateCloner](a)
	f.fileService = app.MustComponent[files.Service](a)
	f.config = app.MustComponent[*config.Config](a)
	f.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	f.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	f.layoutConverter = app.MustComponent[converter.LayoutConverter](a)
	f.picker = app.MustComponent[getblock.Picker](a)
	f.indexer = app.MustComponent[spaceIndexer](a)
	f.eventSender = app.MustComponent[event.Sender](a)
	f.objectDeriver = app.MustComponent[objectDeriver](a)
	f.spaceService = app.MustComponent[spaceService](a)

	return nil
}

const CName = "objectFactory"

func (f *ObjectFactory) Name() (name string) {
	return CName
}

func (f *ObjectFactory) InitObject(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := f.sourceService.NewSource(initCtx.Ctx, id, initCtx.SpaceID, initCtx.BuildOpts)
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
		// using lock from object tree
		sb.SetLocker(ot)
	}

	// we probably don't need any locks here, because the object is initialized synchronously
	initCtx.Source = sc
	err = sb.Init(initCtx)
	if err != nil {
		return nil, fmt.Errorf("init smartblock: %w", err)
	}

	migration.RunMigrations(sb, initCtx)
	return sb, sb.Apply(initCtx.State, smartblock.NoHistory, smartblock.NoEvent, smartblock.NoRestrictions, smartblock.SkipIfNoChanges)
}

func (f *ObjectFactory) produceSmartblock() smartblock.SmartBlock {
	return smartblock.New(
		f.anytype,
		f.fileService,
		f.restrictionService,
		f.objectStore,
		f.systemObjectService,
		f.indexer,
		f.eventSender,
	)
}

func (f *ObjectFactory) New(sbType coresb.SmartBlockType) (smartblock.SmartBlock, error) {
	sb := f.produceSmartblock()
	switch sbType {
	case coresb.SmartBlockTypePage,
		coresb.SmartBlockTypeDate,
		coresb.SmartBlockTypeBundledRelation,
		coresb.SmartBlockTypeBundledObjectType,
		coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption:
		return NewPage(
			sb,
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.picker,
			f.bookmarkService,
			f.systemObjectService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
			f.fileService,
			f.eventSender,
		), nil
	case coresb.SmartBlockTypeArchive:
		return NewArchive(
			sb,
			f.detailsModifier,
			f.objectStore,
		), nil
	case coresb.SmartBlockTypeHome:
		return NewDashboard(
			sb,
			f.detailsModifier,
			f.objectStore,
			f.systemObjectService,
			f.anytype,
			f.layoutConverter,
		), nil
	case coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeAnytypeProfile:
		return NewProfile(
			sb,
			f.objectStore,
			f.systemObjectService,
			f.fileBlockService,
			f.anytype,
			f.picker,
			f.bookmarkService,
			f.tempDirProvider,
			f.layoutConverter,
			f.fileService,
			f.eventSender,
		), nil
	case coresb.SmartBlockTypeFile:
		return NewFiles(sb), nil
	case coresb.SmartBlockTypeTemplate,
		coresb.SmartBlockTypeBundledTemplate:
		return NewTemplate(
			sb,
			f.objectStore,
			f.anytype,
			f.fileBlockService,
			f.picker,
			f.bookmarkService,
			f.systemObjectService,
			f.tempDirProvider,
			f.sbtProvider,
			f.layoutConverter,
			f.fileService,
			f.eventSender,
		), nil
	case coresb.SmartBlockTypeWorkspace:
		return NewWorkspace(
			sb,
			f.objectStore,
			f.anytype,
			f.systemObjectService,
			f.sourceService,
			f.detailsModifier,
			f.sbtProvider,
			f.layoutConverter,
			f.templateCloner,
			f.config,
			f.eventSender,
			f.objectDeriver,
		), nil
	case coresb.SmartBlockTypeSpaceObject:
		return newSpaceObject(
			sb,
			f.spaceService,
		), nil
	case coresb.SmartBlockTypeMissingObject:
		return NewMissingObject(sb), nil
	case coresb.SmartBlockTypeWidget:
		return NewWidgetObject(sb, f.objectStore, f.systemObjectService, f.layoutConverter), nil
	case coresb.SmartBlockTypeSubObject:
		return nil, fmt.Errorf("subobject not supported via factory")
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sbType)
	}
}
