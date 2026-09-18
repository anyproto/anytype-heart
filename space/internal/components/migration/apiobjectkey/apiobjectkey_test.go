package apiobjectkey

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

const spaceId = "space1"

type fixture struct {
	store   *objectstore.StoreFixture
	spc     *mock_space.MockSpace
	written map[string]string // object id -> the slug the migration stored
}

func newFixture(t *testing.T) *fixture {
	store := objectstore.NewStoreFixture(t)
	spc := mock_space.NewMockSpace(t)
	spc.EXPECT().Id().Return(spaceId).Maybe()
	fx := &fixture{store: store, spc: spc, written: map[string]string{}}
	spc.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, objectId string, apply func(smartblock.SmartBlock) error) error {
			sb := smarttest.New(objectId)
			if err := apply(sb); err != nil {
				return err
			}
			fx.written[objectId] = sb.NewState().Details().GetString(bundle.RelationKeyApiObjectKey)
			return nil
		}).Maybe()
	return fx
}

func (fx *fixture) run(t *testing.T) (toMigrate, migrated int) {
	t.Helper()
	toMigrate, migrated, err := Migration{}.Run(context.Background(), logger.NewNamed("test"), fx.store.SpaceIndex(spaceId), fx.spc)
	require.NoError(t, err)
	return toMigrate, migrated
}

func property(id, key, slug, name string) objectstore.TestObject {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
		bundle.RelationKeyRelationKey:    domain.String(key),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}
	if slug != "" {
		obj[bundle.RelationKeyApiObjectKey] = domain.String(slug)
	}
	return obj
}

func objectType(id, key, slug, name string) objectstore.TestObject {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key)
	if err != nil {
		panic(err)
	}
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
		bundle.RelationKeyUniqueKey:      domain.String(uk.Marshal()),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	if slug != "" {
		obj[bundle.RelationKeyApiObjectKey] = domain.String(slug)
	}
	return obj
}

const (
	bsonA = "68b1c0aa4e1f0d0011223344"
	bsonB = "68b1c0aa4e1f0d0011223355"
	bsonC = "78b1c0aa4e1f0d0011223366"
)

func TestBackfill(t *testing.T) {
	t.Run("a BSON-keyed custom property gets a slug derived from its name", func(t *testing.T) {
		// given — the §7.5a-6 case: pre-slug custom keys have no stable
		// bare-op address at all until this runs
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "", "Warranty until"),
		})

		// when
		toMigrate, migrated := fx.run(t)

		// then
		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 1, migrated)
		assert.Equal(t, map[string]string{"rel1": "warranty_until"}, fx.written)
	})

	t.Run("a readable legacy key is its own slug", func(t *testing.T) {
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", "myLegacyKey", "", "Whatever the user renamed it to"),
		})

		_, migrated := fx.run(t)

		assert.Equal(t, 1, migrated)
		assert.Equal(t, "my_legacy_key", fx.written["rel1"])
	})

	t.Run("bundled keys are left alone — their slug is derived in code", func(t *testing.T) {
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel-dueDate", "dueDate", "", "Due date"),
			objectType("ot-page", "page", "", "Page"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 0, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})

	t.Run("an object that already has a slug is never re-pointed", func(t *testing.T) {
		// given — apiObjectKey is mutable and v1-visible; overwriting one
		// would silently re-aim a shipped address
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "stale_but_shipped", "Renamed since"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 0, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})

	t.Run("a corpse neither gets a slug nor holds one", func(t *testing.T) {
		// given — §7.5-requirement-2: uninstalled objects vacate the
		// namespace
		fx := newFixture(t)
		corpse := property("rel1", bsonA, "", "Warranty until")
		corpse[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			corpse,
			property("rel2", bsonB, "", "Warranty until"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 1, migrated)
		assert.Equal(t, map[string]string{"rel2": "warranty_until"}, fx.written)
	})

	t.Run("a corpse-held slug does not block the backfill — even when the corpse STORES it", func(t *testing.T) {
		// given — the sibling case to the one above, with the corpse's
		// apiObjectKey POPULATED: an entity that was backfilled or minted a
		// slug and then UI-deleted still stores it. The vacate policy
		// (§7.5-req-2) says the dead row does not hold the spelling, so the
		// live candidate deriving the same slug is stamped — after which TWO
		// rows persist apiObjectKey "warranty_until", one dead, one live.
		// That is intended and safe while the corpse stays dead (every
		// namespace excludes it); if a revival channel ever appears, the
		// loud-ambiguity floor at lookup is what catches the twins (§8-OQ2's
		// re-slug-on-revive half is not built). Both store shapes are run:
		// flag-only pins the migration's own isUninstalled filter, the prod
		// double-flag ({isUninstalled,isDeleted}) pins what a real UI delete
		// leaves in the index.
		for _, shape := range []struct {
			name string
			prod bool
		}{{"flag-only", false}, {"prod", true}} {
			t.Run(shape.name, func(t *testing.T) {
				fx := newFixture(t)
				corpse := property("rel-corpse", bsonA, "warranty_until", "Warranty until")
				corpse[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
				if shape.prod {
					corpse[bundle.RelationKeyIsDeleted] = domain.Bool(true)
				}
				fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
					corpse,
					property("rel-live", bsonB, "", "Warranty until"),
				})

				toMigrate, migrated := fx.run(t)

				assert.Equal(t, 1, toMigrate)
				assert.Equal(t, 1, migrated)
				assert.Equal(t, map[string]string{"rel-live": "warranty_until"}, fx.written)
			})
		}
	})

	t.Run("the migration is idempotent: a second run over its own output does nothing", func(t *testing.T) {
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "", "Warranty until"),
		})
		fx.run(t)

		// the store now reflects the first run
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "warranty_until", "Warranty until"),
		})
		fx.written = map[string]string{}

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 0, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})
}

