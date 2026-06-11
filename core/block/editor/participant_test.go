package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestParticipant_Init(t *testing.T) {
	t.Run("title block not empty, because name detail is in store", func(t *testing.T) {
		// given
		store := newStoreFixture(t)
		store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId: domain.String("spaceId"),
			bundle.RelationKeyId:      domain.String("root"),
			bundle.RelationKeyName:    domain.String("test"),
		}})

		p, err := newParticipantTestWithStore(t, store)
		require.NoError(t, err)

		// then
		assert.NotNil(t, p.NewState().Get(state.TitleBlockID))
		assert.Equal(t, "test", p.NewState().Get(state.TitleBlockID).Model().GetText().GetText())
	})
	t.Run("title block is empty", func(t *testing.T) {
		// given
		p, err := newParticipantTest(t)
		require.NoError(t, err)

		// then
		assert.NotNil(t, p.NewState().Get(state.TitleBlockID))
		assert.Equal(t, "", p.NewState().Get(state.TitleBlockID).Model().GetText().GetText())
	})
}

func newStoreFixture(t *testing.T) *spaceindex.StoreFixture {
	store := spaceindex.NewStoreFixture(t)

	for _, rel := range []domain.RelationKey{
		bundle.RelationKeyFeaturedRelations, bundle.RelationKeyIdentity, bundle.RelationKeyName,
		bundle.RelationKeyIdentityProfileLink, bundle.RelationKeyIsReadonly, bundle.RelationKeyIsArchived,
		bundle.RelationKeyDescription, bundle.RelationKeyIsHidden, bundle.RelationKeyResolvedLayout,
		bundle.RelationKeyLayoutAlign, bundle.RelationKeyIconImage, bundle.RelationKeyGlobalName,
		bundle.RelationKeyId, bundle.RelationKeyParticipantPermissions, bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeySpaceId, bundle.RelationKeyParticipantStatus, bundle.RelationKeyIsHiddenDiscovery,
	} {
		store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:     domain.String("space1"),
			bundle.RelationKeyUniqueKey:   domain.String(rel.URL()),
			bundle.RelationKeyId:          domain.String(rel.String()),
			bundle.RelationKeyRelationKey: domain.String(rel.String()),
		}})
	}

	return store
}

func newParticipantTest(t *testing.T) (*participant, error) {
	return newParticipantTestWithStore(t, newStoreFixture(t))
}

func newParticipantTestWithStore(t *testing.T, store spaceindex.Store) (*participant, error) {
	sb := smarttest.New("root")
	p := &participant{
		SmartBlock:  sb,
		objectStore: store,
	}

	initCtx := &smartblock.InitContext{
		IsNewObject: true,
		Doc:         sb.Doc,
	}

	if err := p.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(p, initCtx)
	if err := p.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return p, nil
}
