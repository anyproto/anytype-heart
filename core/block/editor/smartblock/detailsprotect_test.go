package smartblock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testRelationId = "relationObjectId"

// newIdentityFixture builds a smartblock of the given type and runs the real Init, so the guard is
// registered exactly the way production registers it. Every assertion below goes through the real
// Apply: if the hook were not reached, the incoming value would simply land in the doc.
func newIdentityFixture(t *testing.T, sbType smartblock.SmartBlockType) *fixture {
	fx := newFixture(testRelationId, t)
	fx.source.sbType = sbType
	fx.init(t, []*model.Block{{Id: testRelationId}})
	fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Maybe()
	return fx
}

// captureLog swaps the package logger for one writing into an observer, so a test can tell a
// silent no-op apart from a rejected write. Other warnings of the package are filtered out by
// rejections, so a count is a count of rejections and nothing else.
func captureLog(t *testing.T) func() []observer.LoggedEntry {
	core, logs := observer.New(zapcore.WarnLevel)
	previous := log
	log = &logging.Sugared{SugaredLogger: zap.New(core).Sugar()}
	t.Cleanup(func() { log = previous })
	return func() []observer.LoggedEntry {
		return logs.FilterMessageSnippet("identity detail of an existing object").All()
	}
}

func TestPreserveIdentityDetails_Relation(t *testing.T) {
	t.Run("creation sets the identity details", func(t *testing.T) {
		// given
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		rejections := captureLog(t)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeyRelationKey, domain.String("author"))
		s.SetDetail(bundle.RelationKeyRelationFormat, domain.Int64(int64(model.RelationFormat_object)))
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("_brauthor"))

		// when
		require.NoError(t, fx.Apply(s))

		// then
		details := fx.Details()
		assert.Equal(t, "author", details.GetString(bundle.RelationKeyRelationKey))
		assert.Equal(t, int64(model.RelationFormat_object), details.GetInt64(bundle.RelationKeyRelationFormat))
		assert.Equal(t, "_brauthor", details.GetString(bundle.RelationKeySourceObject))
		assert.Empty(t, rejections())
	})

	t.Run("change to another value is ignored and reported", func(t *testing.T) {
		// given
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		applyIdentity(t, fx, "author", model.RelationFormat_object, "_brauthor")
		rejections := captureLog(t)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeyRelationKey, domain.String("hijacked"))
		s.SetDetail(bundle.RelationKeyRelationFormat, domain.Int64(int64(model.RelationFormat_longtext)))
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("_brhijacked"))
		s.SetDetail(bundle.RelationKeyName, domain.String("Author"))

		// when
		require.NoError(t, fx.Apply(s))

		// then
		details := fx.Details()
		assert.Equal(t, "author", details.GetString(bundle.RelationKeyRelationKey))
		assert.Equal(t, int64(model.RelationFormat_object), details.GetInt64(bundle.RelationKeyRelationFormat))
		assert.Equal(t, "_brauthor", details.GetString(bundle.RelationKeySourceObject))
		// the rest of the state still lands, one bad detail must not cost the whole apply
		assert.Equal(t, "Author", details.GetString(bundle.RelationKeyName))
		assert.Len(t, rejections(), len(identityDetails))
	})

	t.Run("rewriting the same value is a silent no-op", func(t *testing.T) {
		// given
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		applyIdentity(t, fx, "author", model.RelationFormat_object, "_brauthor")
		rejections := captureLog(t)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeyRelationKey, domain.String("author"))
		s.SetDetail(bundle.RelationKeyRelationFormat, domain.Int64(int64(model.RelationFormat_object)))
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("_brauthor"))

		// when
		require.NoError(t, fx.Apply(s))

		// then
		details := fx.Details()
		assert.Equal(t, "author", details.GetString(bundle.RelationKeyRelationKey))
		assert.Equal(t, int64(model.RelationFormat_object), details.GetInt64(bundle.RelationKeyRelationFormat))
		assert.Empty(t, rejections())
	})

	t.Run("backfill of a missing value is allowed", func(t *testing.T) {
		// given - a relation that never got a sourceObject
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		applyIdentity(t, fx, "author", model.RelationFormat_object, "")
		rejections := captureLog(t)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("_brauthor"))

		// when
		require.NoError(t, fx.Apply(s))

		// then
		assert.Equal(t, "_brauthor", fx.Details().GetString(bundle.RelationKeySourceObject))
		assert.Empty(t, rejections())
	})

	t.Run("longtext format is a value, not an absence", func(t *testing.T) {
		// given - format 0 is longtext, so it must be defended like any other format
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		applyIdentity(t, fx, "author", model.RelationFormat_longtext, "_brauthor")
		rejections := captureLog(t)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeyRelationFormat, domain.Int64(int64(model.RelationFormat_number)))

		// when
		require.NoError(t, fx.Apply(s))

		// then
		assert.Equal(t, int64(model.RelationFormat_longtext), fx.Details().GetInt64(bundle.RelationKeyRelationFormat))
		assert.Len(t, rejections(), 1)
	})

	t.Run("clearing an identity detail is ignored", func(t *testing.T) {
		// given
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		applyIdentity(t, fx, "author", model.RelationFormat_object, "_brauthor")
		rejections := captureLog(t)
		s := fx.NewState()
		s.RemoveDetail(bundle.RelationKeyRelationKey, bundle.RelationKeySourceObject)

		// when
		require.NoError(t, fx.Apply(s))

		// then
		details := fx.Details()
		assert.Equal(t, "author", details.GetString(bundle.RelationKeyRelationKey))
		assert.Equal(t, "_brauthor", details.GetString(bundle.RelationKeySourceObject))
		assert.Len(t, rejections(), 2)
	})

	t.Run("a state stacked on top of another one is compared against the tree", func(t *testing.T) {
		// given - the intermediate state is where the bad value is introduced
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeRelation)
		applyIdentity(t, fx, "author", model.RelationFormat_object, "_brauthor")
		rejections := captureLog(t)
		intermediate := fx.NewState()
		intermediate.SetDetail(bundle.RelationKeyRelationKey, domain.String("hijacked"))
		s := intermediate.NewState()
		s.SetDetail(bundle.RelationKeyName, domain.String("Author"))

		// when
		require.NoError(t, fx.Apply(s))

		// then
		details := fx.Details()
		assert.Equal(t, "author", details.GetString(bundle.RelationKeyRelationKey))
		assert.Equal(t, "Author", details.GetString(bundle.RelationKeyName))
		assert.Len(t, rejections(), 1)
	})
}