func TestBackfill_TakenSlugIsANoOp(t *testing.T) {
	// takenSlugPolicy: a backfill never invents a name. Every case here is
	// a deliberate no-op, not an oversight — ADDRESSING.md §8 open
	// question 3 owns the repair.
	require.Equal(t, "skip", takenSlugPolicy)

	t.Run("a name colliding with a bundled derived slug is skipped", func(t *testing.T) {
		// given — the collision the mint hardening now prevents at birth;
		// old data still holds it
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "", "Due Date"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})

	t.Run("a name colliding with a stored slug is skipped", func(t *testing.T) {
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "warranty_until", "Warranty until"),
			property("rel2", bsonB, "", "Warranty until"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})

	t.Run("a name colliding with a stored key is skipped", func(t *testing.T) {
		// given — a legacy readable key is addressable as itself (chain
		// step 1), so a slug spelled the same would shadow it
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", "legacy_artist", "something_else", "Legacy artist"),
			property("rel2", bsonB, "", "Legacy artist"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})

	t.Run("two candidates deriving one slug: the lower id wins, deterministically", func(t *testing.T) {
		// given — convergence: two devices running this independently must
		// reach the same assignment, so store order can never decide
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("relZ", bsonC, "", "Warranty until"),
			property("relA", bsonA, "", "Warranty until"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 2, toMigrate)
		assert.Equal(t, 1, migrated)
		assert.Equal(t, map[string]string{"relA": "warranty_until"}, fx.written)
	})

	t.Run("a skipped candidate is picked up by a later run once the obstacle is gone", func(t *testing.T) {
		// given — "safe to re-run" is a property, not a hope
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "warranty_until", "Warranty until"),
			property("rel2", bsonB, "", "Warranty until"),
		})
		_, migrated := fx.run(t)
		require.Equal(t, 0, migrated)

		// the squatter is re-pointed elsewhere (v1 PATCH, or deleted)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "warranty_period", "Warranty until"),
		})

		_, migrated = fx.run(t)

		assert.Equal(t, 1, migrated)
		assert.Equal(t, "warranty_until", fx.written["rel2"])
	})

	t.Run("the type and property namespaces are separate", func(t *testing.T) {
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "cocktail", "Cocktail"),
			objectType("typ1", bsonB, "", "Cocktail"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 1, migrated)
		assert.Equal(t, map[string]string{"typ1": "cocktail"}, fx.written)
	})

	t.Run("a name with nothing derivable is skipped", func(t *testing.T) {
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			property("rel1", bsonA, "", "☕"),
		})

		toMigrate, migrated := fx.run(t)

		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 0, migrated)
		assert.Empty(t, fx.written)
	})
}

// TestBackfillLeavesHiddenObjectsAlone is the candidate-side half of the
// hidden-holder rule. A hidden relation is invisible AND undeletable to an
// API caller, so it does not participate in the slug namespace on the request
// side — stamping it a slug manufactures exactly the twin that rule then has
// to paper over, and the twin is the one the caller cannot see, name or
// remove. It still HOLDS a stored slug against a candidate: not stamping and
// not colliding are the same policy from two sides.
func TestBackfillLeavesHiddenObjectsAlone(t *testing.T) {
	t.Run("a hidden candidate is not stamped", func(t *testing.T) {
		// given — two BSON-keyed relations that would slug identically; the
		// hidden one used to win the id race and take `severity`
		fx := newFixture(t)
		hidden := property("rel-hidden", bsonA, "", "Severity")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			hidden,
			property("rel-visible", bsonB, "", "Severity"),
		})

		// when
		toMigrate, migrated := fx.run(t)

		// then
		assert.Equal(t, 1, toMigrate, "the hidden object is not a candidate at all")
		assert.Equal(t, 1, migrated)
		assert.Equal(t, map[string]string{"rel-visible": "severity"}, fx.written,
			"the visible relation gets the clean slug, not a suffix and not a skip")
	})

	t.Run("a hidden object still holds a slug it already stored", func(t *testing.T) {
		// given — the hidden holder already carries `severity` in data; a
		// candidate must not be minted onto it (that IS the ambiguity)
		fx := newFixture(t)
		hidden := property("rel-hidden", bsonA, "severity", "Severity")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			hidden,
			property("rel-visible", bsonB, "", "Severity"),
		})

		// when
		toMigrate, migrated := fx.run(t)

		// then
		assert.Equal(t, 1, toMigrate)
		assert.Equal(t, 0, migrated, "takenSlugPolicy: skip, loudly enough to count")
		assert.Empty(t, fx.written)
	})
}
