package detailservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

func relationObject(key domain.RelationKey, format model.RelationFormat) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(key.URL()),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
		bundle.RelationKeyResolvedLayout: domain.Float64(float64(model.ObjectType_relation)),
		bundle.RelationKeyRelationKey:    domain.String(key.String()),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(format)),
	}
}

func TestService_ListRelationsWithValue(t *testing.T) {
	now := time.Now()
	store := objectstore.NewStoreFixture(t)
	store.AddObjects(t, spaceId, []objectstore.TestObject{
		// relations
		relationObject(bundle.RelationKeyLastModifiedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyAddedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyCreatedDate, model.RelationFormat_date),
		relationObject(bundle.RelationKeyLinks, model.RelationFormat_object),
		relationObject(bundle.RelationKeyMentions, model.RelationFormat_object),
		relationObject(bundle.RelationKeyName, model.RelationFormat_longtext),
		relationObject(bundle.RelationKeyIsHidden, model.RelationFormat_checkbox),
		relationObject(bundle.RelationKeyIsFavorite, model.RelationFormat_checkbox),
		relationObject("daysTillSummer", model.RelationFormat_number),
		relationObject(bundle.RelationKeyCoverX, model.RelationFormat_number),
		{
			bundle.RelationKeyId:               domain.String("obj1"),
			bundle.RelationKeySpaceId:          domain.String(spaceId),
			bundle.RelationKeyCreatedDate:      domain.Int64(now.Add(-5 * time.Minute).Unix()),
			bundle.RelationKeyAddedDate:        domain.Int64(now.Add(-3 * time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: domain.Int64(now.Add(-1 * time.Minute).Unix()),
			bundle.RelationKeyIsFavorite:       domain.Bool(true),
			"daysTillSummer":                   domain.Int64(300),
			bundle.RelationKeyLinks:            domain.StringList([]string{"obj2", "obj3", dateutil.NewDateObject(now.Add(-30*time.Minute), true).Id()}),
		},
		{
			bundle.RelationKeyId:               domain.String("obj2"),
			bundle.RelationKeySpaceId:          domain.String(spaceId),
			bundle.RelationKeyName:             domain.String(dateutil.NewDateObject(now, true).Id()),
			bundle.RelationKeyCreatedDate:      domain.Int64(now.Add(-24*time.Hour - 5*time.Minute).Unix()),
			bundle.RelationKeyAddedDate:        domain.Int64(now.Add(-24*time.Hour - 3*time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: domain.Int64(now.Add(-1 * time.Minute).Unix()),
			bundle.RelationKeyCoverX:           domain.Int64(300),
		},
		{
			bundle.RelationKeyId:               domain.String("obj3"),
			bundle.RelationKeySpaceId:          domain.String(spaceId),
			bundle.RelationKeyIsHidden:         domain.Bool(true),
			bundle.RelationKeyCreatedDate:      domain.Int64(now.Add(-3 * time.Minute).Unix()),
			bundle.RelationKeyLastModifiedDate: domain.Int64(now.Unix()),
			bundle.RelationKeyIsFavorite:       domain.Bool(true),
			bundle.RelationKeyCoverX:           domain.Int64(300),
			bundle.RelationKeyMentions:         domain.StringList([]string{dateutil.NewDateObject(now, true).Id(), dateutil.NewDateObject(now.Add(-24*time.Hour), true).Id()}),
		},
	})

	bs := service{store: store}

	for _, tc := range []struct {
		name         string
		value        domain.Value
		expectedList []*pb.RpcRelationListWithValueResponseResponseItem
	}{
		{
			"date object - today",
			domain.String(dateutil.NewDateObject(now, true).Id()),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyMentions.String(), 1},
				{bundle.RelationKeyAddedDate.String(), 1},
				{bundle.RelationKeyCreatedDate.String(), 2},
				{bundle.RelationKeyLastModifiedDate.String(), 3},
				{bundle.RelationKeyLinks.String(), 1},
				{bundle.RelationKeyName.String(), 1},
			},
		},
		{
			"date object - yesterday",
			domain.String(dateutil.NewDateObject(now.Add(-24*time.Hour), true).Id()),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyMentions.String(), 1},
				{bundle.RelationKeyAddedDate.String(), 1},
				{bundle.RelationKeyCreatedDate.String(), 1},
			},
		},
		{
			"number",
			domain.Int64(300),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyCoverX.String(), 2},
				{"daysTillSummer", 1},
			},
		},
		{
			"bool",
			domain.Bool(true),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyIsFavorite.String(), 2},
				{bundle.RelationKeyIsHidden.String(), 1},
			},
		},
		{
			"string list",
			domain.StringList([]string{"obj2", "obj3", dateutil.NewDateObject(now.Add(-30*time.Minute), true).Id()}),
			[]*pb.RpcRelationListWithValueResponseResponseItem{
				{bundle.RelationKeyLinks.String(), 1},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			list, err := bs.ListRelationsWithValue(spaceId, tc.value)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedList, list)
		})
	}
}

