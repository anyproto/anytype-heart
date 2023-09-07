package smartblock

import (
	"context"
	"errors"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/restriction/mock_restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
	"github.com/anyproto/anytype-heart/util/testMock/mockRelation"
	"github.com/anyproto/anytype-heart/util/testMock/mockSource"

	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
	_ "github.com/anyproto/anytype-heart/core/block/simple/link"
	_ "github.com/anyproto/anytype-heart/core/block/simple/text"
)

func TestSmartBlock_Init(t *testing.T) {
	// given
	id := "one"
	fx := newFixture(t)
	defer fx.tearDown()
	fx.at.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
	fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
		Details: &types.Struct{Fields: map[string]*types.Value{}},
	}, nil)
	fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
	fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
	fx.restrict.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})

	// when
	fx.init([]*model.Block{{Id: id}})

	// then
	assert.Equal(t, id, fx.sb.RootId())
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.at.EXPECT().PredefinedBlocks()
		fx.at.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
		fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil)
		fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
		fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
		fx.restrict.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})

		fx.init([]*model.Block{{Id: "1"}})
		s := fx.sb.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))
		fx.source.EXPECT().ReadOnly()
		var event *pb.Event
		fx.sb.SetEventFunc(func(e *pb.Event) {
			event = e
		})
		fx.source.EXPECT().Heads()
		fx.source.EXPECT().PushChange(gomock.Any()).Return("fake_change_id", nil)
		fx.indexer.EXPECT().Index(gomock.Any(), gomock.Any())

		// when
		err := fx.sb.Apply(s)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, fx.sb.History().Len())
		assert.NotNil(t, event)
	})

}

func TestBasic_SetAlign(t *testing.T) {
	t.Run("with ids", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.at.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
		fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil)
		fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
		fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
		fx.restrict.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		fx.init([]*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.sb.NewState()

		// when
		err := st.SetAlign(model.Block_AlignRight, "2", "3")

		// then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.NewState().Get("2").Model().Align)
	})

	t.Run("without ids", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.at.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
		fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil)
		fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
		fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
		fx.restrict.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		fx.init([]*model.Block{
			{Id: "test", ChildrenIds: []string{"title", "2"}},
			{Id: "title"},
			{Id: "2"},
		})
		st := fx.sb.NewState()

		// when
		err := st.SetAlign(model.Block_AlignRight)

		// then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.Get("title").Model().Align)
		assert.Equal(t, int64(model.Block_AlignRight), pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayoutAlign.String()))
	})

}

func TestSmartBlock_getDetailsFromStore(t *testing.T) {
	id := "id"
	t.Run("details are in the store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		details := &types.Struct{
			Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"number": pbtypes.Float64(2.18281828459045),
				"🔥":      pbtypes.StringList([]string{"Jeanne d'Arc", "Giordano Bruno", "Capocchio"}),
			},
		}
		fx.source.EXPECT().Id().Return(id)
		fx.store.EXPECT().GetDetails(id).Return(&model.ObjectDetails{Details: details}, nil)

		// when
		detailsFromStore, err := fx.sb.getDetailsFromStore()

		// then
		assert.NoError(t, err)
		assert.Equal(t, details, detailsFromStore)
	})

	t.Run("no details in the store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		fx.store.EXPECT().GetDetails(id).Return(nil, nil)

		// when
		details, err := fx.sb.getDetailsFromStore()

		// then
		assert.NoError(t, err)
		assert.Nil(t, details)
	})

	t.Run("failure on retrieving details from store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		details := &model.ObjectDetails{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"someKey": pbtypes.String("someValue"),
			},
		}}
		someErr := errors.New("some error")
		fx.source.EXPECT().Id().Return(id)
		fx.store.EXPECT().GetDetails(id).Return(details, someErr)

		// when
		detailsFromStore, err := fx.sb.getDetailsFromStore()

		// then
		assert.True(t, errors.Is(err, someErr))
		assert.Nil(t, detailsFromStore)
	})
}

