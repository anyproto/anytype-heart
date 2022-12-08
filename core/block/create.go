package block

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func (s *Service) TemplateCreateFromObject(id string) (templateID string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_Page {
			return fmt.Errorf("can't make template from this obect type")
		}
		st, err = b.TemplateCreateFromObjectState()
		return err
	}); err != nil {
		return
	}

	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateClone(id string) (templateID string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if b.Type() != model.SmartBlockType_BundledTemplate {
			return fmt.Errorf("can clone bundled templates only")
		}
		st = b.NewState().Copy()
		st.RemoveDetail(bundle.RelationKeyTemplateIsBundled.String())
		st.SetLocalDetails(nil)
		t := st.ObjectTypes()
		t, _ = relationutils.MigrateObjectTypeIds(t)
		st.SetObjectTypes(t)
		targetObjectType, _ := relationutils.MigrateObjectTypeId(pbtypes.GetString(st.Details(), bundle.RelationKeyTargetObjectType.String()))
		st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(targetObjectType))
		return nil
	}); err != nil {
		return
	}
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) ObjectDuplicate(id string) (objectID string, err error) {
	var st *state.State
	if err = s.Do(id, func(b smartblock.SmartBlock) error {
		if err = b.Restrictions().Object.Check(model.Restrictions_Duplicate); err != nil {
			return err
		}
		st = b.NewState().Copy()
		st.SetLocalDetails(nil)
		return nil
	}); err != nil {
		return
	}

	sbt, err := coresb.SmartBlockTypeFromID(id)
	if err != nil {
		return
	}

	objectID, _, err = s.objectCreator.CreateSmartBlockFromState(context.TODO(), sbt, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) TemplateCreateFromObjectByObjectType(otID string) (templateID string, err error) {
	if err = s.Do(otID, func(_ smartblock.SmartBlock) error { return nil }); err != nil {
		return "", fmt.Errorf("can't open objectType: %v", err)
	}
	var st = state.NewDoc("", nil).(*state.State)
	st.SetDetail(bundle.RelationKeyTargetObjectType.String(), pbtypes.String(otID))
	st.SetObjectTypes([]string{bundle.TypeKeyTemplate.URL(), otID})
	templateID, _, err = s.objectCreator.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeTemplate, nil, nil, st)
	if err != nil {
		return
	}
	return
}

func (s *Service) CreateWorkspace(req *pb.RpcWorkspaceCreateRequest) (workspaceID string, err error) {
	id, _, err := s.objectCreator.CreateSmartBlockFromState(context.TODO(), coresb.SmartBlockTypeWorkspace,
		&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():      pbtypes.String(req.Name),
			bundle.RelationKeyType.String():      pbtypes.String(bundle.TypeKeySpace.URL()),
			bundle.RelationKeyIconEmoji.String(): pbtypes.String("🌎"),
			bundle.RelationKeyLayout.String():    pbtypes.Float64(float64(model.ObjectType_space)),
		}}, nil, nil)
	return id, err
}

// CreateLinkToTheNewObject creates an object and stores the link to it in the context block
func (s *Service) CreateLinkToTheNewObject(ctx *session.Context, req *pb.RpcBlockLinkCreateWithObjectRequest) (linkID string, objectID string, err error) {
	if req.ContextId == req.TemplateId && req.ContextId != "" {
		err = fmt.Errorf("unable to create link to template from this template")
		return
	}

	s.objectCreator.InjectWorkspaceID(req.Details, req.ContextId)
	objectID, _, err = s.CreateObject(req, "")

	if req.ContextId == "" {
		return
	}

	err = DoStateCtx(s, ctx, req.ContextId, func(st *state.State, sb basic.Creatable) error {
		linkID, err = sb.CreateBlock(st, pb.RpcBlockCreateRequest{
			TargetId: req.TargetId,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: objectID,
						Style:         model.BlockContentLink_Page,
					},
				},
				Fields: req.Fields,
			},
			Position: req.Position,
		})
		if err != nil {
			return fmt.Errorf("link create error: %v", err)
		}
		return nil
	})
	return
}

func (s *Service) ObjectToSet(id string, source []string) (string, error) {
	var details *types.Struct
	if err := s.Do(id, func(b smartblock.SmartBlock) error {
		details = pbtypes.CopyStruct(b.Details())

		s := b.NewState()
		if layout, ok := s.Layout(); ok && layout == model.ObjectType_note {
			textBlock, err := s.GetFirstTextBlock()
			if err != nil {
				return err
			}
			if textBlock != nil {
				details.Fields[bundle.RelationKeyName.String()] = pbtypes.String(textBlock.Text.Text)
			}
		}

		return nil
	}); err != nil {
		return "", err
	}

	details.Fields[bundle.RelationKeySetOf.String()] = pbtypes.StringList(source)
	newID, _, err := s.objectCreator.CreateObject(&pb.RpcObjectCreateSetRequest{
		Source:  source,
		Details: details,
	}, bundle.TypeKeySet)
	if err != nil {
		return "", err
	}

	res, err := s.objectStore.GetWithLinksInfoByID(id)
	if err != nil {
		return "", err
	}
	for _, il := range res.Links.Inbound {
		if err = s.replaceLink(il.Id, id, newID); err != nil {
			return "", err
		}
	}
	err = s.DeleteObject(id)
	if err != nil {
		// intentionally do not return error here
		log.Errorf("failed to delete object after conversion to set: %s", err.Error())
	}

	return id, nil
}

func (s *Service) NewSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
	sc, err := s.source.NewSource(id, false)
	if err != nil {
		return
	}
	switch sc.Type() {
	case model.SmartBlockType_Page, model.SmartBlockType_Date:
		sb = editor.NewPage()
	case model.SmartBlockType_Archive:
		sb = editor.NewArchive()
	case model.SmartBlockType_Home:
		sb = editor.NewDashboard()
	case model.SmartBlockType_Set:
		sb = editor.NewSet()
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		sb = editor.NewProfile(s.sendEvent)
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
		sb = editor.NewTemplate()
	case model.SmartBlockType_BundledTemplate:
		sb = editor.NewTemplate()
	case model.SmartBlockType_Breadcrumbs:
		sb = editor.NewBreadcrumbs()
	case model.SmartBlockType_Workspace:
		sb = editor.NewWorkspace()
	case model.SmartBlockType_AccountOld:
		sb = editor.NewThreadDB()
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

func (s *Service) CreateObject(req DetailsGetter, forcedType bundle.TypeKey) (id string, details *types.Struct, err error) {
	return s.objectCreator.CreateObject(req, forcedType)
}