func TestService_objectTypeSetRelations(t *testing.T) {
	t.Run("set recommended relations to type", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(bundle.TypeKeyTask.URL())
		sb.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
				bundle.RelationKeyAssignee.URL(),
				bundle.RelationKeyIsFavorite.URL(),
				bundle.RelationKeyLinkedProjects.URL(),
			}),
		}))
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, bundle.TypeKeyTask.URL(), objectId)
			return sb, nil
		})

		// when
		err := fx.ObjectTypeSetRelations(bundle.TypeKeyTask.URL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()},
			sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
	})

	t.Run("setting recommended relations to bundled type is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.ObjectTypeSetRelations(bundle.TypeKeyTask.BundledURL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrBundledTypeIsReadonly, err)
	})

	t.Run("set recommended featured relations to type", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(bundle.TypeKeyTask.URL())
		sb.Doc.(*state.State).SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{
				bundle.RelationKeyAssignee.URL(),
				bundle.RelationKeyIsFavorite.URL(),
				bundle.RelationKeyLinkedProjects.URL(),
			}),
		}))
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, bundle.TypeKeyTask.URL(), objectId)
			return sb, nil
		})

		// when
		err := fx.ObjectTypeSetFeaturedRelations(bundle.TypeKeyTask.URL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL()},
			sb.Details().GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
	})

	t.Run("setting recommended featured relations to bundled type is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		err := fx.ObjectTypeSetFeaturedRelations(bundle.TypeKeyTask.BundledURL(), []string{
			bundle.RelationKeyAssignee.URL(), bundle.RelationKeyDone.URL(),
		})

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrBundledTypeIsReadonly, err)
	})
}

func TestService_ObjectTypeListConflictingRelations(t *testing.T) {
	t.Run("list conflicting relations", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			// object type
			{
				bundle.RelationKeyId: domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{
					bundle.RelationKeyType.URL(),
					bundle.RelationKeyName.URL(),
				}),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{
					bundle.RelationKeyAssignee.URL(),
					bundle.RelationKeyDone.URL(),
				}),
				bundle.RelationKeyRecommendedHiddenRelations: domain.StringList([]string{
					bundle.RelationKeyCreatedDate.URL(),
				}),
			},
			// objects
			{
				bundle.RelationKeyId:       domain.String("task1"), // 1
				bundle.RelationKeyType:     domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyName:     domain.String("Invent alphabet"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"Kirill", "Methodius"}),
				bundle.RelationKeyDueDate:  domain.Int64(863), // 2
			},
			{
				bundle.RelationKeyId:       domain.String("task2"),
				bundle.RelationKeyType:     domain.String(bundle.TypeKeyTask.URL()),
				bundle.RelationKeyName:     domain.String("Fight CO2 pollution"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"Humanity"}),
				bundle.RelationKeyDone:     domain.Bool(false),
				bundle.RelationKeyStatus:   domain.String("In Progress"), // 3
			},
			// relations
			generateRelationTestObject(bundle.RelationKeyId),
			generateRelationTestObject(bundle.RelationKeyType),
			generateRelationTestObject(bundle.RelationKeyName),
			generateRelationTestObject(bundle.RelationKeyAssignee),
			generateRelationTestObject(bundle.RelationKeyDueDate),
			generateRelationTestObject(bundle.RelationKeyDone),
			generateRelationTestObject(bundle.RelationKeyStatus),
		})

		// when
		relations, err := fx.ObjectTypeListConflictingRelations(spaceId, bundle.TypeKeyTask.URL())

		// then
		assert.NoError(t, err)
		assert.Len(t, relations, 3)
	})
}

