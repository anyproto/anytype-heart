package v2service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestPropertyNotFoundInventoryDoesNotOverpromisePublicList(t *testing.T) {
	fx := newV2Fixture(t)
	properties := make([]objectstore.TestObject, 0, maxListedKeys+1)
	for i := 0; i <= maxListedKeys; i++ {
		key := fmt.Sprintf("repair_property_%02d", i)
		property := objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-" + key),
			bundle.RelationKeyRelationKey:    domain.String(key),
			bundle.RelationKeyName:           domain.String(key),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		}
		if i == 0 {
			property[bundle.RelationKeyIsHidden] = domain.Bool(true)
		}
		properties = append(properties, property)
	}
	fx.objectStore.AddObjects(t, testSpaceId, properties)

	apiErr := v2Err(t, fx.propertyNotFoundError(testSpaceId, "!!!!!!!!", errKeys{}))
	assert.Contains(t, apiErr.Message, "known property keys:")
	assert.Contains(t, apiErr.Message, "total above")
	assert.Contains(t, apiErr.Message, "hidden addressable properties are excluded")
	assert.NotContains(t, apiErr.Message, "list all")

	closeMatch := v2Err(t, fx.propertyNotFoundError(testSpaceId, "repair_property_0x", errKeys{}))
	assert.Contains(t, closeMatch.Message, "did you mean repair_property_")
	assert.NotContains(t, closeMatch.Message, "hidden addressable properties", "a concrete suggestion wins over the fallback")
}