func TestSmartBlock_injectWorkspaceID(t *testing.T) {
	wID := "space"
	id := "id"

	t.Run("workspaceID is already set", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Times(0)
		fx.at.EXPECT().GetWorkspaceIdForObject(gomock.Any()).Times(0)
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyWorkspaceId.String(): pbtypes.String(wID),
		}})

		// when
		fx.sb.injectWorkspaceID(s)

		// then
		assert.Equal(t, wID, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyWorkspaceId.String()))
	})

	t.Run("set workspaceID from core service", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		fx.at.EXPECT().GetWorkspaceIdForObject(id).Return(wID, nil)
		s := &state.State{}

		// when
		fx.sb.injectWorkspaceID(s)

		// then
		assert.Equal(t, wID, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyWorkspaceId.String()))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyWorkspaceId.String()))
	})

	t.Run("object is deleted, so it does not belong to workspace", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id).Times(1)
		fx.at.EXPECT().GetWorkspaceIdForObject(id).Return("", core.ErrObjectDoesNotBelongToWorkspace)
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true),
		}})

		// when
		fx.sb.injectWorkspaceID(s)

		// then
		spaceID, found := s.LocalDetails().Fields[bundle.RelationKeyWorkspaceId.String()]
		assert.Nil(t, spaceID)
		assert.False(t, found)
		assert.Nil(t, s.GetRelationLinks())
	})

	t.Run("object is deleted, but core returned other error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id).Times(2)
		fx.at.EXPECT().GetWorkspaceIdForObject(id).Return("", errors.New("some error from core"))
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true),
		}})

		// when
		fx.sb.injectWorkspaceID(s)

		// then
		spaceID, found := s.LocalDetails().Fields[bundle.RelationKeyWorkspaceId.String()]
		assert.Nil(t, spaceID)
		assert.False(t, found)
		assert.Nil(t, s.GetRelationLinks())
	})

	t.Run("failure on retrieving workspaceID from core service", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id).Times(1)
		fx.at.EXPECT().GetWorkspaceIdForObject(id).Return("", errors.New("some error from core"))
		s := &state.State{}

		// when
		fx.sb.injectWorkspaceID(s)

		// then
		assert.Nil(t, s.LocalDetails())
		assert.Nil(t, s.GetRelationLinks())
	})
}

func TestSmartBlock_injectBackLinks(t *testing.T) {
	backLinks := []string{"1", "2", "3"}
	id := "id"

	t.Run("update back links", func(t *testing.T) {
		// given
		newBackLinks := []string{"4", "5"}
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		fx.store.EXPECT().GetInboundLinksByID(id).Return(newBackLinks, nil)
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyBacklinks.String(): pbtypes.StringList(backLinks),
		}}

		// when
		fx.sb.updateBackLinks(details)

		// then
		assert.Equal(t, newBackLinks, pbtypes.GetStringList(details, bundle.RelationKeyBacklinks.String()))
	})

	t.Run("back links were found in object store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		fx.store.EXPECT().GetInboundLinksByID(id).Return(backLinks, nil)
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		// when
		fx.sb.updateBackLinks(details)

		// then
		assert.NotNil(t, pbtypes.GetStringList(details, bundle.RelationKeyBacklinks.String()))
		assert.Equal(t, backLinks, pbtypes.GetStringList(details, bundle.RelationKeyBacklinks.String()))
	})

	t.Run("back links were not found in object store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		fx.store.EXPECT().GetInboundLinksByID(id).Return(nil, nil)
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		// when
		fx.sb.updateBackLinks(details)

		// then
		assert.Len(t, pbtypes.GetStringList(details, bundle.RelationKeyBacklinks.String()), 0)
	})

	t.Run("failure on retrieving back links from the store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id).Times(2)
		fx.store.EXPECT().GetInboundLinksByID(id).Return(nil, errors.New("some error from store"))
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		// when
		fx.sb.updateBackLinks(details)

		// then
		assert.Zero(t, len(details.Fields))
	})
}

func TestSmartBlock_updatePendingDetails(t *testing.T) {
	id := "id"

	t.Run("no pending details", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		var hasPendingDetails bool
		details := &types.Struct{Fields: map[string]*types.Value{}}
		fx.store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).Return(nil).
			Do(func(id string, f func(*types.Struct) (*types.Struct, error)) { hasPendingDetails = false })

		// when
		_, result := fx.sb.appendPendingDetails(details)

		// then
		assert.Equal(t, hasPendingDetails, result)
		assert.Zero(t, len(details.Fields))
	})

	t.Run("found pending details", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id)
		details := &types.Struct{Fields: map[string]*types.Value{}}
		fx.store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).Return(nil).Do(func(id string, f func(details *types.Struct) (*types.Struct, error)) {
			details.Fields[bundle.RelationKeyIsDeleted.String()] = pbtypes.Bool(false)
		})

		// when
		got, _ := fx.sb.appendPendingDetails(details)

		// then
		assert.Len(t, got.Fields, 1)
	})

	t.Run("failure on retrieving pending details from the store", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		fx.source.EXPECT().Id().Return(id).Times(2)
		fx.store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).Return(errors.New("some error from store"))
		details := &types.Struct{}

		// when
		_, hasPendingDetails := fx.sb.appendPendingDetails(details)

		// then
		assert.False(t, hasPendingDetails)
	})
}