func TestPreserveIdentityDetails_ObjectType(t *testing.T) {
	t.Run("sourceObject of an installed type can not be repointed", func(t *testing.T) {
		// given
		fx := newIdentityFixture(t, smartblock.SmartBlockTypeObjectType)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("_otpage"))
		require.NoError(t, fx.Apply(s))
		rejections := captureLog(t)

		// when
		s = fx.NewState()
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("_ottask"))
		require.NoError(t, fx.Apply(s))

		// then
		assert.Equal(t, "_otpage", fx.Details().GetString(bundle.RelationKeySourceObject))
		assert.Len(t, rejections(), 1)
	})
}

func TestPreserveIdentityDetails_UnrelatedType(t *testing.T) {
	t.Run("a page keeps no identity details, so sourceObject stays writable", func(t *testing.T) {
		// given - duplication repoints sourceObject on ordinary objects
		fx := newIdentityFixture(t, smartblock.SmartBlockTypePage)
		s := fx.NewState()
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("template1"))
		require.NoError(t, fx.Apply(s))
		rejections := captureLog(t)

		// when
		s = fx.NewState()
		s.SetDetail(bundle.RelationKeySourceObject, domain.String("template2"))
		require.NoError(t, fx.Apply(s))

		// then
		assert.Equal(t, "template2", fx.Details().GetString(bundle.RelationKeySourceObject))
		assert.Empty(t, rejections())
	})
}

func applyIdentity(t *testing.T, fx *fixture, relationKey string, format model.RelationFormat, sourceObject string) {
	s := fx.NewState()
	s.SetDetail(bundle.RelationKeyRelationKey, domain.String(relationKey))
	s.SetDetail(bundle.RelationKeyRelationFormat, domain.Int64(int64(format)))
	if sourceObject != "" {
		s.SetDetail(bundle.RelationKeySourceObject, domain.String(sourceObject))
	}
	require.NoError(t, fx.Apply(s))
	require.Equal(t, relationKey, fx.Details().GetString(bundle.RelationKeyRelationKey))
}
