package smartblock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSmartBlock_getDetailsFromStore(t *testing.T) {
	id := "id"
	t.Run("details are in the store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"id":     domain.String(id),
			"number": domain.Float64(2.18281828459045),
			"🔥":      domain.StringList([]string{"Jeanne d'Arc", "Giordano Bruno", "Capocchio"}),
		})

		err := fx.store.UpdateObjectDetails(context.Background(), id, details)
		require.NoError(t, err)

		// when
		detailsFromStore, err := fx.getDetailsFromStore()

		// then
		assert.NoError(t, err)
		assert.Equal(t, details, detailsFromStore)
	})

	t.Run("no details in the store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		// when
		details, err := fx.getDetailsFromStore()

		// then
		assert.NoError(t, err)
		assert.NotNil(t, details)
	})
}

func TestSmartBlock_injectBackLinks(t *testing.T) {
	backLinks := []string{"1", "2", "3"}
	id := "id"

	t.Run("update back links", func(t *testing.T) {
		// given
		newBackLinks := []string{"4", "5"}
		fx := newFixture(id, t)

		ctx := context.Background()
		err := fx.store.UpdateObjectLinks(ctx, "4", []string{id})
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks(ctx, "5", []string{id})
		require.NoError(t, err)

		st := state.NewDoc("", nil).NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyBacklinks, domain.StringList(backLinks))

		// when
		fx.updateBackLinks(st)

		// then
		assert.Equal(t, newBackLinks, st.CombinedDetails().GetStringList(bundle.RelationKeyBacklinks))
	})

	t.Run("back links were found in object store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		ctx := context.Background()
		err := fx.store.UpdateObjectLinks(ctx, "1", []string{id})
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks(ctx, "2", []string{id})
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks(ctx, "3", []string{id})
		require.NoError(t, err)

		// fx.store.EXPECT().GetInboundLinksById(id).Return(backLinks, nil)
		st := state.NewDoc("", nil).NewState()

		// when
		fx.updateBackLinks(st)

		// then
		details := st.CombinedDetails()
		assert.NotNil(t, details.GetStringList(bundle.RelationKeyBacklinks))
		assert.Equal(t, backLinks, details.GetStringList(bundle.RelationKeyBacklinks))
	})

	t.Run("back links were not found in object store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("", nil).NewState()

		// when
		fx.updateBackLinks(st)

		// then
		assert.Len(t, st.CombinedDetails().GetStringList(bundle.RelationKeyBacklinks), 0)
	})
}

func TestSmartBlock_updatePendingDetails(t *testing.T) {
	id := "id"

	t.Run("no pending details", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		var hasPendingDetails bool
		details := domain.NewDetails()

		// when
		_, result := fx.appendPendingDetails(details)

		// then
		assert.Equal(t, hasPendingDetails, result)
		assert.Zero(t, details.Len())
	})

	t.Run("found pending details", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		details := domain.NewDetails()

		err := fx.store.UpdatePendingLocalDetails(id, func(det *domain.Details) (*domain.Details, error) {
			det.Set(bundle.RelationKeyIsDeleted, domain.Bool(false))
			return det, nil
		})
		require.NoError(t, err)

		// when
		got, _ := fx.appendPendingDetails(details)

		// then
		want := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:        domain.String(id),
			bundle.RelationKeyIsDeleted: domain.Bool(false),
		})
		assert.Equal(t, want, got)
	})

	t.Run("failure on retrieving pending details from the store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		details := domain.NewDetails()

		// when
		_, hasPendingDetails := fx.appendPendingDetails(details)

		// then
		assert.False(t, hasPendingDetails)
	})
}

func TestSmartBlock_injectCreationInfo(t *testing.T) {
	creator := "Anytype"
	creationDate := int64(1692127254)

	t.Run("both creator and creation date are already set", func(t *testing.T) {
		// given
		src := &sourceStub{
			creator:     creator,
			createdDate: creationDate,
			err:         nil,
		}
		sb := &smartBlock{source: src}
		s := &state.State{}
		s.SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyCreator:     domain.String(creator),
			bundle.RelationKeyCreatedDate: domain.Int64(creationDate),
		}))

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.NoError(t, err)
		assert.Equal(t, creator, s.LocalDetails().GetString(bundle.RelationKeyCreator))
		assert.Equal(t, creationDate, s.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate))
	})

	t.Run("both creator and creation date are found", func(t *testing.T) {
		// given
		src := &sourceStub{
			creator:     creator,
			createdDate: creationDate,
			err:         nil,
		}
		sb := smartBlock{source: src}
		s := &state.State{}

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.NoError(t, err)
		assert.Equal(t, creator, s.LocalDetails().GetString(bundle.RelationKeyCreator))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyCreator.String()))
		assert.Equal(t, creationDate, s.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyCreatedDate.String()))
	})

	t.Run("failure on retrieving creation info from source", func(t *testing.T) {
		// given
		srcErr := errors.New("source error")
		src := &sourceStub{err: srcErr}
		sb := smartBlock{source: src}
		s := &state.State{}

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.True(t, errors.Is(err, srcErr))
		assert.Nil(t, s.LocalDetails())
	})
}

func TestInjectLocalDetails(t *testing.T) {
	t.Run("with no details in store get creation info from source", func(t *testing.T) {
		const id = "id"

		fx := newFixture(id, t)
		fx.source.creator = domain.NewParticipantId("testSpace", "testIdentity")
		fx.source.createdDate = time.Now().Unix()

		st := state.NewDoc("id", nil).NewState()

		err := fx.injectLocalDetails(st)

		require.NoError(t, err)

		assert.Equal(t, fx.source.creator, st.LocalDetails().GetString(bundle.RelationKeyCreator))
		assert.Equal(t, fx.source.createdDate, st.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate))
	})

	// TODO More tests
}

func TestInjectDerivedDetails(t *testing.T) {
	const (
		id      = "id"
		spaceId = "testSpace"
	)
	t.Run("links are updated on injection", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", map[string]simple.Block{
			id:         simple.New(&model.Block{Id: id, ChildrenIds: []string{"dataview", "link"}}),
			"dataview": simple.New(&model.Block{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{TargetObjectId: "some_set"}}}),
			"link":     simple.New(&model.Block{Id: "link", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "some_obj"}}}),
		}).NewState()
		st.AddRelationLinks(&model.RelationLink{Key: bundle.RelationKeyAssignee.String(), Format: model.RelationFormat_object})
		st.SetDetail(bundle.RelationKeyAssignee, domain.StringList([]string{"Kirill"}))

		// when
		fx.injectDerivedDetails(st, spaceId, smartblock.SmartBlockTypePage)

		// then
		assert.Len(t, st.LocalDetails().GetStringList(bundle.RelationKeyLinks), 3)
	})
}