func TestSmartBlock_injectCreationInfo(t *testing.T) {
	creator := "Anytype"
	creationDate := int64(1692127254)

	t.Run("both creator and creation date are already set", func(t *testing.T) {
		// given
		sb := &smartBlock{}
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyCreator.String():     pbtypes.String(creator),
			bundle.RelationKeyCreatedDate.String(): pbtypes.Int64(creationDate),
		}})

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.NoError(t, err)
		assert.Equal(t, creator, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyCreator.String()))
		assert.Equal(t, creationDate, pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyCreatedDate.String()))
	})

	t.Run("source could not be converted to CreationInfoProvider", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.tearDown()
		s := &state.State{}

		// when
		err := fx.sb.injectCreationInfo(s)

		// then
		assert.NoError(t, err)
		assert.Nil(t, s.LocalDetails())
	})

	t.Run("both creator and creation date are found", func(t *testing.T) {
		// given
		src := &creationInfoProvider{
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
		assert.Equal(t, creator, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyCreator.String()))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyCreator.String()))
		assert.Equal(t, creationDate, pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyCreatedDate.String()))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyCreatedDate.String()))
	})

	t.Run("failure on retrieving creation info from source", func(t *testing.T) {
		// given
		srcErr := errors.New("source error")
		src := &creationInfoProvider{err: srcErr}
		sb := smartBlock{source: src}
		s := &state.State{}

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.True(t, errors.Is(err, srcErr))
		assert.Nil(t, s.LocalDetails())
	})
}

type fixture struct {
	t        *testing.T
	ctrl     *gomock.Controller
	source   *mockSource.MockSource
	at       *testMock.MockService
	store    *testMock.MockObjectStore
	restrict *mock_restriction.MockService
	indexer  *MockIndexer
	sb       *smartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)

	coreService := testMock.NewMockService(ctrl)
	coreService.EXPECT().ProfileID().Return("").AnyTimes()

	source := mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(model.SmartBlockType_Page)

	objectStore := testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().GetObjectType(gomock.Any()).AnyTimes()
	objectStore.EXPECT().Name().Return(objectstore.CName).AnyTimes()

	indexer := NewMockIndexer(ctrl)
	indexer.EXPECT().Name().Return("indexer").AnyTimes()

	restrictionService := mock_restriction.NewMockService(t)

	relationService := mockRelation.NewMockService(ctrl)

	fileService := testMock.NewMockFileService(ctrl)

	return &fixture{
		sb: &smartBlock{
			source:             source,
			coreService:        coreService,
			fileService:        fileService,
			restrictionService: restrictionService,
			objectStore:        objectStore,
			relationService:    relationService,
			indexer:            indexer,
		},
		t:        t,
		at:       coreService,
		ctrl:     ctrl,
		store:    objectStore,
		source:   source,
		restrict: restrictionService,
		indexer:  indexer,
	}
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}

func (fx *fixture) init(blocks []*model.Block) {
	id := blocks[0].Id
	bm := make(map[string]simple.Block)
	for _, b := range blocks {
		bm[b.Id] = simple.New(b)
	}
	doc := state.NewDoc(id, bm)
	fx.source.EXPECT().ReadDoc(context.Background(), gomock.Any(), false).Return(doc, nil)
	fx.source.EXPECT().Id().Return(id).AnyTimes()

	err := fx.sb.Init(&InitContext{Source: fx.source})
	require.NoError(fx.t, err)
}

type creationInfoProvider struct {
	creator     string
	createdDate int64
	err         error
}

func (p *creationInfoProvider) GetCreationInfo() (creator string, createdDate int64, err error) {
	return p.creator, p.createdDate, p.err
}

func (p *creationInfoProvider) Id() string                                { return "" }
func (p *creationInfoProvider) Type() model.SmartBlockType                { return 0 }
func (p *creationInfoProvider) Heads() []string                           { return nil }
func (p *creationInfoProvider) GetFileKeysSnapshot() []*pb.ChangeFileKeys { return nil }
func (p *creationInfoProvider) ReadOnly() bool                            { return false }
func (p *creationInfoProvider) Close() (err error)                        { return nil }
func (p *creationInfoProvider) ReadDoc(_ context.Context, _ source.ChangeReceiver, _ bool) (doc state.Doc, err error) {
	return nil, nil
}
func (p *creationInfoProvider) PushChange(_ source.PushChangeParams) (id string, err error) {
	return "", nil
}