func generateRelationTestObject(key domain.RelationKey) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(key.URL()),
		bundle.RelationKeyRelationKey:    domain.String(key.String()),
		bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_relation),
	}
}

const (
	typePropertyViewA = "viewA"
	typePropertyViewB = "viewB"
)

// typeWithDataview builds a type object whose dataview carries the given
// views under the fixed dataview block id, the way a type built by this app
// does.
func typeWithDataview(t *testing.T, fx *fixture, details map[domain.RelationKey]domain.Value, links []*model.RelationLink, views ...*model.BlockContentDataviewView) *smarttest.SmartTest {
	t.Helper()
	sb := smarttest.New(bundle.TypeKeyTask.URL())
	sb.SetSpace(fx.space)
	st := sb.Doc.(*state.State)
	st.Add(simple.New(&model.Block{Id: bundle.TypeKeyTask.URL(), ChildrenIds: []string{state.DataviewBlockID}}))
	st.Add(simple.New(&model.Block{Id: state.DataviewBlockID, Content: &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{RelationLinks: links, Views: views},
	}}))
	if details != nil {
		st.SetDetails(domain.NewDetailsFromMap(details))
	}
	fx.getter.EXPECT().GetObject(mock.Anything, bundle.TypeKeyTask.URL()).Return(sb, nil)
	return sb
}

func typePropertyView(id string, relations ...*model.BlockContentDataviewRelation) *model.BlockContentDataviewView {
	return &model.BlockContentDataviewView{Id: id, Name: id, Relations: relations}
}

func nameColumn() *model.BlockContentDataviewRelation {
	return &model.BlockContentDataviewRelation{Key: bundle.RelationKeyName.String(), IsVisible: true, Width: 200}
}

func typeDataview(sb *smarttest.SmartTest) *model.BlockContentDataview {
	return sb.Doc.Pick(state.DataviewBlockID).Model().GetDataview()
}

