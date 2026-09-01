package v2service

// apikeyvocab_test.go — the slug vocabulary's interface obligations
// (keyvocab.go's three, plus the API's own): everything emitted inverts,
// a live stored key outranks every table, a squatted bundled slug neither
// emits nor resolves to the bundled key, the 4% non-name-derivable slugs
// keep resolving, and a slug-spelled body owes no legend.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func vocabFixture(t *testing.T) (*v2Fixture, *apiKeyVocab) {
	fx := newV2Fixture(t)
	// the 4% class: a slug minted before a rename — not derivable from the
	// current name, and the name folds nowhere near it
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-awareness"),
		bundle.RelationKeyRelationKey:  domain.String("bsonAwareKey"),
		bundle.RelationKeyApiObjectKey: domain.String("discovery"),
		bundle.RelationKeyName:         domain.String("Awareness"),
	})
	// a legacy relation whose STORED key is snake — the verbatim-first case
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("rel-legacy-due"),
		bundle.RelationKeyRelationKey: domain.String("due_date"),
		bundle.RelationKeyName:        domain.String("Legacy due date"),
	})
	// a BSON key with no slug at all (pre-backfill): its own honest address
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:          domain.String("rel-bare"),
		bundle.RelationKeyRelationKey: domain.String("bsonBareKey"),
		bundle.RelationKeyName:        domain.String("Bare"),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("type-gadget"),
		bundle.RelationKeyUniqueKey:    domain.String("ot-bsonGadget"),
		bundle.RelationKeyApiObjectKey: domain.String("gadget"),
		bundle.RelationKeyName:         domain.String("Gadget"),
	})
	reads := storeresolver.New(fx.store.SpaceIndex(testSpaceId))
	return fx, fx.apiKeys(testSpaceId, reads)
}

func TestApiKeyVocab(t *testing.T) {
	t.Run("emit is the served slug, a table lookup", func(t *testing.T) {
		_, v := vocabFixture(t)
		assert.Equal(t, "discovery", v.PropertySlug("bsonAwareKey"),
			"the stored slug, not anything derived from the name")
		assert.Equal(t, "bsonBareKey", v.PropertySlug("bsonBareKey"),
			"no slug minted: the stored key is its own honest address")
		assert.Equal(t, "created_date", v.PropertySlug("createdDate"),
			"a bundled key spells as its derived slug even when not installed")
		assert.Equal(t, "dueDate", v.PropertySlug("dueDate"),
			"the fixture's legacy relation holds the stored key due_date, and a live stored key wins the spelling — the bundled key demotes")
		assert.Equal(t, "gadget", v.TypeSlug("bsonGadget"))
		assert.Equal(t, "page", v.TypeSlug("page"))
	})

	t.Run("everything emitted inverts (obligation 1)", func(t *testing.T) {
		_, v := vocabFixture(t)
		for _, stored := range []string{"bsonAwareKey", "bsonBareKey", "createdDate", "dueDate", "name"} {
			slug := v.PropertySlug(stored)
			if slug == stored {
				continue // verbatim is the caller's step, not the table's
			}
			key, ok := v.PropertyKey(slug)
			require.True(t, ok, "emitted %q for %q must invert", slug, stored)
			assert.Equal(t, stored, key)
		}
	})

	t.Run("the 4% class resolves through the slug table alone", func(t *testing.T) {
		// the post-switch format chain no longer reads apiObjectKey, so
		// without the table this is exactly the spelling that stops working
		_, v := vocabFixture(t)
		key, ok := v.PropertyKey("discovery")
		require.True(t, ok)
		assert.Equal(t, "bsonAwareKey", key)
	})

	t.Run("a live stored key outranks every table (obligation 3)", func(t *testing.T) {
		_, v := vocabFixture(t)
		key, ok := v.PropertyKey("due_date")
		assert.False(t, ok, "an exact live stored key is not a spelling — the caller takes it verbatim")
		assert.Equal(t, "due_date", key, "the miss convention is (input, false), never empty")
	})

	t.Run("the inner chain stays underneath: names and folds still resolve", func(t *testing.T) {
		_, v := vocabFixture(t)
		key, ok := v.PropertyKey("Awareness")
		require.True(t, ok, "a display name resolves through the embedded resolver")
		assert.Equal(t, "bsonAwareKey", key)
	})

	t.Run("a squatted bundled slug neither emits nor resolves to the bundled key", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-squat"),
			bundle.RelationKeyRelationKey:  domain.String("bsonSquatKey"),
			bundle.RelationKeyApiObjectKey: domain.String("due_date"),
			bundle.RelationKeyName:         domain.String("Squatter"),
		})
		v := fx.apiKeys(testSpaceId, storeresolver.New(fx.store.SpaceIndex(testSpaceId)))

		assert.Equal(t, "bsonSquatKey", v.PropertySlug("bsonSquatKey"),
			"no shadowing of the bundled table (obligation 2): a stored slug the table binds elsewhere is not served")
		assert.Equal(t, "dueDate", v.PropertySlug("dueDate"),
			"the bundled key demotes too while its derived slug is contested")
		key, _ := v.PropertyKey("due_date")
		assert.NotEqual(t, "bsonSquatKey", key,
			"the contested spelling never lands on the squatter (v2's request channels refuse it loudly — resolvePropertyInput's shadow check)")
	})

	t.Run("a slug-spelled body owes no legend", func(t *testing.T) {
		fx, v := vocabFixture(t)
		reads := storeresolver.New(fx.store.SpaceIndex(testSpaceId))
		opts := reads.Options()
		opts.Keys = v

		snapshot := &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"name":         pbtypes.String("Doc"),
				"bsonAwareKey": pbtypes.String("high"),
			}},
		}
		body, err := anyblockjson.Marshal(model.SmartBlockType_Page, snapshot, opts)
		require.NoError(t, err)

		assert.Contains(t, string(body), `"discovery"`,
			"the non-derivable slug is the served spelling")
		// the legend is owed to a reader the export never met: a
		// package-only reader holds the bundled table alone, which cannot
		// invert a space-minted slug — so the document carries the mapping
		// itself, exactly as the raw-name shape carries its own names
		assert.Contains(t, string(body), `"property_internal_keys"`,
			"a space-slug spelling self-describes for package-only readers")
		assert.Contains(t, string(body), `"discovery": "bsonAwareKey"`,
			"the legend binds the served slug to the stored key")
	})
}
