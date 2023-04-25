package object

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"

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
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/uri"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Creator struct {
	blockService      BlockService
	blockPicker       block.Picker
	objectStore       objectstore.ObjectStore
	collectionService CollectionService
	bookmark          bookmark.Service
	objectFactory     *editor.ObjectFactory
	app               *app.App
	sbtProvider       typeprovider.SmartBlockTypeProvider

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
	c.objectFactory = app.MustComponent[*editor.ObjectFactory](a)
	c.collectionService = app.MustComponent[CollectionService](a)
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
	CreateTreeObject(ctx context.Context, tp coresb.SmartBlockType, initFunc block.InitFunc) (sb smartblock.SmartBlock, err error)
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

// CreateSmartBlockFromState create new object from the provided `createState` and `details`. If you pass `details` into the function, it will automatically add missing relationLinks and override the details from the `createState`
// It will return error if some of the relation keys in `details` not installed in the workspace.
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

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Since(startTime).Milliseconds(),
	}
	ctx = context.WithValue(ctx, eventCreate, ev)
	if sbType == coresb.SmartBlockTypeSubObject {
		return c.CreateSubObjectInWorkspace(createState.CombinedDetails(), workspaceID)
	}

	sb, err := c.blockService.CreateTreeObject(ctx, sbType, func(id string) *smartblock.InitContext {
		createState.SetRootId(id)
		createState.SetObjectTypes(objectTypes)
		createState.InjectDerivedDetails()

		return &smartblock.InitContext{
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

func (c *Creator) CreateSet(req *pb.RpcObjectCreateSetRequest) (setID string, newDetails *types.Struct, err error) {
	req.Details = internalflag.PutToDetails(req.Details, req.InternalFlags)

	// TODO remove it, when schema will be refactored
	source := req.Source
	var dvContent model.BlockContentOfDataview
	var dvSchema schema.Schema
	if len(source) == 0 {
		source = []string{bundle.TypeKeyPage.URL()}
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
		req.Details.Fields[bundle.RelationKeySource.String()] = pbtypes.StringList(req.Source)
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
	return c.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypePage, req.Details, newState)
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
	case bundle.TypeKeyBookmark.URL():
		return c.ObjectCreateBookmark(&pb.RpcObjectCreateBookmarkRequest{
			Details: details,
		})
	case bundle.TypeKeySet.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_set))
		return c.CreateSet(&pb.RpcObjectCreateSetRequest{
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
		return c.CreateSmartBlockFromState(context.TODO(), sbType, details, st)
	case bundle.TypeKeyObjectType.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelation.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relation))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyRelationOption.URL():
		details.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_relationOption))
		return c.CreateSubObjectInWorkspace(details, c.anytype.PredefinedBlocks().Account)

	case bundle.TypeKeyTemplate.URL():
		sbType = coresb.SmartBlockTypeTemplate
	}

	return c.CreateSmartBlockFromTemplate(context.TODO(), sbType, details, templateID)
}
