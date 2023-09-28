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
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/system_object/mock_system_object"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
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
	fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
		Details: &types.Struct{Fields: map[string]*types.Value{}},
	}, nil)
	fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
	fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()

	// when
	fx.init(t, []*model.Block{{Id: id}})

	// then
	assert.Equal(t, id, fx.RootId())
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.at.EXPECT().ProfileID("space1").Return("profile1")
		fx.at.EXPECT().PredefinedObjects("space1").Return(threads.DerivedSmartblockIds{})
		defer fx.tearDown()
		fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil)
		fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
		fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
		fx.restrictionService.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{})

		fx.init(t, []*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))
		fx.source.EXPECT().ReadOnly().Return(false)
		var event *pb.Event
		ctx := session.NewContext()
		fx.RegisterSession(ctx)
		fx.eventSender.EXPECT().SendToSession(mock.Anything, mock.Anything).Run(func(token string, e *pb.Event) {
			event = e
		})
		fx.source.EXPECT().Heads()
		fx.source.EXPECT().PushChange(gomock.Any()).Return("fake_change_id", nil)
		fx.indexer.EXPECT().Index(gomock.Any(), gomock.Any())

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
		fx := newFixture(t)
		defer fx.tearDown()
		fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil)
		fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
		fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
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
		fx := newFixture(t)
		defer fx.tearDown()
		fx.store.EXPECT().GetDetails(gomock.Any()).AnyTimes().Return(&model.ObjectDetails{
			Details: &types.Struct{Fields: map[string]*types.Value{}},
		}, nil)
		fx.store.EXPECT().GetInboundLinksByID(gomock.Any()).AnyTimes()
		fx.store.EXPECT().UpdatePendingLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
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
		detailsFromStore, err := fx.getDetailsFromStore()

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
		details, err := fx.getDetailsFromStore()

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
		detailsFromStore, err := fx.getDetailsFromStore()

		// then
		assert.True(t, errors.Is(err, someErr))
		assert.Nil(t, detailsFromStore)
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
		fx.updateBackLinks(details)

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
		fx.updateBackLinks(details)

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
		fx.updateBackLinks(details)

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
		fx.updateBackLinks(details)

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
		fx.store.EXPECT().UpdatePendingLocalDetails(id, gomock.Any()).
			Do(func(id string, f func(*types.Struct) (*types.Struct, error)) { hasPendingDetails = false })

		// when
		_, result := fx.appendPendingDetails(details)

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
		got, _ := fx.appendPendingDetails(details)

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
		err := fx.injectCreationInfo(s)

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

func TestSmartBlock_removeInternalFlags(t *testing.T) {
	t.Run("")
}

type fixture struct {
	ctrl               *gomock.Controller
	source             *mockSource.MockSource
	at                 *mock_core.MockService
	store              *testMock.MockObjectStore
	restrictionService *mock_restriction.MockService
	indexer            *MockIndexer
	eventSender        *mock_event.MockSender

	*smartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)

	coreService := mock_core.NewMockService(t)

	source := mockSource.NewMockSource(ctrl)
	source.EXPECT().Type().AnyTimes().Return(smartblock.SmartBlockTypePage)
	source.EXPECT().SpaceID().Return("space1").AnyTimes()

	objectStore := testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().Name().Return(objectstore.CName).AnyTimes()

	indexer := NewMockIndexer(ctrl)
	indexer.EXPECT().Name().Return("indexer").AnyTimes()

	restrictionService := mock_restriction.NewMockService(t)
	restrictionService.EXPECT().GetRestrictions(mock.Anything).Return(restriction.Restrictions{}).Maybe()

	systemObjectService := mock_system_object.NewMockService(t)
	systemObjectService.EXPECT().GetObjectType(mock.Anything).Return(&model.ObjectType{}, nil).Maybe()

	fileService := testMock.NewMockFileService(ctrl)

	sender := mock_event.NewMockSender(t)

	sb := New(coreService, fileService, restrictionService, objectStore, systemObjectService, indexer, sender).(*smartBlock)
	sb.source = source
	return &fixture{
		smartBlock:         sb,
		at:                 coreService,
		ctrl:               ctrl,
		store:              objectStore,
		source:             source,
		restrictionService: restrictionService,
		indexer:            indexer,
		eventSender:        sender,
	}
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}

func (fx *fixture) init(t *testing.T, blocks []*model.Block) {
	id := blocks[0].Id
	bm := make(map[string]simple.Block)
	for _, b := range blocks {
		bm[b.Id] = simple.New(b)
	}
	doc := state.NewDoc(id, bm)
	fx.source.EXPECT().ReadDoc(gomock.Any(), gomock.Any(), false).Return(doc, nil)
	fx.source.EXPECT().Id().Return(id).AnyTimes()

	err := fx.Init(&InitContext{
		Ctx:     context.Background(),
		SpaceID: "space1",
		Source:  fx.source,
	})
	require.NoError(t, err)
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
func (p *creationInfoProvider) SpaceID() string                           { return "" }
func (p *creationInfoProvider) Type() smartblock.SmartBlockType           { return 0 }
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