func TestService_ObjectTypePropertyAdd(t *testing.T) {
	t.Run("mint a new property when key is empty", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, nil, nil,
			typePropertyView(typePropertyViewA, nameColumn()),
			typePropertyView(typePropertyViewB, nameColumn()),
		)
		fx.space.EXPECT().Id().Return(spaceId)
		minted := domain.NewDetails()
		minted.SetString(bundle.RelationKeyRelationKey, "mintedKey")
		fx.objectCreator.EXPECT().CreateObject(mock.Anything, spaceId, mock.MatchedBy(func(req objectcreator.CreateObjectRequest) bool {
			return req.ObjectTypeKey == bundle.TypeKeyRelation &&
				req.Details.GetString(bundle.RelationKeyName) == "Priority" &&
				req.Details.GetInt64(bundle.RelationKeyRelationFormat) == int64(model.RelationFormat_status)
		})).Return("mintedId", minted, nil)
		want := ObjectTypePropertyAddResult{Key: "mintedKey", PropertyId: "mintedId", ViewIds: []string{typePropertyViewA, typePropertyViewB}}
		wantColumn := &model.BlockContentDataviewRelation{Key: "mintedKey", IsVisible: true, Width: 100}

		// when
		got, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId:  bundle.TypeKeyTask.URL(),
			Name:          "Priority",
			Format:        model.RelationFormat_status,
			EnableInViews: true,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, []string{"mintedId"}, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		dv := typeDataview(sb)
		assert.Equal(t, []*model.RelationLink{{Key: "mintedKey", Format: model.RelationFormat_status}}, dv.RelationLinks)
		for _, view := range dv.Views {
			assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn(), wantColumn}, view.Relations, view.Id)
		}
	})

	t.Run("reference an existing key", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{relationObject("customKey", model.RelationFormat_number)})
		sb := typeWithDataview(t, fx, nil, nil, typePropertyView(typePropertyViewA, nameColumn()))
		fx.space.EXPECT().Id().Return(spaceId)
		want := ObjectTypePropertyAddResult{Key: "customKey", PropertyId: domain.RelationKey("customKey").URL(), ViewIds: []string{typePropertyViewA}}

		// when
		got, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId:  bundle.TypeKeyTask.URL(),
			Key:           "customKey",
			Section:       TypePropertySectionFeatured,
			EnableInViews: true,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, []string{domain.RelationKey("customKey").URL()}, sb.Details().GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
		assert.Empty(t, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		dv := typeDataview(sb)
		assert.Equal(t, []*model.RelationLink{{Key: "customKey", Format: model.RelationFormat_number}}, dv.RelationLinks)
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn(), {Key: "customKey", IsVisible: true, Width: 100}}, dv.Views[0].Relations)
	})

	t.Run("enableInViews false adds the column hidden", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{relationObject("customKey", model.RelationFormat_longtext)})
		sb := typeWithDataview(t, fx, nil, nil, typePropertyView(typePropertyViewA, nameColumn()))
		fx.space.EXPECT().Id().Return(spaceId)
		want := []*model.BlockContentDataviewRelation{nameColumn(), {Key: "customKey", IsVisible: false, Width: 200}}

		// when
		got, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Key:          "customKey",
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{typePropertyViewA}, got.ViewIds)
		assert.Equal(t, want, typeDataview(sb).Views[0].Relations)
	})

	t.Run("move a property between sections", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{relationObject("customKey", model.RelationFormat_longtext)})
		relId := domain.RelationKey("customKey").URL()
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{bundle.RelationKeyAssignee.URL(), relId}),
			bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{bundle.RelationKeyDone.URL()}),
		}, []*model.RelationLink{{Key: "customKey", Format: model.RelationFormat_longtext}},
			typePropertyView(typePropertyViewA, nameColumn(), &model.BlockContentDataviewRelation{Key: "customKey", IsVisible: true, Width: 200}),
		)
		fx.space.EXPECT().Id().Return(spaceId)

		// when
		got, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Key:          "customKey",
			Section:      TypePropertySectionHidden,
		})

		// then
		require.NoError(t, err)
		assert.Empty(t, got.ViewIds, "a column the view already has is not gained again")
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL()}, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Equal(t, []string{bundle.RelationKeyDone.URL()}, sb.Details().GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
		assert.Equal(t, []string{relId}, sb.Details().GetStringList(bundle.RelationKeyRecommendedHiddenRelations))
		assert.True(t, typeDataview(sb).Views[0].Relations[1].IsVisible, "an existing column keeps its owner's visibility")
	})

	t.Run("already listed property heals the missing column and link", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{relationObject("customKey", model.RelationFormat_date)})
		relId := domain.RelationKey("customKey").URL()
		listed := []string{bundle.RelationKeyAssignee.URL(), relId, bundle.RelationKeyDone.URL()}
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList(listed),
		}, nil,
			typePropertyView(typePropertyViewA, nameColumn()),
			typePropertyView(typePropertyViewB, nameColumn(), &model.BlockContentDataviewRelation{Key: "customKey", IsVisible: false, Width: 200}),
		)
		fx.space.EXPECT().Id().Return(spaceId)
		want := ObjectTypePropertyAddResult{Key: "customKey", PropertyId: relId, ViewIds: []string{typePropertyViewA}}

		// when
		got, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId:  bundle.TypeKeyTask.URL(),
			Key:           "customKey",
			EnableInViews: true,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, listed, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations), "the list is untouched, position included")
		dv := typeDataview(sb)
		assert.Equal(t, []*model.RelationLink{{Key: "customKey", Format: model.RelationFormat_date}}, dv.RelationLinks)
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn(), {Key: "customKey", IsVisible: true, Width: 200}}, dv.Views[0].Relations)
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn(), {Key: "customKey", IsVisible: false, Width: 200}}, dv.Views[1].Relations, "the column view B already had is left as it was")
	})

	t.Run("bundled key not installed in the space is installed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, nil, nil, typePropertyView(typePropertyViewA, nameColumn()))
		fx.space.EXPECT().Id().Return(spaceId)
		fx.objectCreator.EXPECT().InstallBundledObjects(mock.Anything, fx.space, []string{bundle.RelationKeyDueDate.BundledURL()}).Return(nil, nil, nil)
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, bundle.RelationKeyDueDate).Return(bundle.RelationKeyDueDate.URL(), nil)
		want := ObjectTypePropertyAddResult{Key: bundle.RelationKeyDueDate, PropertyId: bundle.RelationKeyDueDate.URL(), ViewIds: []string{typePropertyViewA}}

		// when
		got, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Key:          bundle.RelationKeyDueDate,
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, []string{bundle.RelationKeyDueDate.URL()}, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Equal(t, []*model.RelationLink{{Key: bundle.RelationKeyDueDate.String(), Format: model.RelationFormat_date}}, typeDataview(sb).RelationLinks)
	})

	t.Run("unknown key is bad input and mints nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, nil, nil, typePropertyView(typePropertyViewA, nameColumn()))
		fx.space.EXPECT().Id().Return(spaceId)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Key:          "nosuchkey",
		})

		// then
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
		assert.Contains(t, err.Error(), "nosuchkey")
		assert.Empty(t, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Empty(t, typeDataview(sb).RelationLinks)
	})

	t.Run("empty key without a name is bad input and mints nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Format:       model.RelationFormat_number,
		})

		// then
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
	})

	t.Run("unknown format is bad input and mints nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Name:         "Priority",
			Format:       model.RelationFormat(999),
		})

		// then
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
	})

	t.Run("unknown section is bad input", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Key:          bundle.RelationKeyDueDate,
			Section:      TypePropertySection(42),
		})

		// then
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
	})

	t.Run("the file section cannot be asked for", func(t *testing.T) {
		// given — no store or type setup: the refusal lands before either
		fx := newFixture(t)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.URL(),
			Key:          bundle.RelationKeySizeInBytes,
			Section:      TypePropertySectionFile,
		})

		// then
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
	})

	t.Run("a property the type keeps in its file section is not moved out", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{relationObject(bundle.RelationKeySizeInBytes, model.RelationFormat_number)})
		relId := bundle.RelationKeySizeInBytes.URL()
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedFileRelations: domain.StringList([]string{relId}),
		}, nil, typePropertyView(typePropertyViewA, nameColumn()))
		fx.space.EXPECT().Id().Return(spaceId)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId:  bundle.TypeKeyTask.URL(),
			Key:           bundle.RelationKeySizeInBytes,
			Section:       TypePropertySectionFeatured,
			EnableInViews: true,
		})

		// then — refused, and nothing about the type moved
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
		assert.Equal(t, []string{relId}, sb.Details().GetStringList(bundle.RelationKeyRecommendedFileRelations))
		assert.Empty(t, sb.Details().GetStringList(bundle.RelationKeyRecommendedFeaturedRelations))
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn()}, typeDataview(sb).Views[0].Relations)
	})

	t.Run("editing of bundled types is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.ObjectTypePropertyAdd(context.Background(), ObjectTypePropertyAddRequest{
			ObjectTypeId: bundle.TypeKeyTask.BundledURL(),
			Name:         "Priority",
			Format:       model.RelationFormat_status,
		})

		// then
		require.ErrorIs(t, err, ErrBundledTypeIsReadonly)
	})
}

