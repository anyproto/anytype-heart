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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
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
	blockService  BlockService
	blockPicker   block.Picker
	objectStore   objectstore.ObjectStore
	bookmark      bookmark.Service
	objectFactory *editor.ObjectFactory
	app           *app.App

	// TODO: remove it?
	anytype core.Service
}

func NewCreator() *Creator {
	return &Creator{}
}

func (c *Creator) Init(a *app.App) (err error) {
	c.anytype = a.MustComponent(core.CName).(core.Service)
	c.blockService = a.MustComponent(block.CName).(BlockService)
	c.blockPicker = a.MustComponent(block.CName).(block.Picker)
	c.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	c.bookmark = a.MustComponent(bookmark.CName).(bookmark.Service)
	c.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	c.app = a
	return nil
}

const CName = "objectCreator"

func (c *Creator) Name() (name string) {
	return CName
}

// TODO Temporarily
type BlockService interface {
	StateFromTemplate(templateID, name string) (st *state.State, err error)
}

func (c *Creator) CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, templateID string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateID != "" {
		if createState, err = c.blockService.StateFromTemplate(templateID, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return c.CreateSmartBlockFromState(ctx, sbType, details, createState)
}

func (c *Creator) CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, createState *state.State) (id string, newDetails *types.Struct, err error) {
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
			if _, err2 := c.objectStore.GetRelationById(relId); err != nil {
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

	var tid = thread.Undef
	id = pbtypes.GetString(createState.CombinedDetails(), bundle.RelationKeyId.String())
	sbt, _ := coresb.SmartBlockTypeFromID(id)
	if sbt == coresb.SmartBlockTypeSubObject {
		return c.CreateSubObjectInWorkspace(createState.CombinedDetails(), workspaceID)
	} else if id != "" {
		tid, err = thread.Decode(id)
		if err != nil {
			log.Errorf("failed to decode thread id from the state: %s", err.Error())
		}
	}

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Since(startTime).Milliseconds(),
	}
	ctx = context.WithValue(ctx, eventCreate, ev)

	csm, err := c.CreateObjectInWorkspace(ctx, workspaceID, tid, sbType)
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
		RelationKeys:   relationKeys,
	}
	var sb smartblock.SmartBlock

	if sb, err = c.objectFactory.InitObject(id, initCtx); err != nil {
		return id, nil, err
	}
	ev.SmartblockCreateMs = time.Since(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, sb.CombinedDetails(), sb.Close()
}

// todo: rewrite with options
// withId may me empty
func (c *Creator) CreateObjectInWorkspace(ctx context.Context, workspaceID string, withID thread.ID, sbType coresb.SmartBlockType) (csm core.SmartBlock, err error) {
	startTime := time.Now()
	ev, exists := ctx.Value(eventCreate).(*metrics.CreateObjectEvent)
	err = block.DoWithContext(ctx, c.blockPicker, workspaceID, func(workspace *editor.Workspaces) error {
		if exists {
			ev.GetWorkspaceBlockWaitMs = time.Since(startTime).Milliseconds()
		}
		csm, err = workspace.CreateObject(withID, sbType)
		if exists {
			ev.WorkspaceCreateMs = time.Since(startTime).Milliseconds() - ev.GetWorkspaceBlockWaitMs
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

func (c *Creator) InjectWorkspaceID(details *types.Struct, objectID string) {
	workspaceID, err := c.anytype.GetWorkspaceIdForObject(objectID)
	if err != nil {
		workspaceID = ""
	}
	if workspaceID == "" || details == nil {
		return
	}
	threads.WorkspaceLogger.
		With("workspace id", workspaceID).
		Debug("adding workspace id to new object")
	if details.Fields == nil {
		details.Fields = make(map[string]*types.Value)
	}
	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceID)
}

func (c *Creator) CreateSet(req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema

	// TODO remove it, when schema will be refactored
	source := req.Source
	if len(source) == 0 {
		source = []string{bundle.TypeKeyPage.URL()}
	}
	if dvContent, dvSchema, err = dataview.DataviewBlockBySource(c.objectStore, source); err != nil {
		return
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
			template.WithDataview(*blockContent, false),
		)
		if len(req.Source) > 0 {
			tmpls = append(tmpls,
				template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList(req.Source)),
			)
		}
	}

	if err = template.InitTemplate(newState, tmpls...); err != nil {
		return "", nil, err
	}

	// TODO: here can be a deadlock if this is somehow created from workspace (as set)
	return c.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeSet, nil, newState)
}

// TODO: it must be in another component
func (c *Creator) CreateSubObjectInWorkspace(details *types.Struct, workspaceID string) (id string, newDetails *types.Struct, err error) {
	// todo: rewrite to the current workspace id
	err = block.Do(c.blockPicker, workspaceID, func(ws *editor.Workspaces) error {
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
func (c *Creator) ObjectCreateBookmark(req *pb.RpcObjectCreateBookmarkRequest) (objectID string, newDetails *types.Struct, err error) {
	u, err := uri.NormalizeURI(pbtypes.GetString(req.Details, bundle.RelationKeySource.String()))
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
	case bundle.TypeKeyBookmark.String():
		return c.ObjectCreateBookmark(&pb.RpcObjectCreateBookmarkRequest{
			Details: details,
		})
	case bundle.TypeKeySet.String():
		return c.CreateSet(&pb.RpcObjectCreateSetRequest{
			Details:       details,
			InternalFlags: internalFlags,
			Source:        pbtypes.GetStringList(details, bundle.RelationKeySetOf.String()),
		})
	case bundle.TypeKeyObjectType.String():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelation.String():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelationOption.String():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relationOption))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyTemplate.String():
		sbType = coresb.SmartBlockTypeTemplate
	}

	return c.CreateSmartBlockFromTemplate(context.TODO(), sbType, details, templateID)
}
