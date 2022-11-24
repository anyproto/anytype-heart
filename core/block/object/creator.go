package object

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

var log = logging.Logger("object-service")

type eventKey int

const eventCreate eventKey = 0

type Creator struct {
	blockService BlockService
	blockPicker  block.BlockPicker

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
	return nil
}

func (c *Creator) Name() (name string) {
	return "object-creator"
}

// TODO Temporarily
type BlockService interface {
	NewSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error)
	StateFromTemplate(templateId, name string) (st *state.State, err error)
}

func (c *Creator) CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string) (id string, newDetails *types.Struct, err error) {
	return c.CreateSmartBlockFromState(ctx, sbType, details, relationIds, state.NewDoc("", nil).NewState())
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
	// TODO: looks like I can move all araound code into some component and use it under DoWithContext:
	/*
		Do(func(c editor.Creator) {
		   c.CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State)
		}

	*/
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
