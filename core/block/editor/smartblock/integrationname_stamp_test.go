package smartblock

// Creation-provenance stamping tests (APIV2_OBJECT_DELETE.md §11.4/§15).
// The assertions run at the PushChangeParams level — what the source is
// actually asked to write — via the recording sourceStub, so a green Apply
// alone cannot pass them.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// initAs mirrors the creation-path Init exactly as ObjectFactory.InitObject
// performs it (factory.go): the InitContext carries the request ctx and the
// IsNewObject flag, Init prepares initCtx.State, and the caller then applies
// that state. Keeping the ctx and the flag as parameters is what lets the
// tests below distinguish the creation path from an existing-object open —
// the distinction the stamp gates on.
func (fx *fixture) initAs(t *testing.T, ctx context.Context, isNewObject bool) *InitContext {
	// the IsNewObject init branch runs the sub-object links migration, which
	// consults the space kind
	fx.space.EXPECT().IsPersonal().Return(false).Maybe()
	doc := state.NewDoc(fx.source.id, map[string]simple.Block{
		fx.source.id: simple.New(&model.Block{Id: fx.source.id}),
	})
	fx.source.doc = doc
	initCtx := &InitContext{
		Ctx:         ctx,
		IsNewObject: isNewObject,
		SpaceID:     "space1",
		Source:      fx.source,
		Doc:         doc,
	}
	require.NoError(t, fx.Init(initCtx))
	return initCtx
}

// applyWithNewBlock adds one block to s and applies it the way the creation
// path does (no events, no history — factory.go's flag set minus the ones
// the fixture cannot satisfy), so every apply in these tests produces a
// pushable change set.
func (fx *fixture) applyWithNewBlock(t *testing.T, s *state.State, blockId string) {
	s.Add(simple.New(&model.Block{Id: blockId}))
	require.NoError(t, s.InsertTo(fx.source.id, model.Block_Inner, blockId))
	require.NoError(t, fx.Apply(s, NoHistory, NoEvent, NoRestrictions, KeepInternalFlags))
}

func TestSmartBlock_IntegrationNameStamping(t *testing.T) {
	ctxWithName := domain.CtxWithIntegrationName(context.Background(), "Claude Desktop")

	t.Run("creating apply stamps, the next apply does not — the per-apply leak guard", func(t *testing.T) {
		// given: a NEW object whose init ctx carries the API session raw app name —
		// the §8a creation shape shared by v1 and v2
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		initCtx := fx.initAs(t, ctxWithName, true)

		// when: the creating Apply (what factory.InitObject runs), then a
		// later local edit on the very same open object
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		fx.applyWithNewBlock(t, initCtx.State, "created-block")
		fx.applyWithNewBlock(t, fx.NewState(), "edited-later-block")

		// then: exactly the creating change carries the name. The first
		// assertion fails if the Init fill or the Apply plumbing is reverted;
		// the SECOND fails if the value is ever persisted on the doc/smartblock
		// instead of the per-apply state — the leak that would misattribute
		// every subsequent local edit and silently widen the DELETE gate.
		require.Len(t, fx.source.pushed, 2)
		assert.Equal(t, "Claude Desktop", fx.source.pushed[0].IntegrationName)
		assert.Equal(t, "", fx.source.pushed[1].IntegrationName)
	})

	t.Run("existing object opened under an API ctx is not stamped", func(t *testing.T) {
		// given: the SAME ctx, but IsNewObject false — an existing object
		// loaded during an API request (a PATCH, a migration-producing open).
		// This fixture can fail: an implementation that stamps from the ctx on
		// every apply — instead of gating on creation — pushes the name here.
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		fx.initAs(t, ctxWithName, false)

		// when
		fx.applyWithNewBlock(t, fx.NewState(), "block")

		// then
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "", fx.source.pushed[0].IntegrationName)
	})

	t.Run("creation without an API ctx is not stamped", func(t *testing.T) {
		// given: the UI/import/indexer shape — a new object, no carrier on the
		// ctx. Fails if the stamp ever stops being derived from the carrier
		// (e.g. a hardcoded or session-global value).
		fx := newFixture("root", t)
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)
		initCtx := fx.initAs(t, context.Background(), true)

		// when
		initCtx.State.SetChangeType(domain.ChangeTypeObjectInit)
		fx.applyWithNewBlock(t, initCtx.State, "block")

		// then
		require.Len(t, fx.source.pushed, 1)
		assert.Equal(t, "", fx.source.pushed[0].IntegrationName)
	})
}
