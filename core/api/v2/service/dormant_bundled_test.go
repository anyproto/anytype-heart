package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
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

// TestV2OwnPropertyOnADormantBuiltInKey covers a space that made its own
// `priority` while the built-in was never installed: every channel that names
// the key reaches the space's own property, never the bundled one.
func TestV2OwnPropertyOnADormantBuiltInKey(t *testing.T) {
	const ownKey = "6a7663db61fab21cd4b9e302"
	newSpace := func(t *testing.T, format model.RelationFormat) *v2Fixture {
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-own-priority"),
			bundle.RelationKeyRelationKey:    domain.String(ownKey),
			bundle.RelationKeyApiObjectKey:   domain.String("priority"),
			bundle.RelationKeyName:           domain.String("Priority"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(format)),
		})
		return fx
	}

	t.Run("a property block binds the space's own property", func(t *testing.T) {
		// given
		fx := newSpace(t, model.RelationFormat_status)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
			patchBody(`{"op":"insert_blocks","blocks":[{"type":"property","property":"priority"}]}`), "", false, true)

		// then
		require.NoError(t, err)
		var keys []string
		for _, b := range docBlocks(stateDoc(t, *captured)) {
			if b["type"] == "property" {
				key, _ := b["property"].(string)
				keys = append(keys, key)
			}
		}
		assert.Equal(t, []string{ownKey}, keys)
	})

	t.Run("a filter reads the format of the space's own property", func(t *testing.T) {
		// given: the bundled priority is a number; the space's own is a date
		fx := newSpace(t, model.RelationFormat_date)
		kc, err := fx.newKeyCanon(testSpaceId)
		require.NoError(t, err)
		formatName := canonFormatName(fx.formatNameResolver(testSpaceId), kc)

		// when
		got, ok := formatName("priority")

		// then
		require.True(t, ok)
		assert.Equal(t, anyblockjson.FormatName(model.RelationFormat_date), got)
	})

	t.Run("a view filter stores a date for the space's own date property", func(t *testing.T) {
		// given
		fx := newSpace(t, model.RelationFormat_date)
		committed := fx.expectMutateState(editRead(t, editSetDoc), nil)

		// when
		_, err := fx.PatchObject(context.Background(), testSpaceId, "obj1",
			patchBody(`{"op":"update_view","set":{"filters":[{"property":"priority","condition":"greater_or_equal","value":"2026-07-01"}]}}`), "", false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *committed)
		dv := (*committed).Pick(state.DataviewBlockID).Model().GetDataview()
		require.Len(t, dv.Views, 1)
		require.Len(t, dv.Views[0].Filters, 1)
		assert.Equal(t, ownKey, dv.Views[0].Filters[0].RelationKey)
		assert.Equal(t, float64(1782864000), dv.Views[0].Filters[0].Value.GetNumberValue(), "2026-07-01T00:00:00Z")
	})
}
