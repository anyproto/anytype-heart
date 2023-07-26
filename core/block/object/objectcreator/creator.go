package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Service interface {
	CreateSmartBlockFromTemplate(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error)
	// TODO Review those two methods, they are very similar but have slightly different mechanics
	CreateSubObjectInWorkspace(ctx context.Context, details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error)
	CreateSubObjectsInWorkspace(ctx context.Context, spaceID string, details []*types.Struct) (ids []string, objects []*types.Struct, err error)
	app.Component
}

type Creator struct {
	blockService      BlockService
	blockPicker       block.Picker
	objectStore       objectstore.ObjectStore
	collectionService CollectionService
	bookmark          bookmark.Service
	objectFactory     *editor.ObjectFactory
	app               *app.App
	sbtProvider       typeprovider.SmartBlockTypeProvider
	creator           Service //nolint:unused

	// TODO: remove it?
	anytype core.Service
}

type CollectionService interface {
	CreateCollection(details *types.Struct, flags []*model.InternalFlag) (coresb.SmartBlockType, *types.Struct, *state.State, error)
}

func NewCreator(sbtProvider typeprovider.SmartBlockTypeProvider) Service {
	return &Creator{
		sbtProvider: sbtProvider,
	}
}

func (c *Creator) Init(a *app.App) (err error) {
	c.anytype = a.MustComponent(core.CName).(core.Service)
	c.blockService = a.MustComponent(block.CName).(BlockService)
	c.blockPicker = a.MustComponent(block.CName).(block.Picker)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	c.collectionService = app.MustComponent[CollectionService](a)
	c.anytype = a.MustComponent(core.CName).(core.Service)
	c.app = a
	return nil
}

const CName = "objectCreator"

func (c *Creator) Name() (name string) {
	return CName
}

// TODO Temporarily
type BlockService interface {
	StateFromTemplate(templateID string, name string) (st *state.State, err error)
	CreateTreeObject(ctx context.Context, spaceID string, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, err error)
	CreateTreeObjectWithUniqueKey(ctx context.Context, spaceID string, key uniquekey.UniqueKey, initFunc block.InitFunc) (sb smartblock.SmartBlock, err error)
}

func (c *Creator) CreateSmartBlockFromTemplate(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateID != "" {
		if createState, err = c.blockService.StateFromTemplate(templateID, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, spaceID, sbType, details, createState)
}

// CreateSmartBlockFromState create new object from the provided `createState` and `details`. If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
func (c *Creator) CreateSmartBlockFromState(ctx context.Context, spaceID string, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error) {
	if createState == nil {
		createState = state.NewDoc("", nil).(*state.State)
	}
	startTime := time.Now()
	objectTypes := pbtypes.GetStringList(details, bundle.RelationKeyType.String())
	if objectTypes == nil {
		objectTypes = createState.ObjectTypes()
		if objectTypes == nil {
			objectTypes = pbtypes.GetStringList(createState.Details(), bundle.RelationKeyType.String())
		}
	}
	if len(objectTypes) == 0 {
		if ot, exists := bundle.DefaultObjectTypePerSmartblockType[sbType]; exists {
			objectTypes = []string{ot.URL()}
		} else {
			objectTypes = []string{bundle.TypeKeyPage.URL()}
		}
	}

	var relationKeys []string
	var workspaceID string
	if details != nil && details.Fields != nil {
		for k, v := range details.Fields {
			relId := addr.RelationKeyToIdPrefix + k
			if _, err2 := c.objectStore.GetRelationByID(relId); err != nil {
				// check if installed
				err = fmt.Errorf("failed to get installed relation %s: %w", relId, err2)
				return
			}
			relationKeys = append(relationKeys, k)
			createState.SetDetail(k, v)
		}

		detailsWorkspaceID := details.Fields[bundle.RelationKeyWorkspaceId.String()]
		if detailsWorkspaceID != nil && detailsWorkspaceID.GetStringValue() != "" {
			workspaceID = detailsWorkspaceID.GetStringValue()
		}
	}

	// if we don't have anything in details then check the object store
	if workspaceID == "" {
		workspaceID = c.anytype.PredefinedObjects(spaceID).Account
	}

	if workspaceID != "" {
		createState.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceID))
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(c.anytype.ProfileID(spaceID)))

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Since(startTime).Milliseconds(),
	}

	ctx = context.WithValue(ctx, eventCreate, ev)
	initFunc := func(id string) *smartblock.InitContext {
		createState.SetRootId(id)
		createState.SetObjectTypes(objectTypes)
		createState.InjectDerivedDetails()

		return &smartblock.InitContext{
			Ctx:            ctx,
			ObjectTypeUrls: objectTypes,
			State:          createState,
			RelationKeys:   relationKeys,
		}
	}

	var sb smartblock.SmartBlock

	if uKey := createState.UniqueKey(); uKey != "" {
		uk, err := uniquekey.NewUniqueKey(sbType.ToProto(), uKey)
		if err != nil {
			return "", nil, err
		}
		sb, err = c.blockService.CreateTreeObjectWithUniqueKey(ctx, spaceID, uk, initFunc)
		if err != nil {
			return "", nil, err
		}
	} else {
		sb, err = c.blockService.CreateTreeObject(ctx, spaceID, sbType, initFunc)
		if err != nil {
			return
		}
	}

	id = sb.Id()
	ev.SmartblockCreateMs = time.Since(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, sb.CombinedDetails(), nil
}

