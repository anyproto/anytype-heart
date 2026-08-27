package v2service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

// The served spelling (§7.5a): the slug is the ONLY key vocabulary the API
// serves — a bundled key spells its DERIVED slug (the table in code is its
// authority in every space and offline, stored detail or not), everything
// else its stored `apiObjectKey`. A pre-slug entity has none and keeps its
// stored key: the honest degradation, not a second vocabulary.
//
// One guard, three ways to fail it: an address the API serves must resolve
// back to the row it labels, so a spelling that a live stored key wins at
// chain step 1, that another live holder answers to at step 2, or that the
// bundled table resolves elsewhere at step 3 (the §7.5a-6 shadow) is refused
// and the honest stored key is served instead. servedKeyOf is that predicate;
// storeresolver's keyMaps.roundTrips is the same one on the document side, and
// they must stay the same — the address a listing advertises and the address a
// document carries are the same address.

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
		// the D2 shape all over.
		//
		// All three corpse shapes run, THROUGH THE ROW BUILDER — the path
		// that serves production. The original flag-only fixture asserted on
		// typeKeysById alone, whose plain query the injected isDeleted
		// default emptied of every prod corpse: its isUninstalled branch was
		// dead code in production and the fixture could not know (§8.41).
		// typeKeysById now suppresses both defaults, so the flag-only and
		// prod legs serve identically; the tombstone row carries no
		// uniqueKey, so its objects' rows serve an EMPTY type for that
		// window — the only honest answer, pinned here as such.
		// Revert check: dropping the two Condition None clauses from
		// typeKeysById fails the prod leg (the row falls back to the per-row
		// point lookup — same spelling — but the map assertion sees the
		// corpse vanish); dropping the corpseFlagged branch serves the slug
		// and fails flag-only and prod both.
		corpseShapes(t, func(t *testing.T, shape corpseShape) {
			fx := slugSpaceFixture(t)
			if shape == corpseTombstone {
				fx.addTombstone(t, "type-corpse")
			} else {
				obj := objectstore.TestObject{
					bundle.RelationKeyId:            domain.String("type-corpse"),
					bundle.RelationKeyUniqueKey:     domain.String("ot-6a7663db61fab21cd4b9e106"),
					bundle.RelationKeyApiObjectKey:  domain.String("meeting_note"),
					bundle.RelationKeyName:          domain.String("Old meeting note"),
					bundle.RelationKeyIsUninstalled: domain.Bool(true),
				}
				if shape == corpseProd {
					obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
				}
				fx.addType(t, testSpaceId, obj)
			}

			builder, err := fx.newObjectRowBuilder(testSpaceId, nil)
			require.NoError(t, err)
			details := domain.NewDetails()
			details.SetString(bundle.RelationKeyId, "note1")
			details.SetString(bundle.RelationKeyName, "Standup")
			details.SetString(bundle.RelationKeyType, "type-corpse")
			row := builder.row(database.Record{Details: details})

			if shape == corpseTombstone {
				assert.Equal(t, "", row.Type, "a tombstoned type row spells nothing — the store has nothing to spell")
			} else {
				assert.Equal(t, "6a7663db61fab21cd4b9e106", row.Type, "a corpse type spells its internal key")
			}

			keys, err := fx.typeKeysById(testSpaceId)
			require.NoError(t, err)
			assert.Equal(t, "meeting_note", keys["type-meeting"], "the live holder keeps the slug")
			if shape != corpseTombstone {
				// the bulk map itself must hold the corpse (the suppression fix):
				// without it the prod row was served by the per-row point-lookup
				// fallback — same spelling, so the row assertion above cannot
				// tell the paths apart, and the corpse branch was dead code
				assert.Equal(t, "6a7663db61fab21cd4b9e106", keys["type-corpse"],
					"the corpse is in the bulk map, not just the per-row fallback")
			}
		})
	})
}

// The §7.5a sweep: bundled keys re-spell on the wire too. The authority is
// the derived table in code, not a stored detail — an old space stores no
// apiObjectKey for its installed bundled relations and must still serve the
// slug.

func TestV2ListingsServeBundledSlugs(t *testing.T) {
	t.Run("an installed bundled property with no stored slug still serves its derived slug", func(t *testing.T) {
		// given
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-dueDate"),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
		})

		// when
		rows, _, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		keys := map[string]bool{}
		for _, row := range rows {
			keys[row.Key] = true
		}
		assert.True(t, keys["due_date"], "the derived table is the authority, stored detail or not")
		assert.False(t, keys["dueDate"], "the camel stored key is not a wire spelling any more")
	})

	t.Run("a bundled type re-spells in object rows", func(t *testing.T) {
		fx := slugSpaceFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:        domain.String("type-objectType"),
			bundle.RelationKeyUniqueKey: domain.String("ot-objectType"),
			bundle.RelationKeyName:      domain.String("Type"),
		})

		keys, err := fx.typeKeysById(testSpaceId)

		require.NoError(t, err)
		assert.Equal(t, "object_type", keys["type-objectType"])
	})

	t.Run("a squatted bundled slug keeps the honest stored key", func(t *testing.T) {
		// given — a pre-mint-check space where a custom property took
		// due_date. Serving it on the bundled row would advertise an address
		// that now 400s (the shadow is ambiguous), so the bundled row keeps
		// its stored spelling.
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:          domain.String("rel-dueDate"),
			bundle.RelationKeyRelationKey: domain.String("dueDate"),
			bundle.RelationKeyName:        domain.String("Due date"),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-squatter"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e107"),
			bundle.RelationKeyApiObjectKey: domain.String("due_date"),
			bundle.RelationKeyName:         domain.String("Due Date"),
		})

		rows, _, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		require.NoError(t, err)
		byName := map[string]string{}
		for _, row := range rows {
			byName[row.Name] = row.Key
		}
		assert.Equal(t, "dueDate", byName["Due date"])
		assert.Equal(t, "6a7663db61fab21cd4b9e107", byName["Due Date"],
			"the squatter's slug is refused too — the whole spelling is ambiguous, on both rows")
	})
}

