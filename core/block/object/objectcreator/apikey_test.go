package objectcreator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// These tests pin the union the mint checks (APIV2_ADDRESSING.md §7.5a-6). Each
// arm fails on revert of the corresponding branch in apikey.go: without the
// check every case below stores the colliding slug verbatim.

const apiKeySpaceId = "spcApiKey"

func relationRecord(id, key, slug string) objectstore.TestObject {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySpaceId:        domain.String(apiKeySpaceId),
		bundle.RelationKeyRelationKey:    domain.String(key),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}
	if slug != "" {
		obj[bundle.RelationKeyApiObjectKey] = domain.String(slug)
	}
	return obj
}

func typeRecord(id, key, slug string) objectstore.TestObject {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key)
	if err != nil {
		panic(err)
	}
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeySpaceId:        domain.String(apiKeySpaceId),
		bundle.RelationKeyUniqueKey:      domain.String(uk.Marshal()),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	if slug != "" {
		obj[bundle.RelationKeyApiObjectKey] = domain.String(slug)
	}
	return obj
}

func mintRelationSlug(t *testing.T, f *fixture, name, key string) string {
	t.Helper()
	object := domain.NewDetails()
	object.SetString(bundle.RelationKeyName, name)
	injectApiObjectKey(object, key)
	require.NoError(t, f.service.(*service).ensureUniqueApiObjectKey(apiKeySpaceId, object, key, apiKeyKindRelation))
	return object.GetString(bundle.RelationKeyApiObjectKey)
}

func TestMintApiObjectKey_UnionCheck(t *testing.T) {
	t.Run("a name colliding with a bundled derived slug is disambiguated", func(t *testing.T) {
		// given — the §7.5a-6 headline: "Due Date" slugs to due_date, which
		// is exactly bundled dueDate's derived slug. Nothing is stored for
		// it in the space, so ONLY the bundled arm can catch this.
		f := newFixture(t)
		want := "due_date_2"

		// when
		got := mintRelationSlug(t, f, "Due Date", "")

		// then
		assert.Equal(t, want, got)
	})

	t.Run("the bundled arm sees uninstalled bundled keys too", func(t *testing.T) {
		f := newFixture(t)
		// the space holds nothing at all; the derived table is the authority
		assert.Equal(t, "media_artist_url_2", mintRelationSlug(t, f, "Media artist URL", ""))
	})

	t.Run("a name colliding with a stored slug is disambiguated", func(t *testing.T) {
		// given — §2.3-1's two "Manual property" properties
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			relationRecord("rel1", "68b1c0aa4e1f0d0011223344", "manual_property"),
		})

		// when / then
		assert.Equal(t, "manual_property_2", mintRelationSlug(t, f, "Manual property", ""))
	})

	t.Run("a name colliding with a stored KEY is disambiguated", func(t *testing.T) {
		// given — a legacy readable custom key is addressable as itself
		// (chain step 1), so a slug spelled the same is a shadow
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			relationRecord("rel1", "artist", ""),
		})

		assert.Equal(t, "artist_2", mintRelationSlug(t, f, "Artist", ""))
	})

	t.Run("the walk keeps going while spellings are taken", func(t *testing.T) {
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			relationRecord("rel1", "68b1c0aa4e1f0d0011223344", "priority_2"),
			relationRecord("rel2", "68b1c0aa4e1f0d0011223355", "priority_3"),
		})

		// bundled `priority` holds the bare spelling, rel1/rel2 the next two
		assert.Equal(t, "priority_4", mintRelationSlug(t, f, "Priority", ""))
	})

	t.Run("a free slug is stored verbatim", func(t *testing.T) {
		f := newFixture(t)
		assert.Equal(t, "warranty_until", mintRelationSlug(t, f, "Warranty until", ""))
	})

	t.Run("a corpse vacates the namespace", func(t *testing.T) {
		// given — §7.5-requirement-2: a UI-deleted property must not hold
		// its slug against a same-name create
		f := newFixture(t)
		corpse := relationRecord("rel1", "68b1c0aa4e1f0d0011223344", "warranty_until")
		corpse[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{corpse})

		assert.Equal(t, "warranty_until", mintRelationSlug(t, f, "Warranty until", ""))
	})

	t.Run("an entity does not collide with its own slug", func(t *testing.T) {
		// given — a re-create of the same stored key (reinstall, retry)
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			relationRecord("rel1", "myLegacyKey", "my_legacy_key"),
		})

		assert.Equal(t, "my_legacy_key", mintRelationSlug(t, f, "My legacy key", "myLegacyKey"))
	})

	t.Run("a bundled install keeps its derived slug even when a custom holder squats it", func(t *testing.T) {
		// given — the bundled key's slug is DERIVED, not minted: the table
		// in code is its authority, so the install never suffixes. (The
		// squatter is what needs repair; see ADDRESSING §8-OQ3.)
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			relationRecord("rel1", "68b1c0aa4e1f0d0011223344", "due_date"),
		})

		assert.Equal(t, "due_date", mintRelationSlug(t, f, "Due date", "dueDate"))
	})

	t.Run("a caller-supplied slug is checked like a derived one", func(t *testing.T) {
		// given — v1 POST /properties sets apiObjectKey itself; the mint is
		// the last gate before the store
		f := newFixture(t)
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "Anything")
		object.SetString(bundle.RelationKeyApiObjectKey, "due_date")
		injectApiObjectKey(object, "")

		require.NoError(t, f.service.(*service).ensureUniqueApiObjectKey(apiKeySpaceId, object, "", apiKeyKindRelation))
		assert.Equal(t, "due_date_2", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("a name with no derivable slug stays empty", func(t *testing.T) {
		f := newFixture(t)
		assert.Equal(t, "", mintRelationSlug(t, f, "  ", ""))
	})
}

// TestCreateRelation_MintsUniqueApiObjectKey pins the WIRING, not just the
// helper: revert the ensureUniqueApiObjectKey call in createRelation and the
// created relation stores the shadowing `due_date` again.
func TestCreateRelation_MintsUniqueApiObjectKey(t *testing.T) {
	// given
	f := newFixture(t)
	f.spaceService.EXPECT().Get(mock.Anything, apiKeySpaceId).Return(f.spc, nil)
	f.spc.EXPECT().Id().Return(apiKeySpaceId).Maybe()
	f.spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, key domain.UniqueKey) (string, error) { return key.Marshal(), nil }).Maybe()
	var created *state.State
	f.spc.EXPECT().DeriveTreeObject(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, params objectcache.TreeDerivationParams) (smartblock.SmartBlock, error) {
			sb := smarttest.New(params.Key.Marshal())
			initCtx := params.InitFunc(params.Key.Marshal())
			created = initCtx.State
			return sb, sb.Init(initCtx)
		})

	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, "Due Date")
	details.SetInt64(bundle.RelationKeyRelationFormat, int64(model.RelationFormat_date))

	// when
	_, _, err := f.service.CreateObject(context.Background(), apiKeySpaceId, CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyRelation,
		Details:       details,
	})

	// then
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "due_date_2", created.Details().GetString(bundle.RelationKeyApiObjectKey))
}

