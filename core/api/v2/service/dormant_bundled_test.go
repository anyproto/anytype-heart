package v2service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestV2OwnPropertyOnAnInstalledBuiltInKey covers a space that made its own
// `priority` while the built-in was not installed, and later got the built-in
// too (a library type, an older peer): the key then means two properties, and
// an object's type says which.
func TestV2OwnPropertyOnAnInstalledBuiltInKey(t *testing.T) {
	const ownKey = "6a7663db61fab21cd4b9e301"
	newSpace := func(t *testing.T) *v2Fixture {
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-own-priority"),
			bundle.RelationKeyRelationKey:    domain.String(ownKey),
			bundle.RelationKeyApiObjectKey:   domain.String("priority"),
			bundle.RelationKeyName:           domain.String("Priority"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-priority"),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
			bundle.RelationKeyName:           domain.String("Priority"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-feature"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-feature"),
			bundle.RelationKeyName:                 domain.String("Feature"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-own-priority"}),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                   domain.String("type-legacy-task"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-legacy_task"),
			bundle.RelationKeyName:                 domain.String("Legacy task"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-priority"}),
		})
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-note"),
			bundle.RelationKeyUniqueKey: domain.String("ot-plain"),
			bundle.RelationKeyName:      domain.String("Plain"),
		})
		return fx
	}
	storedPropertyKeys := func(t *testing.T, body []byte) []string {
		var doc struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(body, &doc))
		return sortedKeys(doc.Properties)
	}

	for _, tc := range []struct {
		typeKey string
		want    string
	}{
		{"feature", ownKey},
		{"legacy_task", "priority"},
	} {
		t.Run("an object of "+tc.typeKey+" writes the property its type lists", func(t *testing.T) {
			// given
			fx := newSpace(t)

			// when
			body, _, err := fx.canonicalizeDocumentKeys(testSpaceId,
				[]byte(`{"type":"`+tc.typeKey+`","properties":{"priority":"High"}}`))

			// then
			require.NoError(t, err)
			assert.Equal(t, []string{tc.want}, storedPropertyKeys(t, body))
		})
	}

	t.Run("a type listing neither leaves the key ambiguous", func(t *testing.T) {
		// given
		fx := newSpace(t)

		// when
		_, _, err := fx.canonicalizeDocumentKeys(testSpaceId, []byte(`{"type":"plain","properties":{"priority":"High"}}`))

		// then
		var apiErr *v2model.Error
		require.ErrorAs(t, err, &apiErr)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, ownKey)
		assert.Contains(t, apiErr.Issues[0].Message, "the bundled \"Priority\"")
	})

	t.Run("a search over the type reads the property its type lists", func(t *testing.T) {
		// given
		fx := newSpace(t)
		kc, err := fx.newKeyCanon(testSpaceId)
		require.NoError(t, err)
		kc.scope = fx.typePropertyScope(testSpaceId, "type-feature")

		// when
		got, ambiguous := kc.canon("priority")

		// then
		assert.Empty(t, ambiguous)
		assert.Equal(t, ownKey, got)
	})
}
