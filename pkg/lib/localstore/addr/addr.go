package addr

import (
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	SubObjectCollectionIdSeparator = "-"
	RelationKeyToIdPrefix          = "rel-" //
	ObjectTypeKeyToIdPrefix        = "ot-"  //
	ObjectTypeAllViewId            = "all"
	ObjectTypeAllTableViewId       = "table" // used for types created during import and ai-onboarding

	BundledRelationURLPrefix = "_br"

	BundledObjectTypeURLPrefix = "_ot"
	BundledTemplatesURLPrefix  = "_bt"

	AnytypeProfileId            = "_anytype_profile"
	AnytypeMarketplaceWorkspace = "_anytype_marketplace"
	VirtualPrefix               = "_virtual"
	DatePrefix                  = "_date_"

	MissingObject = "_missing_object"
)

func ExtractVirtualSourceType(id string) (model.SmartBlockType, error) {
	if !strings.HasPrefix(id, VirtualPrefix) {
		return 0, fmt.Errorf("invalid id: prefix not found")
	}

	trimmedId := strings.TrimPrefix(id, VirtualPrefix)
	delimPos := strings.LastIndex(trimmedId, "_")
	if delimPos == -1 {
		return 0, fmt.Errorf("invalid id: type delimiter not found")
	}

	sbTypeName := trimmedId[:delimPos]

	if v, exists := model.SmartBlockType_value[sbTypeName]; exists {
		return model.SmartBlockType(v), nil
	}
	return 0, fmt.Errorf("sb type '%s' not found", sbTypeName)
}
