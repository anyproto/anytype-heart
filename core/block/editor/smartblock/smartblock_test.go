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

var id = "root"

func TestSmartBlock_Init(t *testing.T) {
	//given
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sb, source, coreSvc, _, restrictionSvc, store, _, _ := newSmartBlockAndMocks(t, ctrl)
	coreSvc.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
	restrictionSvc.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
	store.EXPECT().GetDetails(gomock.Any()).AnyTimes()
	//when
	err := initBlocks(sb, source, []*model.Block{{Id: "one"}})

	//then
	require.NoError(t, err)
	assert.Equal(t, "one", sb.RootId())
}

func TestSmartBlock_Apply(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("no flags", func(t *testing.T) {
		//given
		sb, source, at, _, restrictionSvc, store, _, indexer := newSmartBlockAndMocks(t, ctrl)
		source.EXPECT().ReadOnly()
		source.EXPECT().Heads()
		source.EXPECT().PushChange(gomock.Any()).Return("fake_change_id", nil)
		at.EXPECT().PredefinedBlocks()
		at.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
		restrictionSvc.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		store.EXPECT().GetDetails(gomock.Any()).AnyTimes()
		indexer.EXPECT().Index(gomock.Any(), gomock.Any())

		var event *pb.Event
		sb.sendEvent = func(e *pb.Event) { event = e }
		require.NoError(t, initBlocks(sb, source, []*model.Block{{Id: "1"}}))
		s := sb.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))

		//when
		err := sb.Apply(s)

		//then
		require.NoError(t, err)
		assert.Equal(t, 1, sb.History().Len())
		assert.NotNil(t, event)
	})

}

func TestBasic_SetAlign(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	blocks := []*model.Block{
		{Id: "test", ChildrenIds: []string{"title", "2"}},
		{Id: "title"},
		{Id: "2"},
	}

	t.Run("with ids", func(t *testing.T) {
		//given
		sb, source, coreSvc, _, restrictionSvc, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		coreSvc.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
		restrictionSvc.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		store.EXPECT().GetDetails(gomock.Any()).AnyTimes()
		require.NoError(t, initBlocks(sb, source, blocks))
		st := sb.NewState()

		//when
		err := st.SetAlign(model.Block_AlignRight, "2", "3")

		//then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.NewState().Get("2").Model().Align)
	})

	t.Run("without ids", func(t *testing.T) {
		//given
		sb, source, coreSvc, _, restrictionSvc, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		coreSvc.EXPECT().GetWorkspaceIdForObject(gomock.Any()).AnyTimes()
		restrictionSvc.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})
		store.EXPECT().GetDetails(gomock.Any()).AnyTimes()
		require.NoError(t, initBlocks(sb, source, blocks))
		st := sb.NewState()

		//when
		err := st.SetAlign(model.Block_AlignRight)

		//then
		require.NoError(t, err)
		assert.Equal(t, model.Block_AlignRight, st.Get("title").Model().Align)
		assert.Equal(t, int64(model.Block_AlignRight), pbtypes.GetInt64(st.Details(), bundle.RelationKeyLayoutAlign.String()))
	})

}

func TestSmartBlock_getDetailsFromStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("details are in the store", func(t *testing.T) {
		//given
		sb, source, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		details := &types.Struct{
			Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"number": pbtypes.Float64(2.18281828459045),
				"🔥":      pbtypes.StringList([]string{"Jeanne d'Arc", "Giordano Bruno", "Capocchio"}),
			},
		}
		source.EXPECT().Id().Return(id)
		store.EXPECT().GetDetails(id).Return(&model.ObjectDetails{Details: details}, nil)

		//when
		detailsFromStore, err := sb.getDetailsFromStore()

		//then
		assert.NoError(t, err)
		assert.Equal(t, details, detailsFromStore)
	})

	t.Run("no details in the store", func(t *testing.T) {
		//given
		sb, source, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		source.EXPECT().Id().Return(id)
		store.EXPECT().GetDetails(id).Return(nil, nil)

		//when
		details, err := sb.getDetailsFromStore()

		//then
		assert.NoError(t, err)
		assert.Nil(t, details)
	})

	t.Run("failure on retrieving details from store", func(t *testing.T) {
		//given
		sb, source, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		details := &model.ObjectDetails{Details: &types.Struct{
			Fields: map[string]*types.Value{
				"someKey": pbtypes.String("someValue"),
			},
		}}
		someErr := errors.New("some error")
		source.EXPECT().Id().Return(id)
		store.EXPECT().GetDetails(id).Return(details, someErr)

		//when
		detailsFromStore, err := sb.getDetailsFromStore()

		//then
		assert.True(t, errors.Is(err, someErr))
		assert.Nil(t, detailsFromStore)
	})
}

