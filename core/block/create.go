package block

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func (s *Service) CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string) (id string, newDetails *types.Struct, err error) {
	return s.CreateSmartBlockFromState(ctx, sbType, details, relationIds, state.NewDoc("", nil).NewState())
}

func (s *Service) CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, templateId string) (id string, newDetails *types.Struct, err error) {
	var createState *state.State
	if templateId != "" {
		if createState, err = s.stateFromTemplate(templateId, pbtypes.GetString(details, bundle.RelationKeyName.String())); err != nil {
			return
		}
	} else {
		createState = state.NewDoc("", nil).NewState()
	}
	return s.CreateSmartBlockFromState(ctx, sbType, details, relationIds, createState)
}

func (s *Service) CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error) {
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
		workspaceId = s.anytype.PredefinedBlocks().Account
	}

	if workspaceId != "" {
		createState.SetDetailAndBundledRelation(bundle.RelationKeyWorkspaceId, pbtypes.String(workspaceId))
	}
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreatedDate, pbtypes.Int64(time.Now().Unix()))
	createState.SetDetailAndBundledRelation(bundle.RelationKeyCreator, pbtypes.String(s.anytype.ProfileID()))

	ev := &metrics.CreateObjectEvent{
		SetDetailsMs: time.Now().Sub(startTime).Milliseconds(),
	}
	ctx = context.WithValue(ctx, ObjectCreateEvent, ev)
	var tid = thread.Undef
	if id := pbtypes.GetString(createState.CombinedDetails(), bundle.RelationKeyId.String()); id != "" {
		tid, err = thread.Decode(id)
		if err != nil {
			log.Errorf("failed to decode thread id from the state: %s", err.Error())
		}
	}

	csm, err := s.CreateObjectInWorkspace(ctx, workspaceId, tid, sbType)
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
	if sb, err = s.newSmartBlock(id, initCtx); err != nil {
		return id, nil, err
	}
	ev.SmartblockCreateMs = time.Now().Sub(startTime).Milliseconds() - ev.SetDetailsMs - ev.WorkspaceCreateMs - ev.GetWorkspaceBlockWaitMs
	ev.SmartblockType = int(sbType)
	ev.ObjectId = id
	metrics.SharedClient.RecordEvent(*ev)
	return id, sb.CombinedDetails(), sb.Close()
}

func (s *Service) CreateObjectFromState(ctx *session.Context, contextBlock smartblock.SmartBlock, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest, state *state.State) (linkId string, objectId string, err error) {
	return s.createObject(ctx, contextBlock, groupId, req, false, func(ctx context.Context) (string, error) {
		objectId, _, err = s.CreateSmartBlockFromState(ctx, coresb.SmartBlockTypePage, req.Details, nil, state)
		if err != nil {
			return objectId, fmt.Errorf("create smartblock error: %v", err)
		}
		return objectId, nil
	})
}

func (s *Service) createObject(ctx *session.Context, contextBlock smartblock.SmartBlock, groupId string, req pb.RpcBlockLinkCreateWithObjectRequest, storeLink bool, create func(context.Context) (objectId string, err error)) (linkId string, objectId string, err error) {
	if contextBlock != nil {
		if contextBlock.Type() == model.SmartBlockType_Set {
			return "", "", basic.ErrNotSupported
		}
	}
	workspaceId, err := s.anytype.GetWorkspaceIdForObject(req.ContextId)
	if err != nil {
		workspaceId = ""
	}
	if workspaceId != "" && req.Details != nil {
		threads.WorkspaceLogger.
			With("workspace id", workspaceId).
			Debug("adding workspace id to new object")
		if req.Details.Fields == nil {
			req.Details.Fields = make(map[string]*types.Value)
		}
		req.Details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceId)
	}

	objectId, err = create(context.TODO())
	if err != nil {
		err = fmt.Errorf("create smartblock error: %v", err)
	}

	// do not create a link
	if (!storeLink) || contextBlock == nil {
		return "", objectId, err
	}

	st := contextBlock.NewStateCtx(ctx).SetGroupId(groupId)
	b, ok := contextBlock.(basic.Creatable)
	if !ok {
		err = fmt.Errorf("%T doesn't implement basic.Basic", contextBlock)
		return
	}
	linkId, err = b.CreateBlock(st, pb.RpcBlockCreateRequest{
		TargetId: req.TargetId,
		Block: &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: objectId,
					Style:         model.BlockContentLink_Page,
				},
			},
			Fields: req.Fields,
		},
		Position: req.Position,
	})
	if err != nil {
		err = fmt.Errorf("link create error: %v", err)
	}
	err = contextBlock.Apply(st)
	return
}

// todo: rewrite with options
// withId may me empty
func (s *Service) CreateObjectInWorkspace(
	ctx context.Context,
	workspaceId string,
	withId thread.ID,
	sbType coresb.SmartBlockType,
) (csm core.SmartBlock, err error) {
	startTime := time.Now()
	ev, exists := ctx.Value(ObjectCreateEvent).(*metrics.CreateObjectEvent)
	err = s.DoWithContext(ctx, workspaceId, func(b smartblock.SmartBlock) error {
		if exists {
			ev.GetWorkspaceBlockWaitMs = time.Now().Sub(startTime).Milliseconds()
		}
		workspace, ok := b.(*editor.Workspaces)
		if !ok {
			return fmt.Errorf("incorrect object with workspace id")
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

// TODO move it to smarblock package? But first figure out how to pass necessary dependencies
func (s *Service) newSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := s.source.NewSource(id, false)
	if err != nil {
		return
	}
	switch sc.Type() {
	case model.SmartBlockType_Page, model.SmartBlockType_Date:
		sb = editor.NewPage(s, s, s, s.bookmark)
	case model.SmartBlockType_Archive:
		sb = editor.NewArchive(s)
	case model.SmartBlockType_Home:
		sb = editor.NewDashboard(s, s)
	case model.SmartBlockType_Set:
		sb = editor.NewSet()
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		sb = editor.NewProfile(s, s, s.bookmark, s.sendEvent)
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		sb = editor.NewObjectType()
	case model.SmartBlockType_BundledRelation:
		sb = editor.NewSet()
	case model.SmartBlockType_SubObject:
		sb = editor.NewSubObject()
	case model.SmartBlockType_File:
		sb = editor.NewFiles()
	case model.SmartBlockType_MarketplaceType:
		sb = editor.NewMarketplaceType()
	case model.SmartBlockType_MarketplaceRelation:
		sb = editor.NewMarketplaceRelation()
	case model.SmartBlockType_MarketplaceTemplate:
		sb = editor.NewMarketplaceTemplate()
	case model.SmartBlockType_Template:
		sb = editor.NewTemplate(s, s, s, s.bookmark)
	case model.SmartBlockType_BundledTemplate:
		sb = editor.NewTemplate(s, s, s, s.bookmark)
	case model.SmartBlockType_Breadcrumbs:
		sb = editor.NewBreadcrumbs()
	case model.SmartBlockType_Workspace:
		sb = editor.NewWorkspace(s)
	case model.SmartBlockType_AccountOld:
		sb = editor.NewThreadDB(s)
	case model.SmartBlockType_Widget:
		sb = editor.NewWidgetObject()
	default:
		return nil, fmt.Errorf("unexpected smartblock type: %v", sc.Type())
	}

	sb.Lock()
	defer sb.Unlock()
	if initCtx == nil {
		initCtx = &smartblock.InitContext{}
	}
	initCtx.App = s.app
	initCtx.Source = sc
	err = sb.Init(initCtx)
	return
}
