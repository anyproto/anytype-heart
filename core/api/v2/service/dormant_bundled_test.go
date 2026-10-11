package v2service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
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

// TestV2BlockKeySlotsOnAnInstalledBuiltInKey: the key slots blocks carry — a
// property block, a query's views — settle a key two properties answer to by
// the type they describe, not by the codec's preference for the built-in.
func TestV2BlockKeySlotsOnAnInstalledBuiltInKey(t *testing.T) {
	const ownKey = "6a7663db61fab21cd4b9e304"
	ctx := context.Background()
	newSpace := func(t *testing.T) *v2Fixture {
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-own-priority"),
			bundle.RelationKeyRelationKey:    domain.String(ownKey),
			bundle.RelationKeyApiObjectKey:   domain.String("priority"),
			bundle.RelationKeyName:           domain.String("Rank"),
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
		return fx
	}
	viewSortKeys := func(blocks []*model.Block) []string {
		var keys []string
		for _, b := range blocks {
			for _, view := range b.GetDataview().GetViews() {
				for _, sort := range view.Sorts {
					keys = append(keys, sort.RelationKey)
				}
			}
		}
		return keys
	}

	t.Run("a created object's property block binds the property its type lists", func(t *testing.T) {
		// given
		fx := newSpace(t)
		captured := fx.expectCreate("obj-new")
		fx.expectEtagRead("obj-new")

		// when
		_, err := fx.CreateObject(ctx, testSpaceId, []byte(`{"formatVersion":"2.0","type":"feature",
			"properties":{"name":"Ship it"},"blocks":[{"type":"property","property":"priority"}]}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		var keys []string
		for _, b := range (*captured).Blocks {
			if rel := b.GetRelation(); rel != nil {
				keys = append(keys, rel.Key)
			}
		}
		assert.Equal(t, []string{ownKey}, keys)
	})

	t.Run("a created query's views sort by the property its queried type lists", func(t *testing.T) {
		// given
		fx := newSpace(t)
		captured := fx.expectCreate("obj-new")
		fx.expectEtagRead("obj-new")

		// when
		_, err := fx.CreateObject(ctx, testSpaceId, []byte(`{"formatVersion":"2.0","type":"set",
			"properties":{"name":"Features"},"query_source":{"types":["feature"]},
			"blocks":[{"id":"dataview","type":"dataview","views":[{"id":"view1","name":"All",
				"sorts":[{"property":"priority","direction":"asc"}]}]}]}`), false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *captured)
		assert.Equal(t, []string{ownKey}, viewSortKeys((*captured).Blocks))
	})

	t.Run("an inserted dataview sorts by the property its source type lists", func(t *testing.T) {
		// given
		fx := newSpace(t)
		committed := fx.expectMutateState(editRead(t, editBaseDoc), nil)

		// when
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"insert_blocks","blocks":[{"type":"dataview","source":["feature"],
				"views":[{"name":"All","sorts":[{"property":"priority","direction":"asc"}]}]}]}`), "", false, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, *committed)
		var blocks []*model.Block
		(*committed).Iterate(func(b simple.Block) bool {
			blocks = append(blocks, b.Model())
			return true
		})
		assert.Equal(t, []string{ownKey}, viewSortKeys(blocks))
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

// TestV2TypedSearchSortsOnAnInstalledBuiltInKey: a sort on `due_date` in a
// search over a type listing the space's own due_date must not be rewritten to
// the installed built-in dueDate on the way through the codec.
func TestV2TypedSearchSortsOnAnInstalledBuiltInKey(t *testing.T) {
	// given
	const ownKey = "6a7663db61fab21cd4b9e303"
	fx := slugSpaceFixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-own-due"),
		bundle.RelationKeyRelationKey:    domain.String(ownKey),
		bundle.RelationKeyApiObjectKey:   domain.String("due_date"),
		bundle.RelationKeyName:           domain.String("Ship by"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
	})
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-due"),
		bundle.RelationKeyRelationKey:    domain.String("dueDate"),
		bundle.RelationKeyName:           domain.String("Due date"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_date)),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                   domain.String("type-feature"),
		bundle.RelationKeyUniqueKey:            domain.String("ot-feature"),
		bundle.RelationKeyName:                 domain.String("Feature"),
		bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-own-due"}),
	})

	// when
	plan, err := fx.buildSearchPlan(testSpaceId, v2model.SearchRequest{
		Type:  "feature",
		Sorts: json.RawMessage(`[{"property":"due_date","direction":"asc"}]`),
	}, true, errKeys{})

	// then
	require.NoError(t, err)
	require.NotEmpty(t, plan.sorts)
	assert.Equal(t, domain.RelationKey(ownKey), plan.sorts[0].RelationKey)
}
