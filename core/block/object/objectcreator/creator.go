package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/session"
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
	CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error)
	CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error)
	CreateSet(req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error)
	CreateSubObjectInWorkspace(details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error)
	CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error)
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

func NewCreator(sbtProvider typeprovider.SmartBlockTypeProvider) *Creator {
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
	StateFromTemplate(ctx session.Context, templateID, name string) (st *state.State, err error)
	CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, err error)
}

func (c *Creator) CreateSmartBlockFromTemplate(ctx session.Context, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateID != "" {
		if createState, err = c.blockService.StateFromTemplate(ctx, templateID, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, sbType, details, createState)
}

// CreateSmartBlockFromState create new object from the provided `createState` and `details`. If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
func (c *Creator) CreateSmartBlockFromState(ctx session.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error) {
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
		workspaceID = c.anytype.PredefinedBlocks().Account
	}

	if workspaceID != "" {
		createState.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceID))
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(c.anytype.ProfileID()))

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Since(startTime).Milliseconds(),
	}

	if sbType == coresb.SmartBlockTypeSubObject {
		return c.CreateSubObjectInWorkspace(ctx, createState.CombinedDetails(), workspaceID)
	}

	cctx := context.WithValue(ctx.Context(), eventCreate, ev)
	sb, err := c.blockService.CreateTreeObject(cctx, sbType, func(id string) *smartblock.InitContext {
		createState.SetRootId(id)
		createState.SetObjectTypes(objectTypes)
		createState.InjectDerivedDetails()

		return &smartblock.InitContext{
			Ctx:            ctx,
			ObjectTypeUrls: objectTypes,
			State:          createState,
			RelationKeys:   relationKeys,
		}
	})
	if err != nil {
		return
	}
	id = sb.Id()
	ev.SmartblockCreateMs = time.Since(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, sb.CombinedDetails(), nil
}

func (c *Creator) InjectWorkspaceID(details *types.Struct, objectID string) {
	workspaceID, err := c.anytype.GetWorkspaceIdForObject(objectID)
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

func (c *Creator) CreateSet(ctx session.Context, req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	// TODO remove it, when schema will be refactored
	source := req.Source
	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema
	if len(source) == 0 {
		source = []string{converter.DefaultSetSource.URL()}
	}
	if dvContent, dvSchema, err = dataview.DataviewBlockBySource(c.sbtProvider, c.objectStore, source); err != nil {
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
	return c.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypePage, req.Details, newState)
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectInWorkspace(ctx session.Context, details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = block.Do(c.blockPicker, ctx, workspaceID, func(ws *editor.Workspaces) error {
		id, newDetails, err = ws.CreateSubObject(ctx, details)
		return err
	})
	return
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectsInWorkspace(ctx session.Context, details []*types.Struct) (ids []string, objects []*types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = block.Do(c.blockPicker, ctx, c.anytype.PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
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
func (c *Creator) ObjectCreateBookmark(ctx session.Context, req *pb.RpcObjectCreateBookmarkRequest) (objectID string, newDetails *types.Struct, err error) {
	source := pbtypes.GetString(req.Details, bundle.RelationKeySource.String())
	var res bookmark.ContentFuture
	if source != "" {
		u, err := uri.NormalizeURI(source)
		if err != nil {
			return "", nil, fmt.Errorf("process uri: %w", err)
		}
		res = c.bookmark.FetchBookmarkContent(u)
	} else {
		res = func() *model.BlockContentBookmark {
			return nil
		}
	}
	return c.bookmark.CreateBookmarkObject(ctx, req.Details, res)
}

func (c *Creator) CreateObject(ctx session.Context, req block.DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
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

	var objectType string
	if forcedType != "" {
		objectType = forcedType.URL()
	} else if objectType = pbtypes.GetString(details, bundle.RelationKeyType.String()); objectType == "" {
		return "", nil, fmt.Errorf("missing type in details or in forcedType")
	}

	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(objectType)
	var sbType = coresb.SmartBlockTypePage

	switch objectType {
	case bundle.TypeKeyBookmark.URL():
		return c.ObjectCreateBookmark(ctx, &pb.RpcObjectCreateBookmarkRequest{
			Details: details,
		})
	case bundle.TypeKeySet.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return c.CreateSet(ctx, &pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: internalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
		})
	case bundle.TypeKeyCollection.URL():
		var st *state.State
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_collection))
		sbType, details, st, err = c.collectionService.CreateCollection(details, internalFlags)
		if err != nil {
			return "", nil, err
		}
		return c.CreateSmartBlockFromState(ctx, sbType, details, st)
	case bundle.TypeKeyObjectType.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
		return c.CreateSubObjectInWorkspace(ctx, details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelation.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
		return c.CreateSubObjectInWorkspace(ctx, details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelationOption.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relationOption))
		return c.CreateSubObjectInWorkspace(ctx, details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyTemplate.URL():
		sbType = coresb.SmartBlockTypeTemplate
	}

	return c.CreateSmartBlockFromTemplate(ctx, sbType, details, templateID)
}
