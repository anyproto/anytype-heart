package object

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/schema"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Creator struct {
	blockService BlockService
	blockPicker  block.BlockPicker
	objectStore  objectstore.ObjectStore
	bookmark     bookmark.Service

	// TODO: remove it?
	anytype core.Service
}

func NewCreator() *Creator {
	return &Creator{}
}

func (c *Creator) Init(a *app.App) (err error) {
	c.anytype = a.MustComponent(core.CName).(core.Service)
	c.blockService = a.MustComponent(block.CName).(BlockService)
	c.blockPicker = a.MustComponent(block.CName).(block.BlockPicker)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	return nil
}

const CName = "objectCreator"

func (c *Creator) Name() (name string) {
	return CName
}

// TODO Temporarily
type BlockService interface {
	NewSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error)
	StateFromTemplate(templateId, name string) (st *state.State, err error)
}

func (c *Creator) CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, templateId string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateId != "" {
		if createState, err = c.blockService.StateFromTemplate(templateId, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, sbType, details, relationIds, createState)
}

func (c *Creator) CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error) {
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

	var workspaceId string
	if details != nil && details.Fields != nil {
		for k, v := range details.Fields {
			createState.SetDetail(k, v)
			// TODO: add relations to relationIds
		}

		detailsWorkspaceId := details.Fields[bundle.RelationKeyWorkspaceId.String()]
		if detailsWorkspaceId != nil && detailsWorkspaceId.GetStringValue() != "" {
			workspaceId = detailsWorkspaceId.GetStringValue()
		}
	}

	// if we don't have anything in details then check the object store
	if workspaceId == "" {
		workspaceId = c.anytype.PredefinedBlocks().Account
	}

	if workspaceId != "" {
		createState.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceId))
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(c.anytype.ProfileID()))

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Now().Sub(startTime).Milliseconds(),
	}
	ctx = context.WithValue(ctx, eventCreate, ev)
	var tid = thread.Undef
	if id := pbtypes.GetString(createState.CombinedDetails(), bundle.RelationKeyId.String()); id != "" {
		tid, err = thread.Decode(id)
		if err != nil {
			log.Errorf("failed to decode thread id from the state: %s", err.Error())
		}
	}
	csm, err := c.CreateObjectInWorkspace(ctx, workspaceId, tid, sbType)
	if err != nil {
		err = fmt.Errorf("anytype.CreateBlock error: %v", err)
		return
	}
	id = csm.ID()
	createState.SetRootId(id)
	createState.SetObjectTypes(objectTypes)
	createState.InjectDerivedDetails()

	initCtx := &smartblock.InitContext{
		ObjectTypeUrls: objectTypes,
		State:          createState,
		RelationIds:    relationIds,
	}
	var sb smartblock.SmartBlock
	if sb, err = c.blockService.NewSmartBlock(id, initCtx); err != nil {
		return id, nil, err
	}
	ev.SmartblockCreateMs = time.Now().Sub(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, sb.CombinedDetails(), sb.Close()
}

