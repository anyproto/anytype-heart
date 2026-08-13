package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// bsonTypeKey is a minted BSON type key — the shape getUniqueKeyOrGenerate
// gives EVERY type not created with an explicit unique key, i.e. every custom
// type in every existing account. A fixture over `page` or another readable
// key cannot fail this test and proves nothing.
const bsonTypeKey = "67b0d3e3cda913b84c1299b1"

// preBackfillType is how the cache saw a BSON-keyed custom type before the
// apiObjectKey backfill: no stored slug, so getTypeFromStruct's fallback
// (util.ToTypeApiKey) makes the bare hex the served key.
func preBackfillType() *apimodel.Type {
	return &apimodel.Type{
		Object:    "type",
		Id:        "type-id-1",
		Key:       bsonTypeKey,
		Name:      "Invoice",
		UniqueKey: "ot-" + bsonTypeKey,
	}
}

// postBackfillType is the SAME type after the migration stamped a slug:
// getTypeFromStruct now prefers apiObjectKey, so Key becomes the slug and the
// hex is gone from that slot.
func postBackfillType() *apimodel.Type {
	t := preBackfillType()
	t.Key = "invoice"
	return t
}

func TestCacheType_BsonKeyStaysAddressableAfterTheApiObjectKeyBackfill(t *testing.T) {
	t.Run("the derived key resolves in a cache built from post-backfill details", func(t *testing.T) {
		// given — a fresh process (the restart-latent case: cacheType adds
		// without evicting, so a live process keeps the pre-backfill slot and
		// the break only appears on the next heart restart)
		fx := newFixture(t)
		fx.service.cache.cacheType(mockedSpaceId, postBackfillType())

		// when / then — both spellings address the type
		assert.Equal(t, "ot-"+bsonTypeKey, fx.service.ResolveTypeApiKey(mockedSpaceId, bsonTypeKey),
			"the hex is the only key v1 ever served for this type before the backfill")
		assert.Equal(t, "ot-"+bsonTypeKey, fx.service.ResolveTypeApiKey(mockedSpaceId, "invoice"))
	})

	t.Run("search does not silently drop the type from its filter", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.service.cache.cacheType(mockedSpaceId, postBackfillType())

		// when — prepareTypeFilters DROPS an unresolvable key, so a 200 with
		// an empty data array is what an integration polling "all my
		// Invoices" would see
		filters, _ := fx.service.prepareTypeFilters([]string{bsonTypeKey}, mockedSpaceId)

		// then
		require.Len(t, filters, 1)
		require.Len(t, filters[0].NestedFilters, 1)
		assert.Equal(t, "type-id-1", filters[0].NestedFilters[0].Value.GetStringValue())
	})

	t.Run("removeType clears the derived slot too", func(t *testing.T) {
		// given
		fx := newFixture(t)
		typ := postBackfillType()
		fx.service.cache.cacheType(mockedSpaceId, typ)

		// when
		fx.service.cache.removeType(mockedSpaceId, typ.Id, typ.UniqueKey, typ.Key)

		// then — a deleted type must not keep answering under any spelling
		assert.Empty(t, fx.service.ResolveTypeApiKey(mockedSpaceId, bsonTypeKey))
		assert.Empty(t, fx.service.ResolveTypeApiKey(mockedSpaceId, "invoice"))
		assert.Empty(t, fx.service.cache.getTypes(mockedSpaceId))
	})

	t.Run("an explicit slug still wins a slot it shares with a derived key", func(t *testing.T) {
		// given — a readable-uniqueKey type whose derived key is "invoice",
		// and a second type that stored "invoice" as its apiObjectKey. The
		// derived write must not clobber the slug holder.
		fx := newFixture(t)
		derivedHolder := &apimodel.Type{Object: "type", Id: "type-id-2", Key: "other", Name: "Other", UniqueKey: "ot-invoice"}
		slugHolder := postBackfillType()
		fx.service.cache.cacheType(mockedSpaceId, derivedHolder)
		fx.service.cache.cacheType(mockedSpaceId, slugHolder)

		// then
		assert.Equal(t, "ot-"+bsonTypeKey, fx.service.ResolveTypeApiKey(mockedSpaceId, "invoice"))
		assert.Equal(t, "ot-invoice", fx.service.ResolveTypeApiKey(mockedSpaceId, "other"))
	})
}

// TestCrossSpacePropertyFiltersVacateCorpses. v1's property cache IS v1's key
// namespace, and it filtered isHidden but not isUninstalled — so a UI-deleted
// property still listed, still resolved as an address and still blocked a
// same-key create in v1, while v2 had already vacated that slug and would
// happily mint onto it. Two versions, one slug, opposite verdicts.
func TestCrossSpacePropertyFiltersVacateCorpses(t *testing.T) {
	byKey := map[domain.RelationKey]database.FilterRequest{}
	for _, f := range crossSpacePropertyFilters() {
		byKey[f.RelationKey] = f
	}

	require.Contains(t, byKey, bundle.RelationKeyIsUninstalled,
		"the UI-delete flag must exclude a corpse from v1's namespace, as it does from v2's")
	assert.Equal(t, model.BlockContentDataviewFilter_NotEqual, byKey[bundle.RelationKeyIsUninstalled].Condition)
	assert.Equal(t, domain.Bool(true), byKey[bundle.RelationKeyIsUninstalled].Value)
	assert.Contains(t, byKey, bundle.RelationKeyIsHidden, "and the pre-existing hidden filter stays")
}
