package smartblock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

// spaceConfigFixture builds a smartblock over a space configuration object - the workspace unless
// told otherwise - in a space the account may or may not manage.
func spaceConfigFixture(t *testing.T, canManage bool, sbType coresb.SmartBlockType) *fixture {
	fx := newFixture("workspaceId", t)
	fx.source.sbType = sbType
	fx.canManageSpace.Unset()
	fx.space.EXPECT().CanManageSpace().Return(canManage).Maybe()
	fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: "workspaceId"}).Maybe()
	fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil).Maybe()
	fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Maybe()
	fx.init(t, []*model.Block{{Id: "workspaceId"}})
	return fx
}

// renameState is the shape of an ordinary user edit: a details write with no change type set, which
// is what every editor and every RPC produces.
func renameState(fx *fixture, name string) *state.State {
	s := fx.NewState()
	s.SetDetail(bundle.RelationKeyName, domain.String(name))
	return s
}

func TestApplySpaceConfigLock(t *testing.T) {
	t.Run("plain member cannot edit the workspace", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypeWorkspace)

		err := fx.Apply(renameState(fx, "renamed by a writer"))

		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})

	t.Run("owner, admin and one-to-one members can edit the workspace", func(t *testing.T) {
		fx := spaceConfigFixture(t, true, coresb.SmartBlockTypeWorkspace)

		require.NoError(t, fx.Apply(renameState(fx, "renamed by an admin")))
	})

	t.Run("plain member cannot edit the widgets object", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypeWidget)

		assert.ErrorIs(t, fx.Apply(renameState(fx, "widgets")), restriction.ErrRestricted)
	})

	// Every automated writer to these objects stamps its own change type, so a reviser pass or a
	// migration on a plain member's device must go through untouched.
	t.Run("a system change type is not a user edit", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypeWorkspace)
		s := renameState(fx, "revised")
		s.SetChangeType(domain.ChangeTypeSystemObjectReviserMigration)

		require.NoError(t, fx.Apply(s))
	})

	// NoRestrictions only means "skip the block-level Edit restrictions", and basic.UpdateDetails
	// sets it on every details write - so it must NOT waive this lock, or rule 2 would be blind to
	// the single path that matters most.
	t.Run("NoRestrictions does not waive the lock", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypeWorkspace)

		err := fx.Apply(renameState(fx, "sneaked in"), NoRestrictions)

		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})

	t.Run("NoSpaceConfigCheck waives the lock for internal writers", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypeWorkspace)

		require.NoError(t, fx.Apply(renameState(fx, "bootstrap"), NoSpaceConfigCheck))
	})

	// An apply that never reaches the tree cannot break a rule about what syncs. The space chat
	// writes its derived details this way on every member's device.
	t.Run("NotPushChanges waives the lock", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypeWorkspace)

		require.NoError(t, fx.Apply(renameState(fx, "local only"), NotPushChanges))
	})

	t.Run("ordinary content is untouched", func(t *testing.T) {
		fx := spaceConfigFixture(t, false, coresb.SmartBlockTypePage)

		require.NoError(t, fx.Apply(renameState(fx, "a page a writer may rename")))
	})
}

// The creator relation is derived, so it lives in LOCAL details, and nothing on the details write
// path rejects a write to it: ObjectSetDetails can point it at another participant and
// injectCreationInfo keeps the forged value on every later load. The policy must read the signed
// tree root instead, so a forged detail can neither grant nor withhold a delete.
func TestMemberPolicyIgnoresCreatorDetail(t *testing.T) {
	fx := spaceConfigFixture(t, false, coresb.SmartBlockTypePage)
	fx.smartBlock.currentParticipantId = "participantMe"

	st := fx.NewState()
	st.SetDetail(bundle.RelationKeyCreator, domain.String("participantSomebodyElse"))
	require.NoError(t, fx.Apply(st))

	require.Equal(t, "participantSomebodyElse", fx.LocalDetails().GetString(bundle.RelationKeyCreator),
		"the forged value is stored - that is the whole problem")
	assert.False(t, fx.MemberPolicy().LockForeignDelete,
		"the unsigned stub root carries no identity, so the policy must stay silent rather than trust the detail")
	assert.Empty(t, fx.treeCreator())
}