// todo: rewrite with options
// withId may me empty
func (c *Creator) CreateObjectInWorkspace(ctx context.Context, workspaceId string, withId thread.ID, sbType coresb.SmartBlockType) (csm core.SmartBlock, err error) {
	startTime := time.Now()
	ev, exists := ctx.Value(eventCreate).(*metrics.CreateObjectEvent)
	err = block.DoWithContext(c.blockPicker, ctx, workspaceId, func(workspace *editor.Workspaces) error {
		if exists {
			ev.GetWorkspaceBlockWaitMs = time.Now().Sub(startTime).Milliseconds()
		}
		csm, err = workspace.CreateObject(withId, sbType)
		if exists {
			ev.WorkspaceCreateMs = time.Now().Sub(startTime).Milliseconds() - ev.GetWorkspaceBlockWaitMs
		}
		if err != nil {
			return fmt.Errorf("anytype.CreateBlock error: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return csm, nil
}

func (c *Creator) InjectWorkspaceId(details *types.Struct, objectId string) {
	workspaceId, err := c.anytype.GetWorkspaceIdForObject(objectId)
	if err != nil {
		workspaceId = ""
	}
	if workspaceId == "" || details == nil {
		return
	}
	threads.WorkspaceLogger.
		With("workspace id", workspaceId).
		Debug("adding workspace id to new object")
	if details.Fields == nil {
		details.Fields = make(map[string]*types.Value)
	}
	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceId)
}

func (c *Creator) CreateSet(req *pb.RpcObjectCreateSetRequest) (setId string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema
	if len(req.Source) != 0 {
		if dvContent, dvSchema, err = dataview.DataviewBlockBySource(c.objectStore, req.Source); err != nil {
			return
		}
	}

	newState := state.NewDoc("", nil).NewState()

	name := pbtypes.GetString(req.Details, bundle.RelationKeyName.String())
	icon := pbtypes.GetString(req.Details, bundle.RelationKeyIconEmoji.String())

	tmpls := []template.StateTransformer{
		template.WithForcedDetail(bundle.RelationKeyName, pbtypes.String(name)),
		template.WithForcedDetail(bundle.RelationKeyIconEmoji, pbtypes.String(icon)),
		template.WithRequiredRelations(),
	}
	var blockContent *model.BlockContentOfDataview
	if dvSchema != nil {
		blockContent = &dvContent
	}
	if blockContent != nil {
		for i, view := range blockContent.Dataview.Views {
			if view.Relations == nil {
				blockContent.Dataview.Views[i].Relations = editor.GetDefaultViewRelations(blockContent.Dataview.Relations)
			}
		}
		tmpls = append(tmpls,
			template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList(blockContent.Dataview.Source)),
			template.WithDataview(*blockContent, false),
		)
	}

	if err = template.InitTemplate(newState, tmpls...); err != nil {
		return "", nil, err
	}

	// TODO: here can be a deadlock if this is somehow created from workspace (as set)
	return c.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeSet, req.Details, nil, newState)
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectInWorkspace(details *types.Struct, workspaceId string) (id string, newDetails *types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = block.Do(c.blockPicker, workspaceId, func(ws *editor.Workspaces) error {
		id, newDetails, err = ws.CreateSubObject(details)
		return err
	})
	return
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectsInWorkspace(details []*types.Struct) (ids []string, objects []*types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = block.Do(c.blockPicker, c.anytype.PredefinedBlocks().Account, func(b smartblock.SmartBlock) error {
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
		}
		ids, objects, err = workspace.CreateSubObjects(details)
		return err
	})
	return
}

// ObjectCreateBookmark creates a new Bookmark object for provided URL or returns id of existing one
func (c *Creator) ObjectCreateBookmark(req *pb.RpcObjectCreateBookmarkRequest) (objectId string, newDetails *types.Struct, err error) {
	u, err := uri.ProcessURI(pbtypes.GetString(req.Details, bundle.RelationKeySource.String()))
	if err != nil {
		return "", nil, fmt.Errorf("process uri: %w", err)
	}
	res := c.bookmark.FetchBookmarkContent(u)
	return c.bookmark.CreateBookmarkObject(req.Details, res)
}

func (c *Creator) CreateObject(req block.DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
	details = req.GetDetails()
	if details.GetFields() == nil {
		details = &types.Struct{Fields: map[string]*types.Value{}}
	}

	var internalFlags []*model.InternalFlag
	if v, ok := req.(block.InternalFlagsGetter); ok {
		internalFlags = v.GetInternalFlags()
		details = internalflag.PutToDetails(details, internalFlags)
	}

	objectType, _ := bundle.TypeKeyFromUrl(pbtypes.GetString(details, bundle.RelationKeyType.String()))
	if forcedType != "" {
		objectType = forcedType
		details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(objectType.URL())
	}
	var sbType = coresb.SmartBlockTypePage

	switch objectType {
	case bundle.TypeKeyBookmark:
		return c.ObjectCreateBookmark(&pb.RpcObjectCreateBookmarkRequest{
			Details: details,
		})
	case bundle.TypeKeySet:
		return c.CreateSet(&pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: internalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
		})
	case bundle.TypeKeyObjectType:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelation:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelationOption:
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relationOption))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyTemplate:
		sbType = coresb.SmartBlockTypeTemplate
	}

	var templateId string
	if v, ok := req.(block.TemplateIdGetter); ok {
		templateId = v.GetTemplateId()
	}
	return c.CreateSmartBlockFromTemplate(context.TODO(), sbType, details, nil, templateId)
}