func TestSmartBlock_injectWorkspaceID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	wID := "space"

	t.Run("workspaceID is already set", func(t *testing.T) {
		//given
		sb, src, coreSvc, _, _, _, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Times(0)
		coreSvc.EXPECT().GetWorkspaceIdForObject(gomock.Any()).Times(0)
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyWorkspaceId.String(): pbtypes.String(wID),
		}})

		//when
		sb.injectWorkspaceID(s)

		//then
		assert.Equal(t, wID, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyWorkspaceId.String()))
	})

	t.Run("set workspaceID from core service", func(t *testing.T) {
		//given
		sb, src, coreSvc, _, _, _, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id)
		coreSvc.EXPECT().GetWorkspaceIdForObject(id).Return(wID, nil)
		s := &state.State{}

		//when
		sb.injectWorkspaceID(s)

		//then
		assert.Equal(t, wID, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyWorkspaceId.String()))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyWorkspaceId.String()))
	})

	t.Run("object is deleted, so it does not belong to workspace", func(t *testing.T) {
		//given
		sb, src, coreSvc, _, _, _, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id).Times(1)
		coreSvc.EXPECT().GetWorkspaceIdForObject(id).Return("", core.ErrObjectDoesNotBelongToWorkspace)
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true),
		}})

		//when
		sb.injectWorkspaceID(s)

		//then
		spaceID, found := s.LocalDetails().Fields[bundle.RelationKeyWorkspaceId.String()]
		assert.Nil(t, spaceID)
		assert.False(t, found)
		assert.Nil(t, s.GetRelationLinks())
	})

	t.Run("object is deleted, but core returned other error", func(t *testing.T) {
		//given
		sb, src, coreSvc, _, _, _, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id).Times(2)
		coreSvc.EXPECT().GetWorkspaceIdForObject(id).Return("", errors.New("some error from core"))
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyIsDeleted.String(): pbtypes.Bool(true),
		}})

		//when
		sb.injectWorkspaceID(s)

		//then
		spaceID, found := s.LocalDetails().Fields[bundle.RelationKeyWorkspaceId.String()]
		assert.Nil(t, spaceID)
		assert.False(t, found)
		assert.Nil(t, s.GetRelationLinks())
	})

	t.Run("failure on retrieving workspaceID from core service", func(t *testing.T) {
		//given
		sb, src, coreSvc, _, _, _, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id).Times(1)
		coreSvc.EXPECT().GetWorkspaceIdForObject(id).Return("", errors.New("some error from core"))
		s := &state.State{}

		//when
		sb.injectWorkspaceID(s)

		//then
		assert.Nil(t, s.LocalDetails())
		assert.Nil(t, s.GetRelationLinks())
	})
}

func TestSmartBlock_injectBackLinks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	backLinks := []string{"1", "2", "3"}

	t.Run("back links are already set", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Times(0)
		store.EXPECT().GetInboundLinksByID(gomock.Any()).Times(0)
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyBacklinks.String(): pbtypes.StringList(backLinks),
		}}

		//when
		sb.injectBackLinks(details)

		//then
		assert.Equal(t, backLinks, pbtypes.GetStringList(details, bundle.RelationKeyBacklinks.String()))
	})

	t.Run("back links were found in object store", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id)
		store.EXPECT().GetInboundLinksByID(id).Return(backLinks, nil)
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		//when
		sb.injectBackLinks(details)

		//then
		assert.Equal(t, backLinks, pbtypes.GetStringList(details, bundle.RelationKeyBacklinks.String()))
	})

	t.Run("back links were not found in object store", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id)
		store.EXPECT().GetInboundLinksByID(id).Return(nil, nil)
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		//when
		sb.injectBackLinks(details)

		//then
		assert.Zero(t, len(details.Fields))
	})

	t.Run("failure on retrieving back links from the store", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id).Times(2)
		store.EXPECT().GetInboundLinksByID(id).Return(nil, errors.New("some error from store"))
		details := &types.Struct{Fields: make(map[string]*types.Value)}

		//when
		sb.injectBackLinks(details)

		//then
		assert.Zero(t, len(details.Fields))
	})
}

