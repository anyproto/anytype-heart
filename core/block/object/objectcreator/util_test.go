package objectcreator

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestEnsureUniqueApiObjectKey(t *testing.T) {
	t.Run("when key is empty should return nil", func(t *testing.T) {
		fx := newFixture(t)

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		assert.Equal(t, "", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("when key is unique should keep original key", func(t *testing.T) {
		fx := newFixture(t)

		// Mock no existing objects with this key
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "unique_key")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		assert.Equal(t, "unique_key", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("when key exists should add sequential suffix", func(t *testing.T) {
		fx := newFixture(t)

		// Add existing object with same key
		existingObject := objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("existing-id"),
			bundle.RelationKeyApiObjectKey: domain.String("task"),
			bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
		}
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{existingObject})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "task")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		assert.Equal(t, "task1", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("when multiple keys exist should find next available suffix", func(t *testing.T) {
		fx := newFixture(t)

		// Add existing objects with sequential keys
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:           domain.String("id1"),
				bundle.RelationKeyApiObjectKey: domain.String("task"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id2"),
				bundle.RelationKeyApiObjectKey: domain.String("task1"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id3"),
				bundle.RelationKeyApiObjectKey: domain.String("task2"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
		})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "task")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		assert.Equal(t, "task3", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("should check uniqueness only within same object type", func(t *testing.T) {
		fx := newFixture(t)

		// Add existing relation with same key
		existingRelation := objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("relation-id"),
			bundle.RelationKeyApiObjectKey: domain.String("task"),
			bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_relation)),
		}
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{existingRelation})

		// Creating object type with same key should work
		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "task")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		assert.Equal(t, "task", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("should fail after max iterations", func(t *testing.T) {
		fx := newFixture(t)

		// Add many existing objects to force hitting the limit
		var existingObjects []objectstore.TestObject
		for i := 0; i <= 1000; i++ {
			key := "task"
			if i > 0 {
				key = fmt.Sprintf("task%d", i)
			}
			existingObjects = append(existingObjects, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String(fmt.Sprintf("id-%d", i)),
				bundle.RelationKeyApiObjectKey: domain.String(key),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			})
		}
		fx.objectStore.AddObjects(t, spaceId, existingObjects)

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "task")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to find unique apiObjectKey after 1000 attempts")
	})
}

func TestInjectAndEnsureUniqueApiObjectKey(t *testing.T) {
	t.Run("when apiObjectKey already set should not change it", func(t *testing.T) {
		fx := newFixture(t)
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "existing_key")
		object.SetString(bundle.RelationKeyName, "My Name")

		err := fx.service.(*service).injectAndEnsureUniqueApiObjectKey(spaceId, object, "provided_key", coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)

		assert.Equal(t, "existing_key", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("when apiObjectKey empty and key provided should use provided key", func(t *testing.T) {
		fx := newFixture(t)
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "My Name")

		err := fx.service.(*service).injectAndEnsureUniqueApiObjectKey(spaceId, object, "provided_key", coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)

		assert.Equal(t, "provided_key", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("when apiObjectKey empty and no key provided should transliterate name", func(t *testing.T) {
		fx := newFixture(t)
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "My Task Name")

		err := fx.service.(*service).injectAndEnsureUniqueApiObjectKey(spaceId, object, "", coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)

		assert.Equal(t, "my_task_name", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("should handle unicode characters", func(t *testing.T) {
		fx := newFixture(t)
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyName, "Привет мир")

		err := fx.service.(*service).injectAndEnsureUniqueApiObjectKey(spaceId, object, "", coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)

		assert.Equal(t, "privet_mir", object.GetString(bundle.RelationKeyApiObjectKey))
	})
}

func TestEnsureUniqueApiObjectKey_BatchQueryBehavior(t *testing.T) {
	t.Run("batch query should only load keys with potential conflicts", func(t *testing.T) {
		fx := newFixture(t)

		// Add various objects with different key patterns
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			// These should be loaded (same prefix)
			{
				bundle.RelationKeyId:           domain.String("id1"),
				bundle.RelationKeyApiObjectKey: domain.String("task"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id2"),
				bundle.RelationKeyApiObjectKey: domain.String("task1"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id3"),
				bundle.RelationKeyApiObjectKey: domain.String("task10"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			// These should be loaded but not conflict (different prefix)
			{
				bundle.RelationKeyId:           domain.String("id4"),
				bundle.RelationKeyApiObjectKey: domain.String("different_key"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id5"),
				bundle.RelationKeyApiObjectKey: domain.String("task_with_suffix"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			// Different object types - should not be loaded
			{
				bundle.RelationKeyId:           domain.String("id6"),
				bundle.RelationKeyApiObjectKey: domain.String("task"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_relation)),
			},
		})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "task")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		// Should find task2 since task, task1, and task10 exist
		assert.Equal(t, "task2", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("should handle non-sequential suffixes correctly", func(t *testing.T) {
		fx := newFixture(t)

		// Add objects with gaps in sequence
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:           domain.String("id1"),
				bundle.RelationKeyApiObjectKey: domain.String("task"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id2"),
				bundle.RelationKeyApiObjectKey: domain.String("task1"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id3"),
				bundle.RelationKeyApiObjectKey: domain.String("task5"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id4"),
				bundle.RelationKeyApiObjectKey: domain.String("task10"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
		})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "task")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		// Should find task2 (the first available sequential suffix)
		assert.Equal(t, "task2", object.GetString(bundle.RelationKeyApiObjectKey))
	})

	t.Run("should filter out keys that don't match the baseKey pattern", func(t *testing.T) {
		fx := newFixture(t)

		// Add objects with various key patterns
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			// This will conflict
			{
				bundle.RelationKeyId:           domain.String("id1"),
				bundle.RelationKeyApiObjectKey: domain.String("mytask"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			// These won't conflict with "mytask" base key
			{
				bundle.RelationKeyId:           domain.String("id2"),
				bundle.RelationKeyApiObjectKey: domain.String("my"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id3"),
				bundle.RelationKeyApiObjectKey: domain.String("myt"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id4"),
				bundle.RelationKeyApiObjectKey: domain.String("mytasks"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:           domain.String("id5"),
				bundle.RelationKeyApiObjectKey: domain.String("mytask_other"),
				bundle.RelationKeyLayout:       domain.Int64(int64(model.ObjectType_objectType)),
			},
		})

		object := domain.NewDetails()
		object.SetString(bundle.RelationKeyApiObjectKey, "mytask")

		err := fx.service.(*service).ensureUniqueApiObjectKey(spaceId, object, coresb.SmartBlockTypeObjectType)
		require.NoError(t, err)
		// Should find mytask1 since only "mytask" exists
		assert.Equal(t, "mytask1", object.GetString(bundle.RelationKeyApiObjectKey))
	})
}
