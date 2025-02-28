package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	spaceId = "spc"
)

type fixture struct {
	sb    *smarttest.SmartTest
	store *objectstore.StoreFixture
	space *smartblock.MockSpace
	*ObjectType
}

func newFixture(t *testing.T, id string) *fixture {
	sb := smarttest.New(id)
	store := objectstore.NewStoreFixture(t)
	spc := smartblock.NewMockSpace(t)
	sb.SetSpace(spc)
	page := &ObjectType{
		SmartBlock:  sb,
		objectStore: store.SpaceIndex(spaceId),
	}

	return &fixture{
		sb:         sb,
		store:      store,
		space:      spc,
		ObjectType: page,
	}
}

func TestObjectType_syncLayoutForObjectsAndTemplates(t *testing.T) {
	typeId := bundle.TypeKeyTask.URL()
	t.Run("recommendedLayout is updated", func(t *testing.T) {
		// given
		fx := newFixture(t, typeId)
		fx.sb.SetType(coresb.SmartBlockTypeObjectType)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("obj1"),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_basic)),
				// layout detail should be deleted from obj1, because its value equals old recommendedLayout value
			},
			{
				bundle.RelationKeyId:             domain.String("obj2"),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_todo)),
				// layout detail should be deleted from obj2, because its value equals new recommendedLayout value
			},
			{
				bundle.RelationKeyId:             domain.String("obj3"),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_profile)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_profile)),
				// obj3 should not be modified, because old layout does not correspond to old and new recommendedLayout values
			},
			{
				bundle.RelationKeyId:             domain.String("obj4"),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
				// obj4 does not have layout detail set, so it has nothing to delete
				// StateAppend must be called for this object because resolvedLayout must be reinjected
			},
			{
				bundle.RelationKeyId:             domain.String("obj5"),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
				// obj5 does not have layout detail set, so it has nothing to delete
				// StateAppend must be called for this object because resolvedLayout must be reinjected
			},
			{
				bundle.RelationKeyId:             domain.String("obj6"),
				bundle.RelationKeyType:           domain.String(typeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
				// obj6 does not have layout detail set, so it has nothing to delete
				// obj6 will not be modified, because it already has correct resolvedLauout value
			},
			{
				bundle.RelationKeyId:               domain.String("tmpl"),
				bundle.RelationKeyType:             domain.String(bundle.TypeKeyTemplate.URL()),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLayout:           domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyTargetObjectType: domain.String(typeId),
				// layout detail should be deleted from template, because its value equals old recommendedLayout value
			},
		})

		obj1 := smarttest.New("obj1")
		obj2 := smarttest.New("obj2")
		obj4 := smarttest.New("obj4")
		tmpl := smarttest.New("tmpl")

		fx.space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).RunAndReturn(func(id string, f func() error) error {
			assert.Equal(t, "obj5", id)
			return f()
		})

		fx.space.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(func(id string, f func(smartblock.SmartBlock) error) error {
			switch id {
			case "obj1":
				assert.NoError(t, f(obj1))
			case "obj2":
				assert.NoError(t, f(obj2))
			case "obj4":
				assert.NoError(t, f(obj4))
			case "tmpl":
				assert.NoError(t, f(tmpl))
			default:
				return ocache.ErrExists
			}
			return nil
		})

		// when
		err := fx.syncLayoutForObjectsAndTemplates(makeApplyInfo(typeId,
			// recommendedLayout is changed: basic -> todo
			layoutState{isLayoutSet: true, layout: int64(model.ObjectType_basic)},
			layoutState{isLayoutSet: true, layout: int64(model.ObjectType_todo)},
		))

		// then
		assert.NoError(t, err)
	})

	// t.Run("change resolvedLayout, do not delete layout", func(t *testing.T) {
	// 	// given
	// 	fx := newFixture(t, typeId)
	// 	fx.sb.SetType(coresb.SmartBlockTypeObjectType)

	// 	fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
	// 		{
	// 			bundle.RelationKeyId:     domain.String("obj1"),
	// 			bundle.RelationKeyType:   domain.String(typeId),
	// 			bundle.RelationKeyLayout: domain.Int64(int64(model.ObjectType_basic)),
	// 		},
	// 		{
	// 			bundle.RelationKeyId:             domain.String("obj2"),
	// 			bundle.RelationKeyType:           domain.String(typeId),
	// 			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
	// 			bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_todo)),
	// 		},
	// 		{
	// 			bundle.RelationKeyId:     domain.String("obj3"),
	// 			bundle.RelationKeyType:   domain.String(typeId),
	// 			bundle.RelationKeyLayout: domain.Int64(int64(model.ObjectType_profile)),
	// 		},
	// 		{
	// 			bundle.RelationKeyId:             domain.String("obj4"),
	// 			bundle.RelationKeyType:           domain.String(typeId),
	// 			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_note)),
	// 		},
	// 	})

	// 	fx.space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).RunAndReturn(func(id string, f func() error) error {
	// 		assert.Equal(t, "obj4", id)
	// 		return f()
	// 	})

	// 	counter := 0
	// 	fx.space.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(func(id string, f func(smartblock.SmartBlock) error) error {
	// 		counter++
	// 		return nil
	// 	})

	// 	// when
	// 	err := fx.syncLayoutForObjectsAndTemplates(makeApplyInfo(int64(model.ObjectType_todo)))

	// 	// then
	// 	assert.NoError(t, err)
	// 	assert.Equal(t, 3, counter)
	// })
}

func makeApplyInfo(typeId string, oldLS, newLS layoutState) smartblock.ApplyInfo {
	events := make([]simple.EventMessage, 0, 3)
	if newLS.isLayoutSet {
		events = append(events, makeObjectDetailsAmendMsg(domain.Detail{
			Key:   bundle.RelationKeyRecommendedLayout,
			Value: domain.Int64(newLS.layout),
		}))
	}

	if newLS.isLayoutAlignSet {
		events = append(events, makeObjectDetailsAmendMsg(domain.Detail{
			Key:   bundle.RelationKeyLayoutAlign,
			Value: domain.Int64(newLS.layoutAlign),
		}))
	}

	if newLS.isFeaturedRelationsSet {
		events = append(events, makeObjectDetailsAmendMsg(domain.Detail{
			Key:   bundle.RelationKeyRecommendedFeaturedRelations,
			Value: domain.StringList(newLS.featuredRelations),
		}))
	}

	ps := state.NewDoc(typeId, nil).NewState().SetDetails(domain.NewDetails())
	if oldLS.isLayoutSet {
		ps.SetDetail(bundle.RelationKeyRecommendedLayout, domain.Int64(oldLS.layout))
	}
	if oldLS.isLayoutAlignSet {
		ps.SetDetail(bundle.RelationKeyLayoutAlign, domain.Int64(oldLS.layoutAlign))
	}
	if oldLS.isFeaturedRelationsSet {
		ps.SetDetail(bundle.RelationKeyRecommendedFeaturedRelations, domain.StringList(oldLS.featuredRelations))
	}

	return smartblock.ApplyInfo{
		Events:      events,
		ParentState: ps,
	}
}

func makeObjectDetailsAmendMsg(detail domain.Detail) simple.EventMessage {
	return simple.EventMessage{
		Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfObjectDetailsAmend{ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
			Details: []*pb.EventObjectDetailsAmendKeyValue{{
				Key:   detail.Key.String(),
				Value: detail.Value.ToProto(),
			}},
		}}},
	}
}
