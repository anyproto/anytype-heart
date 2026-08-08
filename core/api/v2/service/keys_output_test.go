package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// The served spelling (§7.5a interim): a BSON-keyed property or type
// answers to its slug, so the slug is what listings advertise — but ONLY
// when the slug round-trips through the resolution chain to the row it
// labels (twins and slugs shadowed by a live stored key keep the honest
// BSON spelling). Readable stored keys keep their spelling until the full
// respelling sweep.

func TestV2ListingsServeSlugs(t *testing.T) {
	t.Run("a BSON-keyed property row advertises its slug", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)

		// when
		rows, _, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		keys := map[string]bool{}
		for _, row := range rows {
			keys[row.Key] = true
		}
		assert.True(t, keys["manual_property"], "the slug is the served address")
		assert.False(t, keys[slugPropKey], "the BSON spelling must not appear once the slug serves")
	})

	t.Run("twin slugs fall back to the honest BSON spelling", func(t *testing.T) {
		// given: two live holders of one slug — serving it on either row
		// would advertise an address that resolves to a 400
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-manual-twin"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e105"),
			bundle.RelationKeyApiObjectKey: domain.String("manual_property"),
			bundle.RelationKeyName:         domain.String("Manual property twin"),
		})

		// when
		rows, _, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		keys := map[string]bool{}
		for _, row := range rows {
			keys[row.Key] = true
		}
		assert.False(t, keys["manual_property"])
		assert.True(t, keys[slugPropKey])
		assert.True(t, keys["6a7663db61fab21cd4b9e105"])
	})

	t.Run("a slug shadowed by a live stored key is not served", func(t *testing.T) {
		// given: a legacy relation whose STORED key equals another's slug —
		// chain step 1 wins on input, so serving the slug would label the
		// wrong row
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-legacy"),
			bundle.RelationKeyRelationKey: domain.String("manual_property"),
			bundle.RelationKeyName:        domain.String("Legacy readable key"),
		})

		// when
		rows, _, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then: the legacy row keeps its key; the BSON row keeps its BSON
		require.NoError(t, err)
		byId := map[string]string{}
		for _, row := range rows {
			byId[row.Name] = row.Key
		}
		assert.Equal(t, "manual_property", byId["Legacy readable key"])
		assert.Equal(t, slugPropKey, byId["Manual property"])
	})

	t.Run("a BSON-keyed type row advertises its slug", func(t *testing.T) {
		fx := slugSpaceFixture(t)

		rows, _, _, err := fx.ListTypes(context.Background(), testSpaceId, 0, 25)

		require.NoError(t, err)
		keys := map[string]bool{}
		for _, row := range rows {
			keys[row.Key] = true
		}
		assert.True(t, keys["meeting_note"])
		assert.False(t, keys[slugTypeKey])
	})

	t.Run("object rows spell a slug-keyed type by its slug", func(t *testing.T) {
		// typeKeysById feeds every object/search row's type column — the
		// spelling must be the address the search type filter resolves back
		fx := slugSpaceFixture(t)

		keys, err := fx.typeKeysById(testSpaceId)

		require.NoError(t, err)
		assert.Equal(t, "meeting_note", keys["type-meeting"])
	})

	t.Run("a corpse type keeps its internal spelling in rows", func(t *testing.T) {
		// its slug vacated the namespace (§8-OQ2) — a recreated live type
		// may hold it now, and two rows advertising one address would be
		// the D2 shape all over
		fx := slugSpaceFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("type-corpse"),
			bundle.RelationKeyUniqueKey:     domain.String("ot-6a7663db61fab21cd4b9e106"),
			bundle.RelationKeyApiObjectKey:  domain.String("meeting_note"),
			bundle.RelationKeyName:          domain.String("Old meeting note"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})

		keys, err := fx.typeKeysById(testSpaceId)

		require.NoError(t, err)
		assert.Equal(t, "6a7663db61fab21cd4b9e106", keys["type-corpse"])
		assert.Equal(t, "meeting_note", keys["type-meeting"], "the live holder keeps the slug")
	})
}