func TestService_ObjectTypePropertyRemove(t *testing.T) {
	column := func(key domain.RelationKey) *model.BlockContentDataviewRelation {
		return &model.BlockContentDataviewRelation{Key: key.String(), IsVisible: true, Width: 200}
	}
	links := func() []*model.RelationLink {
		return []*model.RelationLink{
			{Key: bundle.RelationKeyName.String(), Format: model.RelationFormat_shorttext},
			{Key: "customKey", Format: model.RelationFormat_longtext},
		}
	}
	relId := domain.RelationKey("customKey").URL()

	t.Run("prunes the column from every view and drops the link", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations:       domain.StringList([]string{bundle.RelationKeyAssignee.URL(), relId}),
			bundle.RelationKeyRecommendedHiddenRelations: domain.StringList([]string{relId}),
		}, links(),
			typePropertyView(typePropertyViewA, nameColumn(), column("customKey")),
			typePropertyView(typePropertyViewB, column("customKey"), nameColumn()),
		)
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, domain.RelationKey("customKey")).Return(relId, nil)
		want := ObjectTypePropertyRemoveResult{}

		// when
		got, err := fx.ObjectTypePropertyRemove(context.Background(), bundle.TypeKeyTask.URL(), "customKey")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL()}, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Empty(t, sb.Details().GetStringList(bundle.RelationKeyRecommendedHiddenRelations))
		dv := typeDataview(sb)
		assert.Equal(t, links()[:1], dv.RelationLinks)
		for _, view := range dv.Views {
			assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn()}, view.Relations, view.Id)
		}
	})

	t.Run("leaves a view that groups by the property and reports it", func(t *testing.T) {
		// given
		fx := newFixture(t)
		grouped := typePropertyView(typePropertyViewA, nameColumn(), column("customKey"))
		grouped.GroupRelationKey = "customKey"
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{relId}),
		}, links(),
			grouped,
			typePropertyView(typePropertyViewB, nameColumn(), column("customKey")),
		)
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, domain.RelationKey("customKey")).Return(relId, nil)
		want := ObjectTypePropertyRemoveResult{InUseViewIds: []string{typePropertyViewA}}

		// when
		got, err := fx.ObjectTypePropertyRemove(context.Background(), bundle.TypeKeyTask.URL(), "customKey")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.Empty(t, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		dv := typeDataview(sb)
		assert.Equal(t, links(), dv.RelationLinks, "a link the grouped view still uses stays")
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn(), column("customKey")}, dv.Views[0].Relations)
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn()}, dv.Views[1].Relations)
	})

	t.Run("a key the type does not list still converges the views", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{bundle.RelationKeyAssignee.URL()}),
		}, links(),
			typePropertyView(typePropertyViewA, nameColumn(), column("customKey")),
		)
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, domain.RelationKey("customKey")).Return(relId, nil)

		// when
		got, err := fx.ObjectTypePropertyRemove(context.Background(), bundle.TypeKeyTask.URL(), "customKey")

		// then
		require.NoError(t, err)
		assert.Empty(t, got.InUseViewIds)
		assert.Equal(t, []string{bundle.RelationKeyAssignee.URL()}, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		dv := typeDataview(sb)
		assert.Equal(t, links()[:1], dv.RelationLinks)
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn()}, dv.Views[0].Relations)
	})

	t.Run("a property in the file section is refused", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedFileRelations: domain.StringList([]string{relId}),
		}, links(), typePropertyView(typePropertyViewA, nameColumn(), column("customKey")))
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, domain.RelationKey("customKey")).Return(relId, nil)

		// when
		_, err := fx.ObjectTypePropertyRemove(context.Background(), bundle.TypeKeyTask.URL(), "customKey")

		// then — refused, and neither the list nor the views moved
		require.ErrorIs(t, err, ErrTypePropertyBadInput)
		assert.Equal(t, []string{relId}, sb.Details().GetStringList(bundle.RelationKeyRecommendedFileRelations))
		dv := typeDataview(sb)
		assert.Equal(t, links(), dv.RelationLinks)
		assert.Equal(t, []*model.BlockContentDataviewRelation{nameColumn(), column("customKey")}, dv.Views[0].Relations)
	})

	t.Run("a link no view shows is dropped", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := typeWithDataview(t, fx, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{relId}),
		}, links(),
			typePropertyView(typePropertyViewA, nameColumn()),
		)
		fx.space.EXPECT().GetRelationIdByKey(mock.Anything, domain.RelationKey("customKey")).Return(relId, nil)

		// when
		_, err := fx.ObjectTypePropertyRemove(context.Background(), bundle.TypeKeyTask.URL(), "customKey")

		// then
		require.NoError(t, err)
		assert.Empty(t, sb.Details().GetStringList(bundle.RelationKeyRecommendedRelations))
		assert.Equal(t, links()[:1], typeDataview(sb).RelationLinks)
	})

	t.Run("editing of bundled types is prohibited", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		_, err := fx.ObjectTypePropertyRemove(context.Background(), bundle.TypeKeyTask.BundledURL(), "customKey")

		// then
		require.ErrorIs(t, err, ErrBundledTypeIsReadonly)
	})
}
