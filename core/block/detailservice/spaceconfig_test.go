package detailservice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Apply refuses these writes too, but only after building and merging the state. Answering at the
// service boundary leaves the loaded document untouched and gives the caller ErrRestricted.
func TestSetDetailsRefusesRestrictedObject(t *testing.T) {
	const objectId = "objectId"

	// A plain member looking at the workspace: restrictions are recomputed from the holder, so the
	// object has to be a real space configuration object rather than one with a planted restriction.
	newObject := func(plainMember bool) *smarttest.SmartTest {
		sb := smarttest.New(objectId)
		sb.SetType(coresb.SmartBlockTypeWorkspace)
		if plainMember {
			sb.TestMemberPolicy = restriction.MemberPolicy{LockSpaceConfig: true}
		}
		return sb
	}

	rename := []domain.Detail{{Key: bundle.RelationKeyName, Value: domain.String("renamed")}}

	t.Run("details write to a restricted object is refused", func(t *testing.T) {
		fx := newFixture(t)
		sb := newObject(true)
		fx.getter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		err := fx.SetDetails(nil, objectId, rename)

		assert.ErrorIs(t, err, restriction.ErrRestricted)
		assert.Empty(t, sb.Details().GetString(bundle.RelationKeyName), "the document must be left untouched")
	})

	t.Run("details write to an unrestricted object goes through", func(t *testing.T) {
		fx := newFixture(t)
		sb := newObject(false)
		fx.getter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		require.NoError(t, fx.SetDetails(nil, objectId, rename))
		assert.Equal(t, "renamed", sb.Details().GetString(bundle.RelationKeyName))
	})

	t.Run("ModifyDetails is refused on a restricted object", func(t *testing.T) {
		fx := newFixture(t)
		fx.getter.EXPECT().GetObject(mock.Anything, objectId).Return(newObject(true), nil)

		err := fx.ModifyDetails(nil, objectId, func(current *domain.Details) (*domain.Details, error) {
			current.SetString(bundle.RelationKeyName, "renamed")
			return current, nil
		})

		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})

	// Migrations and bootstrap must reach the workspace on every member's device, including plain
	// writers who may not edit it themselves.
	t.Run("SetDetailsInternal ignores the restriction", func(t *testing.T) {
		fx := newFixture(t)
		sb := newObject(true)
		fx.getter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

		require.NoError(t, fx.SetDetailsInternal(objectId, rename))
		assert.Equal(t, "renamed", sb.Details().GetString(bundle.RelationKeyName))
	})
}

var _ smartblock.SmartBlock = (*smarttest.SmartTest)(nil)

// The account object carries objRestrictAll by its sbType alone, with no ACL input, and account
// creation writes its own profile details through SetDetails. Enforcing that base restriction here
// broke account creation and every integration test with it; the guard is the ACL lock only.
func TestSetDetailsAllowsBaseRestrictedObject(t *testing.T) {
	const objectId = "accountObjectId"

	fx := newFixture(t)
	sb := smarttest.New(objectId)
	sb.SetType(coresb.SmartBlockTypeAccountObject)
	fx.getter.EXPECT().GetObject(mock.Anything, objectId).Return(sb, nil)

	require.Error(t, restriction.CheckRestrictions(sb, model.Restrictions_Details),
		"precondition: the account object restricts details by its own nature")

	err := fx.SetDetails(nil, objectId, []domain.Detail{
		{Key: bundle.RelationKeyName, Value: domain.String("account name")},
	})

	require.NoError(t, err)
	assert.Equal(t, "account name", sb.Details().GetString(bundle.RelationKeyName))
}
