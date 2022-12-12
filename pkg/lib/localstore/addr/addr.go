package addr

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"strings"
)

const (
	SubObjectCollectionIdSeparator = "-"
	RelationKeyToIdPrefix          = "rel-" //
	ObjectTypeKeyToIdPrefix        = "ot-"  //

	BundledRelationURLPrefix    = "_br"
	OldIndexedRelationURLPrefix = "_ir"

	BundledObjectTypeURLPrefix = "_ot"

	AnytypeProfileId            = "_anytype_profile"
	AnytypeMarketplaceWorkspace = "_anytype_marketplace"
	VirtualPrefix               = "_virtual"
	DatePrefix                  = "_date_"
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

// returns the
func GetVirtualCollectionObjectId(collectionName, key string) string {
	return collectionName + SubObjectCollectionIdSeparator + key
}

func ConvertBundledObjectIdToInstalledId(bundledId string) (string, error) {
	if strings.HasPrefix(bundledId, BundledRelationURLPrefix) {
		return RelationKeyToIdPrefix + strings.TrimPrefix(bundledId, BundledRelationURLPrefix), nil
	} else if strings.HasPrefix(bundledId, BundledObjectTypeURLPrefix) {
		return ObjectTypeKeyToIdPrefix + strings.TrimPrefix(bundledId, BundledObjectTypeURLPrefix), nil
	}

	return "", fmt.Errorf("unknown bundled id")
}
