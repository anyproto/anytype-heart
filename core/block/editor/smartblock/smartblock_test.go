package smartblock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/restriction/mock_restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
)

func TestSmartBlock_Init(t *testing.T) {
	// given
	id := "one"
	fx := newFixture(id, t)

	// when
	initCtx := fx.init(t, []*model.Block{{Id: id}})

	require.NotNil(t, initCtx)
	require.NotNil(t, initCtx.State)
	links := initCtx.State.GetRelationLinks()
	for _, key := range bundle.RequiredInternalRelations {
		assert.Truef(t, links.Has(key.String()), "missing relation %s", key)
	}
	// then
	assert.Equal(t, id, fx.RootId())
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		// given
		fx := newFixture("", t)

		fx.restrictionService.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})

		fx.init(t, []*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))
		var event *pb.Event
		ctx := session.NewContext()
		fx.RegisterSession(ctx)
		fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Run(func(token string, e *pb.Event) {
			event = e
		})
		fx.indexer.EXPECT().Index(mock.Anything, mock.Anything).Return(nil)

		// when
		err := fx.Apply(s)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, fx.History().Len())
		assert.NotNil(t, event)
	})

}

func TestBasic_SetAlign(t *testing.T) {
	t.Run("with ids", func(t *testing.T) {
		// given
		fx := newFixture("", t)

		fx.restrictionService.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		fx.init(t, []*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.NewState()

		// when
		err := st.SetAlign(model.Block_AlignRight, "2", "3")

		// then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.NewState().Get("2").Model().Align)
	})

	t.Run("without ids", func(t *testing.T) {
		// given
		fx := newFixture("", t)

		fx.restrictionService.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		fx.init(t, []*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.NewState()

		// when
		err := st.SetAlign(model.Block_AlignRight)

		// then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.Get("title").Model().Align)
		assert.Equal(t, int64(model.Block_AlignRight), st.Details().GetInt64(bundle.RelationKeyLayoutAlign))
	})

}

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

func Test_removeInternalFlags(t *testing.T) {
	t.Run("no flags - no changes", func(t *testing.T) {
		// given
		st := state.NewDoc("test", nil).(*state.State)
		st.SetDetail(bundle.RelationKeyInternalFlags, domain.Int64List([]int64{}))

		// when
		removeInternalFlags(st)

		// then
		assert.Empty(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags))
	})
	t.Run("EmptyDelete flag is not removed when state is empty", func(t *testing.T) {
		// given
		st := state.NewDoc("test", nil).(*state.State)
		flags := defaultInternalFlags()
		flags.AddToState(st)

		// when
		removeInternalFlags(st)

		// then
		assert.Len(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags), 1)
	})
	t.Run("all flags are removed when title is not empty", func(t *testing.T) {
		// given
		st := state.NewDoc("test", map[string]simple.Block{
			"title": simple.New(&model.Block{Id: "title"}),
		}).(*state.State)
		st.SetDetail(bundle.RelationKeyName, domain.String("some name"))
		flags := defaultInternalFlags()
		flags.AddToState(st)

		// when
		removeInternalFlags(st)

		// then
		assert.Empty(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags))
	})
	t.Run("all flags are removed when state has non-empty text blocks", func(t *testing.T) {
		// given
		st := state.NewDoc("test", map[string]simple.Block{
			"test": simple.New(&model.Block{Id: "test", ChildrenIds: []string{"text"}}),
			"text": simple.New(&model.Block{Id: "text", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "some text"},
			}}),
		}).(*state.State)
		flags := defaultInternalFlags()
		flags.AddToState(st)

		// when
		removeInternalFlags(st)

		// then
		assert.Empty(t, st.CombinedDetails().GetInt64List(bundle.RelationKeyInternalFlags))
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

type fixture struct {
	objectStore        *objectstore.StoreFixture
	store              spaceindex.Store
	restrictionService *mock_restriction.MockService
	indexer            *MockIndexer
	eventSender        *mock_event.MockSender
	source             *sourceStub
	spaceIdResolver    *mock_idresolver.MockResolver

	*smartBlock
}

const testSpaceId = "space1"

func newFixture(id string, t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	spaceIndex := objectStore.SpaceIndex(testSpaceId)

	spaceIdResolver := mock_idresolver.NewMockResolver(t)

	indexer := NewMockIndexer(t)

	restrictionService := mock_restriction.NewMockService(t)
	restrictionService.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{}).Maybe()

	sender := mock_event.NewMockSender(t)

	sb := New(nil, "", nil, restrictionService, spaceIndex, objectStore, indexer, sender, spaceIdResolver).(*smartBlock)
	source := &sourceStub{
		id:      id,
		spaceId: "space1",
		sbType:  smartblock.SmartBlockTypePage,
	}
	sb.source = source

	return &fixture{
		source:             source,
		smartBlock:         sb,
		store:              spaceIndex,
		restrictionService: restrictionService,
		indexer:            indexer,
		eventSender:        sender,
		spaceIdResolver:    spaceIdResolver,
		objectStore:        objectStore,
	}
}

func (fx *fixture) init(t *testing.T, blocks []*model.Block) *InitContext {
	bm := make(map[string]simple.Block)
	for _, b := range blocks {
		bm[b.Id] = simple.New(b)
	}
	doc := state.NewDoc(fx.source.id, bm)
	fx.source.doc = doc

	initCtx := &InitContext{
		Ctx:     context.Background(),
		SpaceID: "space1",
		Source:  fx.source,
	}
	err := fx.Init(initCtx)
	require.NoError(t, err)
	return initCtx
}

type sourceStub struct {
	spaceId     string
	creator     string
	createdDate int64
	sbType      smartblock.SmartBlockType
	err         error
	doc         state.Doc
	id          string
}

func (s *sourceStub) GetCreationInfo() (creator string, createdDate int64, err error) {
	return s.creator, s.createdDate, s.err
}

func (s *sourceStub) Id() string                                { return s.id }
func (s *sourceStub) SpaceID() string                           { return s.spaceId }
func (s *sourceStub) Type() smartblock.SmartBlockType           { return s.sbType }
func (s *sourceStub) Heads() []string                           { return nil }
func (s *sourceStub) GetFileKeysSnapshot() []*pb.ChangeFileKeys { return nil }
func (s *sourceStub) ReadOnly() bool                            { return false }
func (s *sourceStub) Close() (err error)                        { return nil }
func (s *sourceStub) ReadDoc(_ context.Context, _ source.ChangeReceiver, _ bool) (doc state.Doc, err error) {
	return s.doc, nil
}
func (s *sourceStub) PushChange(_ source.PushChangeParams) (id string, err error) {
	return "", nil
}

func defaultInternalFlags() (flags internalflag.Set) {
	flags.Add(model.InternalFlag_editorDeleteEmpty)
	flags.Add(model.InternalFlag_editorSelectType)
	flags.Add(model.InternalFlag_editorSelectTemplate)
	return
}