func (c *Creator) InjectWorkspaceID(details *types.Struct, spaceID string, objectID string) {
	workspaceID, err := c.anytype.GetWorkspaceIdForObject(spaceID, objectID)
	if err != nil {
		workspaceID = ""
	}
	if workspaceID == "" || details == nil {
		return
	}
	if details.Fields == nil {
		details.Fields = make(map[string]*types.Value)
	}
	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
}

func (c *Creator) CreateSet(ctx context.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	// TODO remove it, when schema will be refactored
	source := req.Source
	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema
	if len(source) == 0 {
		source = []string{converter.DefaultSetSource.URL()}
	}
	if dvContent, dvSchema, err = dataview.DataviewBlockBySource(req.SpaceId, c.sbtProvider, c.objectStore, source); err != nil {
		return
	}

	newState := state.NewDoc("", nil).NewState()

	tmpls := []template.StateTransformer{
		template.WithRequiredRelations(),
	}
	var blockContent *model.BlockContentOfDataview
	if dvSchema != nil {
		blockContent = &dvContent
	}

	if len(req.Source) > 0 {
		newState.SetDetailAndBundledRelation(bundle.RelationKeySetOf, pbtypes.StringList(req.Source))
	}

	if blockContent != nil {
		for i, view := range blockContent.Dataview.Views {
			if view.Relations == nil {
				blockContent.Dataview.Views[i].Relations = editor.GetDefaultViewRelations(blockContent.Dataview.Relations)
			}
		}
		tmpls = append(tmpls,
			template.WithDataview(*blockContent, false),
		)
	}

	template.InitTemplate(newState, tmpls...)

	// TODO: here can be a deadlock if this is somehow created from workspace (as set)
	return c.CreateSmartBlockFromState(ctx, req.SpaceId, coresb.SmartBlockTypePage, req.Details, newState)
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectInWorkspace(ctx context.Context, details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = block.Do(c.blockPicker, workspaceID, func(ws *editor.Workspaces) error {
		id, newDetails, err = ws.CreateSubObject(ctx, details)
		return err
	})
	return
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectsInWorkspace(ctx context.Context, spaceID string, details []*types.Struct) (ids []string, objects []*types.Struct, err error) {
	err = block.Do(c.blockPicker, c.anytype.PredefinedObjects(spaceID).Account, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		ids, objects, err = workspace.CreateSubObjects(ctx, details)
		return err
	})
	return
}

// ObjectCreateBookmark creates a new Bookmark object for provided URL or returns id of existing one
func (c *Creator) ObjectCreateBookmark(ctx context.Context, req *pb.RpcObjectCreateBookmarkRequest) (objectID string, newDetails *types.Struct, err error) {
	source := pbtypes.GetString(req.Details, bundle.RelationKeySource.String())
	var res bookmark.ContentFuture
	if source != "" {
		u, err := uri.NormalizeURI(source)
		if err != nil {
			return "", nil, fmt.Errorf("process uri: %w", err)
		}
		res = c.bookmark.FetchBookmarkContent(req.SpaceId, u)
	} else {
		res = func() *model.BlockContentBookmark {
			return nil
		}
	}
	return c.bookmark.CreateBookmarkObject(ctx, req.SpaceId, req.Details, res)
}

func (c *Creator) CreateObject(ctx context.Context, spaceID string, req block.DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
	details = req.GetDetails()
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	var internalFlags []*model.InternalFlag
	if v, ok := req.(block.InternalFlagsGetter); ok {
		internalFlags = v.GetInternalFlags()
		details = internalflag.PutToDetails(details, internalFlags)
	}

	var templateID string
	if v, ok := req.(block.TemplateIDGetter); ok {
		templateID = v.GetTemplateId()
	}

	var (
		objectTypeKey bundle.TypeKey
		objectTypeId  string
	)
	if forcedType != "" {
		objectTypeKey = forcedType
	} else if objectTypeId = pbtypes.GetString(details, bundle.RelationKeyType.String()); objectTypeId == "" {
		return "", nil, fmt.Errorf("missing type in details or in forcedType")
	} else {
		for typeKey, typeId := range c.anytype.PredefinedObjects(spaceID).SystemTypes {
			if typeId == objectTypeId {
				objectTypeKey = typeKey
				break
			}
		}
	}

	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(objectTypeId)
	var sbType = coresb.SmartBlockTypePage

	getUniqueKeyOrGenerate := func(sbType coresb.SmartBlockType, details *types.Struct) (uniquekey.UniqueKey, error) {
		uniqueKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
		if uniqueKey == "" {
			return uniquekey.NewUniqueKey(sbType.ToProto(), bson.NewObjectId().Hex())
		}
		return uniquekey.UniqueKeyFromString(uniqueKey)
	}

	switch objectTypeKey {
	case bundle.TypeKeyBookmark:
		return c.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details: details,
			SpaceId: spaceID,
		})
	case bundle.TypeKeySet:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return c.CreateSet(ctx, &pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: internalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
			SpaceId:       spaceID,
		})
	case bundle.TypeKeyCollection:
		var st *state.State
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
		sbType, details, st, err = c.collectionService.CreateCollection(details, internalFlags)
		if err != nil {
			return "", nil, err
		}
		return c.CreateSmartBlockFromState(ctx, spaceID, sbType, details, st)
	case bundle.TypeKeyObjectType, bundle.TypeKeyRelation:
		if templateID != "" {
			return "", nil, fmt.Errorf("template is not supported for %s", objectTypeKey)
		}
		if objectTypeKey == bundle.TypeKeyObjectType {
			sbType = coresb.SmartBlockTypeObjectType
		} else {
			sbType = coresb.SmartBlockTypeRelation
		}
		uk, err := getUniqueKeyOrGenerate(sbType, details)
		if err != nil {
			return "", nil, err
		}

		if details.GetFields() == nil {
			details.Fields = map[string]*types.Value{}
		}
		details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uk.String())

	case bundle.TypeKeyTemplate:
		sbType = coresb.SmartBlockTypeTemplate
	}

	return c.CreateSmartBlockFromTemplate(ctx, spaceID, sbType, details, templateID)
}