func TestMintApiObjectKey_TypeNamespace(t *testing.T) {
	mint := func(t *testing.T, f *fixture, name, key string) string {
		t.Helper()
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, name)
		injectApiObjectKey(object, key)
		require.NoError(t, f.service.(*service).ensureUniqueApiObjectKey(apiKeySpaceId, object, key, apiKeyKindType))
		return object.GetString(bundle.RelationKeyApiObjectKey)
	}

	t.Run("a type name colliding with a bundled type slug is disambiguated", func(t *testing.T) {
		f := newFixture(t)
		// objectType is one of the five bundled type keys that re-spell
		assert.Equal(t, "object_type_2", mint(t, f, "Object type", ""))
	})

	t.Run("a type name colliding with a stored type slug is disambiguated", func(t *testing.T) {
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			typeRecord("typ1", "68b1c0aa4e1f0d0011223344", "cocktail"),
		})

		assert.Equal(t, "cocktail_2", mint(t, f, "Cocktail", ""))
	})

	t.Run("the type and property namespaces are separate", func(t *testing.T) {
		// given — a PROPERTY holding `cocktail` must not push a TYPE named
		// "Cocktail" onto a suffix: different routes, different slots
		f := newFixture(t)
		f.objectStore.AddObjects(t, apiKeySpaceId, []objectstore.TestObject{
			relationRecord("rel1", "68b1c0aa4e1f0d0011223344", "cocktail"),
		})

		assert.Equal(t, "cocktail", mint(t, f, "Cocktail", ""))
	})
}

// TestCreateObjectType_MintsUniqueApiObjectKey is TestCreateRelation_… for the
// OTHER namespace: the type-create path had the same mint wiring and no test
// of it, so reverting the ensureUniqueApiObjectKey call in createObjectType
// left every suite green while a new type stored the shadowing `object_type`.
func TestCreateObjectType_MintsUniqueApiObjectKey(t *testing.T) {
	// given
	f := newFixture(t)
	f.spaceService.EXPECT().Get(mock.Anything, apiKeySpaceId).Return(f.spc, nil)
	f.spc.EXPECT().Id().Return(apiKeySpaceId).Maybe()
	f.spc.EXPECT().IsReadOnly().Return(true).Maybe() // skips the bundled-relation install
	f.spc.EXPECT().GetRelationIdByKey(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, key domain.RelationKey) (string, error) { return key.URL(), nil }).Maybe()
	f.spc.EXPECT().GetTypeIdByKey(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, key domain.TypeKey) (string, error) { return key.URL(), nil }).Maybe()
	f.spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, key domain.UniqueKey) (string, error) { return key.Marshal(), nil }).Maybe()
	var created *state.State
	f.spc.EXPECT().DeriveTreeObject(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, params objectcache.TreeDerivationParams) (smartblock.SmartBlock, error) {
			sb := smarttest.New(params.Key.Marshal())
			initCtx := params.InitFunc(params.Key.Marshal())
			created = initCtx.State
			return sb, sb.Init(initCtx)
		})

	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, "Object type")

	// when
	_, _, err := f.service.CreateObject(context.Background(), apiKeySpaceId, CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyObjectType,
		Details:       details,
	})

	// then
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "object_type_2", created.Details().GetString(bundle.RelationKeyApiObjectKey),
		"the bundled objectType holds the bare spelling — the mint must disambiguate")
}