// TestV2ServedKeyRefusesAShadowedSlug is the third guard, alone. The subtest
// above only asserted the BUNDLED row, which the slugHolders guard already
// covered; with the bundled relation NOT INSTALLED there is no bundled row at
// all and nothing in the space reveals the clash — so the listing served
// `due_date` for the squatter and the very next GET /properties/due_date
// answered 400 ambiguous. Revert the shadowed() call in servedKeyOf (keys.go)
// and this fails.
func TestV2ServedKeyRefusesAShadowedSlug(t *testing.T) {
	t.Run("a property slug the bundled table resolves elsewhere is not served", func(t *testing.T) {
		// given: the bundled dueDate is NOT installed in this space
		fx := slugSpaceFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-squatter"),
			bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e107"),
			bundle.RelationKeyApiObjectKey: domain.String("due_date"),
			bundle.RelationKeyName:         domain.String("Due Date"),
		})

		// when
		rows, _, _, err := fx.ListProperties(context.Background(), testSpaceId, 0, 25)

		// then
		require.NoError(t, err)
		served := map[string]string{}
		for _, row := range rows {
			served[row.Name] = row.Key
		}
		assert.Equal(t, "6a7663db61fab21cd4b9e107", served["Due Date"],
			"an address the listing serves must resolve back to the row it labels")

		// and the reason, executed: the served spelling is exactly the one the
		// input chain refuses
		entries, err := fx.liveProperties(testSpaceId)
		require.NoError(t, err)
		_, ok, ambiguous := fx.resolvePropertyInput("due_date", entries)
		assert.False(t, ok)
		assert.NotEmpty(t, ambiguous)
		entry, ok, ambiguous := fx.resolvePropertyInput(served["Due Date"], entries)
		require.True(t, ok, "the served address resolves")
		assert.Empty(t, ambiguous)
		assert.Equal(t, "rel-squatter", entry.Id)
	})

	t.Run("a type slug the bundled table resolves elsewhere is not served", func(t *testing.T) {
		fx := slugSpaceFixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("type-squatter"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-6a7663db61fab21cd4b9e108"),
			bundle.RelationKeyApiObjectKey: domain.String("object_type"),
			bundle.RelationKeyName:         domain.String("Object Type"),
		})

		rows, _, _, err := fx.ListTypes(context.Background(), testSpaceId, 0, 25)

		require.NoError(t, err)
		served := map[string]string{}
		for _, row := range rows {
			served[row.Name] = row.Key
		}
		assert.Equal(t, "6a7663db61fab21cd4b9e108", served["Object Type"])
	})
}

// TestV2ShadowedBundledSlugIsLoud is the 1.2 floor: a pre-existing shadow
// used to resolve silently to the squatter. Revert the shadowedBundled*
// branches in keys.go and this passes with the WRONG entity instead of
// refusing.
func TestV2ShadowedBundledSlugIsLoud(t *testing.T) {
	// given
	fx := slugSpaceFixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("rel-squatter"),
		bundle.RelationKeyRelationKey:  domain.String("6a7663db61fab21cd4b9e107"),
		bundle.RelationKeyApiObjectKey: domain.String("due_date"),
		bundle.RelationKeyName:         domain.String("Due Date"),
	})
	entries, err := fx.liveProperties(testSpaceId)
	require.NoError(t, err)

	// when
	_, ok, ambiguous := fx.resolvePropertyInput("due_date", entries)

	// then
	assert.False(t, ok, "a shadowed bundled slug must never resolve by store order")
	require.Len(t, ambiguous, 2)
	assert.Contains(t, ambiguous[0], "6a7663db61fab21cd4b9e107", "the squatter, addressable by its stored key")
	assert.Contains(t, ambiguous[1], "dueDate", "and the bundled property it shadows")
}

// TestV2ShadowedBundledTypeIsLoud is the same floor in the TYPE namespace,
// which had the branch and no test at all: revert the shadowedBundledType
// branch in resolveTypeInput and a document naming `object_type` silently
// binds the squatter instead of refusing.
func TestV2ShadowedBundledTypeIsLoud(t *testing.T) {
	// given
	fx := slugSpaceFixture(t)
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String("type-squatter"),
		bundle.RelationKeyUniqueKey:    domain.String("ot-6a7663db61fab21cd4b9e108"),
		bundle.RelationKeyApiObjectKey: domain.String("object_type"),
		bundle.RelationKeyName:         domain.String("Object Type"),
	})
	entries, err := fx.liveTypes(testSpaceId)
	require.NoError(t, err)

	// when
	_, ok, ambiguous := fx.resolveTypeInput("object_type", entries)

	// then
	assert.False(t, ok)
	require.Len(t, ambiguous, 2)
	assert.Contains(t, ambiguous[0], "6a7663db61fab21cd4b9e108")
	assert.Contains(t, ambiguous[1], "objectType")

	// and the stored key still addresses the squatter, which is what makes the
	// refusal actionable
	entry, ok, ambiguous := fx.resolveTypeInput("6a7663db61fab21cd4b9e108", entries)
	require.True(t, ok)
	assert.Empty(t, ambiguous)
	assert.Equal(t, "type-squatter", entry.Id)
}
