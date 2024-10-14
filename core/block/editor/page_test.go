package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_migrateRelationOptions(t *testing.T) {
	t.Run("not relation option", func(t *testing.T) {
		// given
		page := &Page{objectStore: newStoreFixture(t)}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_basic)))

		// when
		page.migrateRelationOptions(s)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), pbtypes.GetInt64(s.Details(), bundle.RelationKeyLayout.String()))
	})
	t.Run("not tag relation option", func(t *testing.T) {
		// given
		page := &Page{objectStore: newStoreFixture(t)}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relationOption)))
		s.SetDetail(bundle.RelationKeyRelationKey.String(), pbtypes.String("key"))

		// when
		page.migrateRelationOptions(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relationOption), pbtypes.GetInt64(s.Details(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, "key", pbtypes.GetString(s.Details(), bundle.RelationKeyRelationKey.String()))
	})
	t.Run("migrate relation option", func(t *testing.T) {
		// given
		storeFixture := newStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyUniqueKey: pbtypes.String(bundle.TypeKeyTag.URL()),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
			},
		})
		smartTest := smarttest.New("id2")
		smartTest.SetSpaceId("spaceId")
		page := &Page{objectStore: storeFixture, SmartBlock: smartTest}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relationOption)))
		s.SetDetail(bundle.RelationKeyRelationKey.String(), pbtypes.String(bundle.RelationKeyTag.String()))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String("key"))

		// when
		page.migrateRelationOptions(s)

		// then
		assert.Equal(t, int64(model.ObjectType_tag), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, "id1", pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyType.String()))
		assert.Empty(t, pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
		assert.Empty(t, pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyRelationKey.String()))
	})
	t.Run("type tage not exist - not migrate", func(t *testing.T) {
		// given
		smartTest := smarttest.New("id2")
		smartTest.SetSpaceId("spaceId")
		page := &Page{objectStore: newStoreFixture(t), SmartBlock: smartTest}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relationOption)))
		s.SetDetail(bundle.RelationKeyRelationKey.String(), pbtypes.String(bundle.RelationKeyTag.String()))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String("key"))

		// when
		page.migrateRelationOptions(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relationOption), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, bundle.RelationKeyTag.String(), pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyRelationKey.String()))
		assert.Equal(t, "key", pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
	})
}

func Test_migrateRelation(t *testing.T) {
	t.Run("not relation", func(t *testing.T) {
		// given
		page := &Page{objectStore: newStoreFixture(t)}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_basic)))

		// when
		page.migrateRelation(s)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), pbtypes.GetInt64(s.Details(), bundle.RelationKeyLayout.String()))
	})
	t.Run("not tag relation", func(t *testing.T) {
		// given
		page := &Page{objectStore: newStoreFixture(t)}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String("key"))

		// when
		page.migrateRelation(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relation), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, "key", pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
	})
	t.Run("not tag format relation", func(t *testing.T) {
		// given
		page := &Page{objectStore: newStoreFixture(t)}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String(bundle.RelationKeyTag.URL()))
		s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_object)))

		// when
		page.migrateRelation(s)

		// then
		assert.Equal(t, int64(model.ObjectType_relation), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyLayout.String()))
		assert.Equal(t, bundle.RelationKeyTag.URL(), pbtypes.GetString(s.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
		assert.Equal(t, int64(model.RelationFormat_object), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String()))
	})
	t.Run("migrate relation", func(t *testing.T) {
		// given
		storeFixture := newStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyUniqueKey: pbtypes.String(bundle.TypeKeyTag.URL()),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
			},
		})
		smartTest := smarttest.New("id2")
		smartTest.SetSpaceId("spaceId")
		page := &Page{objectStore: storeFixture, SmartBlock: smartTest}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String(bundle.RelationKeyTag.URL()))
		s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_tag)))

		// when
		page.migrateRelation(s)

		// then
		assert.Equal(t, int64(model.RelationFormat_object), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String()))
		assert.Equal(t, []string{"id1"}, pbtypes.GetStringList(s.CombinedDetails(), bundle.RelationKeyRelationFormatObjectTypes.String()))
	})
	t.Run("type tage not exist - not migrate", func(t *testing.T) {
		// given
		smartTest := smarttest.New("id2")
		smartTest.SetSpaceId("spaceId")
		page := &Page{objectStore: newStoreFixture(t), SmartBlock: smartTest}
		s := &state.State{}
		s.SetDetail(bundle.RelationKeyLayout.String(), pbtypes.Int64(int64(model.ObjectType_relation)))
		s.SetDetail(bundle.RelationKeyUniqueKey.String(), pbtypes.String(bundle.RelationKeyTag.URL()))
		s.SetDetail(bundle.RelationKeyRelationFormat.String(), pbtypes.Int64(int64(model.RelationFormat_tag)))

		// when
		page.migrateRelation(s)

		// then
		assert.Equal(t, int64(model.RelationFormat_tag), pbtypes.GetInt64(s.CombinedDetails(), bundle.RelationKeyRelationFormat.String()))
	})
}
