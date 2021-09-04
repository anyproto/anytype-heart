package addr

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"strings"
)

const (
	BundledRelationURLPrefix   = "_br"
	BundledObjectTypeURLPrefix = "_ot"
	CustomRelationURLPrefix    = "_ir"

	AnytypeProfileId = "_anytype_profile"
	VirtualPrefix    = "_virtual"

	OldCustomObjectTypeURLPrefix  = "https://anytype.io/schemas/object/custom/"
	OldBundledObjectTypeURLPrefix = "https://anytype.io/schemas/object/bundled/"
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
