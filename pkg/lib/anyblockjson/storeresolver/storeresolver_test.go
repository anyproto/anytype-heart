package storeresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type fixture struct {
	*Resolvers
	index *spaceindex.StoreFixture
}

func newFixture(t *testing.T) *fixture {
	index := spaceindex.NewStoreFixture(t)
	index.AddObjects(t, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:             domain.String("rel-priority"),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
			bundle.RelationKeyName:           domain.String("Priority"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			bundle.RelationKeyId:             domain.String("opt-high"),
			bundle.RelationKeyRelationKey:    domain.String("priority"),
			bundle.RelationKeyName:           domain.String("High"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		},
	})
	return &fixture{Resolvers: New(index), index: index}
}

func TestResolveFormat(t *testing.T) {
	t.Run("known custom key resolves", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		format, ok := fx.ResolveFormat("priority")

		// then
		require.True(t, ok)
		assert.Equal(t, model.RelationFormat_status, format)
	})

	t.Run("unknown key does not resolve", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, ok := fx.ResolveFormat("nope")

		// then
		assert.False(t, ok)
	})
}

func TestOptionResolution(t *testing.T) {
	// given
	fx := newFixture(t)

	// when / then: both directions invert each other
	name, ok := fx.OptionName("priority", "opt-high")
	require.True(t, ok)
	assert.Equal(t, "High", name)

	id, ok := fx.OptionId("priority", "High")
	require.True(t, ok)
	assert.Equal(t, "opt-high", id)

	_, ok = fx.OptionName("priority", "missing")
	assert.False(t, ok)
}

func TestPropertyResolution(t *testing.T) {
	// given
	fx := newFixture(t)
	want := anyblockjson.PropertyDefinition{
		Key:    "priority",
		Name:   "Priority",
		Format: model.RelationFormat_status,
	}

	// when / then: id -> definition -> id round-trips
	def, ok := fx.PropertyById("rel-priority")
	require.True(t, ok)
	assert.Equal(t, want, def)

	id, ok := fx.PropertyId(def)
	require.True(t, ok)
	assert.Equal(t, "rel-priority", id)
}

func TestOptionsWiring(t *testing.T) {
	// given
	fx := newFixture(t)

	// when
	opts := fx.Options()

	// then
	assert.NotNil(t, opts.ResolveFormat)
	assert.Equal(t, fx.Resolvers, opts.ResolveOptions)
	assert.Equal(t, fx.Resolvers, opts.ResolveProperties)
}
