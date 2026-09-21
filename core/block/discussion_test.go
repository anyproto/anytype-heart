package block

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator/mock_objectcreator"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

// discussionFixture wires ObjectAddDiscussion's four dependencies around a
// parent object held in memory, so the test can read back what the parent
// link write left on it.
type discussionFixture struct {
	*Service
	spc     *mock_clientspace.MockSpace
	creator *mock_objectcreator.MockService
	parent  *smarttest.SmartTest
}

func newDiscussionFixture(t *testing.T, spaceId, parentId string) *discussionFixture {
	resolver := mock_idresolver.NewMockResolver(t)
	resolver.EXPECT().ResolveSpaceID(parentId).Return(spaceId, nil)
	spc := mock_clientspace.NewMockSpace(t)
	spaceSvc := mock_space.NewMockService(t)
	spaceSvc.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil)
	accountSvc := mock_account.NewMockService(t)
	accountSvc.EXPECT().AccountID().Return("account1").Maybe()
	creator := mock_objectcreator.NewMockService(t)
	return &discussionFixture{
		Service: &Service{resolver: resolver, spaceService: spaceSvc, accountService: accountSvc, objectCreator: creator},
		spc:     spc,
		creator: creator,
		parent:  smarttest.New(parentId),
	}
}

// expectParentLink makes the space run the parent DoCtx callback against
// the in-memory parent, and the discussion's subscriber DoCtx fail — the
// subscribe is best effort (logged), and failing it here keeps the fixture
// to one smartblock.
func (fx *discussionFixture) expectParentLink(discussionId, parentId string) {
	fx.spc.EXPECT().DoCtx(mock.Anything, discussionId, mock.Anything).Return(errors.New("discussion not loaded in this test")).Once()
	fx.spc.EXPECT().DoCtx(mock.Anything, parentId, mock.Anything).RunAndReturn(
		func(_ context.Context, _ string, apply func(smartblock.SmartBlock) error) error {
			return apply(fx.parent)
		}).Once()
}

func TestService_ObjectAddDiscussion(t *testing.T) {
	const (
		spaceId  = "space1"
		parentId = "page1"
	)
	uk := domain.MustUniqueKey(coresb.SmartBlockTypeDiscussionObject, parentId)

	t.Run("a fresh mint links the parent to the created id", func(t *testing.T) {
		// given
		fx := newDiscussionFixture(t, spaceId, parentId)
		fx.creator.EXPECT().AddDiscussionDerivedObject(mock.Anything, fx.spc, parentId).Return("disc-new", nil).Once()
		fx.expectParentLink("disc-new", parentId)

		// when
		got, err := fx.ObjectAddDiscussion(context.Background(), parentId)

		// then
		require.NoError(t, err)
		assert.Equal(t, "disc-new", got)
		assert.Equal(t, "disc-new", fx.parent.Details().GetString(bundle.RelationKeyDiscussionId))
	})

	t.Run("a tree that already exists is recovered by the SAME derivation the create used, parent id included", func(t *testing.T) {
		// given: an earlier attempt created the tree but never linked the
		// parent (a crash, or another device racing); the create derives
		// WITH the parent id, and in a shared space a key-only derivation
		// would name a different tree
		fx := newDiscussionFixture(t, spaceId, parentId)
		fx.creator.EXPECT().AddDiscussionDerivedObject(mock.Anything, fx.spc, parentId).
			Return("", fmt.Errorf("create discussion: create smartblock from state: put tree: %w", treestorage.ErrTreeExists)).Once()
		fx.spc.EXPECT().DeriveTreePayload(mock.Anything, payloadcreator.PayloadDerivationParams{Key: uk, ParentId: parentId}).
			Return(treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "disc-existing"}}, nil).Once()
		fx.expectParentLink("disc-existing", parentId)

		// when
		got, err := fx.ObjectAddDiscussion(context.Background(), parentId)

		// then
		require.NoError(t, err)
		assert.Equal(t, "disc-existing", got)
		assert.Equal(t, "disc-existing", fx.parent.Details().GetString(bundle.RelationKeyDiscussionId),
			"the retry finishes the link a crash left undone")
	})

	t.Run("any other create failure surfaces and links nothing", func(t *testing.T) {
		// given: no DeriveTreePayload, no DoCtx expectation
		fx := newDiscussionFixture(t, spaceId, parentId)
		fx.creator.EXPECT().AddDiscussionDerivedObject(mock.Anything, fx.spc, parentId).Return("", errors.New("boom")).Once()

		// when
		_, err := fx.ObjectAddDiscussion(context.Background(), parentId)

		// then
		require.ErrorContains(t, err, "add discussion derived object: boom")
		assert.Empty(t, fx.parent.Details().GetString(bundle.RelationKeyDiscussionId))
	})

	t.Run("a failed parent link is an error, not a success with a dangling tree", func(t *testing.T) {
		fx := newDiscussionFixture(t, spaceId, parentId)
		fx.creator.EXPECT().AddDiscussionDerivedObject(mock.Anything, fx.spc, parentId).Return("disc-new", nil).Once()
		fx.spc.EXPECT().DoCtx(mock.Anything, "disc-new", mock.Anything).Return(nil).Once()
		fx.spc.EXPECT().DoCtx(mock.Anything, parentId, mock.Anything).Return(errors.New("insufficient permissions")).Once()

		_, err := fx.ObjectAddDiscussion(context.Background(), parentId)

		require.ErrorContains(t, err, "set discussionId on parent object")
	})
}
