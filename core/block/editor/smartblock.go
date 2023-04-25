package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewUninitialized(sbType model.SmartBlockType) smartblock.SmartBlock {
	switch sbType {
	case model.SmartBlockType_Page, model.SmartBlockType_Date:
		return &Page{}
	case model.SmartBlockType_Archive:
		return &Archive{}
	case model.SmartBlockType_Home:
		return &Dashboard{}
	case model.SmartBlockType_Set:
		return &Set{}
	case model.SmartBlockType_ProfilePage, model.SmartBlockType_AnytypeProfile:
		return &Profile{}
	case model.SmartBlockType_STObjectType,
		model.SmartBlockType_BundledObjectType:
		return &ObjectType{}
	case model.SmartBlockType_BundledRelation:
		return &Set{}
	case model.SmartBlockType_SubObject:
		return &SubObject{}
	case model.SmartBlockType_File:
		return &Files{}
	case model.SmartBlockType_MarketplaceType:
		return &MarketplaceType{}
	case model.SmartBlockType_MarketplaceRelation:
		return &MarketplaceRelation{}
	case model.SmartBlockType_MarketplaceTemplate:
		return &MarketplaceTemplate{}
	case model.SmartBlockType_Template:
		return &Template{}
	case model.SmartBlockType_BundledTemplate:
		return &Template{}
	case model.SmartBlockType_Breadcrumbs:
		return &Breadcrumbs{}
	case model.SmartBlockType_Workspace:
		return &Workspaces{}
	case model.SmartBlockType_Widget:
		return &WidgetObject{}
	default:
		panic(fmt.Errorf("unexpected smartblock type: %v", sbType))
	}
}
