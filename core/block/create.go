package block

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// TODO temporarily. Remove this and just use CreateSmartBlockFromState
func (s *Service) CreateSmartBlock(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string) (id string, newDetails *types.Struct, err error) {
	return s.objectCreator.CreateSmartBlock(ctx, sbType, details, relationIds)
}

func (s *Service) CreateSmartBlockFromState(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, createState *state.State) (id string, newDetails *types.Struct, err error) {
	return s.objectCreator.CreateSmartBlockFromState(ctx, sbType, details, relationIds, createState)
}

func (s *Service) CreateSmartBlockFromTemplate(ctx context.Context, sbType coresb.SmartBlockType, details *types.Struct, relationIds []string, templateId string) (id string, newDetails *types.Struct, err error) {
	return s.objectCreator.CreateSmartBlockFromTemplate(ctx, sbType, details, relationIds, templateId)
}

// TODO move it to smarblock package? But first figure out how to pass necessary dependencies
func (s *Service) NewSmartBlock(id string, initCtx *smartblock.InitContext) (sb smartblock.SmartBlock, err error) {
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
