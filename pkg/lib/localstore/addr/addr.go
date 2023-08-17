package addr

import (
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	SubObjectCollectionIdSeparator = "-"
	RelationKeyToIdPrefix          = "rel-" //
	ObjectTypeKeyToIdPrefix        = "ot-"  //

	BundledRelationURLPrefix = "_br"

	BundledObjectTypeURLPrefix = "_ot"
	BundledTemplatesURLPrefix  = "_bt"

	AnytypeProfileId            = "_anytype_profile"
	AnytypeMarketplaceWorkspace = "_anytype_marketplace"
	VirtualPrefix               = "_virtual"
	DatePrefix                  = "_date_"

	MissingObject = "_missing_object"
)

func IsBundledId(id string) bool {
	return strings.HasPrefix(id, BundledRelationURLPrefix) ||
		strings.HasPrefix(id, BundledObjectTypeURLPrefix) ||
		strings.HasPrefix(id, BundledTemplatesURLPrefix) ||
		strings.HasPrefix(id, DatePrefix) ||
		id == AnytypeMarketplaceWorkspace ||
		id == MissingObject ||
		id == AnytypeProfileId
}

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

func TimeToID(t time.Time) string {
	return DatePrefix + t.Format("2006-01-02")
}