func TestSmartBlock_updatePendingDetails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("no pending details", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id)
		var hasPendingDetails bool
		details := &types.Struct{Fields: map[string]*types.Value{}}
		store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).Return(nil).
			Do(func(id string, f func(*types.Struct) (*types.Struct, error)) { hasPendingDetails = false })

		//when
		result := sb.updatePendingDetails(details)

		//then
		assert.Equal(t, hasPendingDetails, result)
		assert.Zero(t, len(details.Fields))
	})

	t.Run("found pending details", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id)
		details := &types.Struct{Fields: map[string]*types.Value{}}
		store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).Return(nil).Do(func(id string, f func(*types.Struct) (*types.Struct, error)) {
			details.Fields[bundle.RelationKeyIsDeleted.String()] = pbtypes.Bool(false)
		})

		//when
		_ = sb.updatePendingDetails(details)

		//then
		assert.Len(t, details.Fields, 1)
	})

	t.Run("failure on retrieving pending details from the store", func(t *testing.T) {
		//given
		sb, src, _, _, _, store, _, _ := newSmartBlockAndMocks(t, ctrl)
		src.EXPECT().Id().Return(id).Times(2)
		store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).Return(errors.New("some error from store"))
		details := &types.Struct{}

		//when
		hasPendingDetails := sb.updatePendingDetails(details)

		//then
		assert.False(t, hasPendingDetails)
	})
}

func TestSmartBlock_injectCreationInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	creator := "Anytype"
	creationDate := int64(1692127254)

	t.Run("both creator and creation date are already set", func(t *testing.T) {
		//given
		sb := &smartBlock{}
		s := &state.State{}
		s.SetLocalDetails(&types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyCreator.String():     pbtypes.String(creator),
			bundle.RelationKeyCreatedDate.String(): pbtypes.Int64(creationDate),
		}})

		//when
		err := sb.injectCreationInfo(s)

		//then
		assert.NoError(t, err)
		assert.Equal(t, creator, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyCreator.String()))
		assert.Equal(t, creationDate, pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyCreatedDate.String()))
	})

	t.Run("source could not be converted to CreationInfoProvider", func(t *testing.T) {
		//given
		sb, _, _, _, _, _, _, _ := newSmartBlockAndMocks(t, ctrl)
		s := &state.State{}

		//when
		err := sb.injectCreationInfo(s)

		//then
		assert.NoError(t, err)
		assert.Nil(t, s.LocalDetails())
	})

	t.Run("both creator and creation date are found", func(t *testing.T) {
		//given
		src := &creationInfoProvider{
			creator:     creator,
			createdDate: creationDate,
			err:         nil,
		}
		sb := smartBlock{source: src}
		s := &state.State{}

		//when
		err := sb.injectCreationInfo(s)

		//then
		assert.NoError(t, err)
		assert.Equal(t, creator, pbtypes.GetString(s.LocalDetails(), bundle.RelationKeyCreator.String()))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyCreator.String()))
		assert.Equal(t, creationDate, pbtypes.GetInt64(s.LocalDetails(), bundle.RelationKeyCreatedDate.String()))
		assert.NotNil(t, s.GetRelationLinks().Get(bundle.RelationKeyCreatedDate.String()))
	})

	t.Run("failure on retrieving creation info from source", func(t *testing.T) {
		//given
		srcErr := errors.New("source error")
		src := &creationInfoProvider{err: srcErr}
		sb := smartBlock{source: src}
		s := &state.State{}

		//when
		err := sb.injectCreationInfo(s)

		//then
		assert.True(t, errors.Is(err, srcErr))
		assert.Nil(t, s.LocalDetails())
	})
}

func newSmartBlockAndMocks(t *testing.T, ctrl *gomock.Controller) (
	sb *smartBlock,
	source *mockSource.MockSource,
	coreService *testMock.MockService,
	fileService *testMock.MockFileService,
	restrictionService *mock_restriction.MockService,
	objectStore *testMock.MockObjectStore,
	relationService *mockRelation.MockService,
	indexer *MockIndexer,
) {
	source = mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(model.SmartBlockType_Page)

	coreService = testMock.NewMockService(ctrl)
	coreService.EXPECT().ProfileID().Return("").AnyTimes()

	fileService = testMock.NewMockFileService(ctrl)

	restrictionService = mock_restriction.NewMockService(t)

	objectStore = testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().GetObjectType(gomock.Any()).AnyTimes()
	objectStore.EXPECT().Name().Return(objectstore.CName).AnyTimes()

	relationService = mockRelation.NewMockService(ctrl)

	indexer = NewMockIndexer(ctrl)
	indexer.EXPECT().Name().Return("indexer").AnyTimes()

	sb = &smartBlock{
		source:             source,
		coreService:        coreService,
		fileService:        fileService,
		restrictionService: restrictionService,
		objectStore:        objectStore,
		relationService:    relationService,
		indexer:            indexer,
	}

	return
}

func initBlocks(sb *smartBlock, source *mockSource.MockSource, blocks []*model.Block) error {
	id := blocks[0].Id
	bm := make(map[string]simple.Block)
	for _, b := range blocks {
		bm[b.Id] = simple.New(b)
	}
	doc := state.NewDoc(id, bm)
	source.EXPECT().ReadDoc(context.Background(), gomock.Any(), false).Return(doc, nil)
	source.EXPECT().Id().Return(id).AnyTimes()

	return sb.Init(&InitContext{Source: source})
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
