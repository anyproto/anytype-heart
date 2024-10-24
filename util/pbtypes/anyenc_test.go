package pbtypes

import (
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffJson(t *testing.T) {
	arena := &anyenc.Arena{}
	t.Run("empty objects -- no changes", func(t *testing.T) {
		diff, err := DiffAnyEnc(arena.NewObject(), arena.NewObject())
		require.NoError(t, err)
		assert.Empty(t, diff)
	})
	t.Run("equal objects", func(t *testing.T) {
		fillObject := func(obj *anyenc.Value) {
			obj.Set("key1", arena.NewString("value1"))
			obj.Set("key2", arena.NewNumberFloat64(42))
			obj.Set("key3", arena.NewTrue())
			obj.Set("key4", arena.NewFalse())
			obj.Set("key5", arena.NewNull())
			arrA := arena.NewArray()
			arrA.SetArrayItem(0, arena.NewString("value1"))
			arrA.SetArrayItem(1, arena.NewNumberFloat64(666))
			obj.Set("key6", arrA)

			objA := arena.NewObject()
			objA.Set("nestedKey1", arena.NewString("value1"))
			objA.Set("nestedKey2", arena.NewNumberFloat64(123))
			obj.Set("key7", objA)
		}
		a := arena.NewObject()
		fillObject(a)
		b := arena.NewObject()
		fillObject(b)

		diff, err := DiffAnyEnc(a, b)
		require.NoError(t, err)
		assert.Empty(t, diff)
	})
	t.Run("all values are differ", func(t *testing.T) {
		a := arena.NewObject()
		a.Set("key1", arena.NewString("value1"))
		a.Set("key2", arena.NewNumberFloat64(42))
		a.Set("key3", arena.NewTrue())
		a.Set("key4", arena.NewFalse())
		a.Set("key5", arena.NewNull())
		arrA := arena.NewArray()
		arrA.SetArrayItem(0, arena.NewString("value1"))
		arrA.SetArrayItem(1, arena.NewNumberFloat64(666))
		a.Set("key6", arrA)

		objA := arena.NewObject()
		objA.Set("nestedKey1", arena.NewString("value1"))
		objA.Set("nestedKey2", arena.NewNumberFloat64(123))
		a.Set("key7", objA)

		b := arena.NewObject()
		b.Set("key1", arena.NewString("value2"))
		b.Set("key2", arena.NewNumberFloat64(43))
		b.Set("key3", arena.NewFalse())
		b.Set("key4", arena.NewTrue())
		b.Set("key5", arena.NewFalse())
		arrB := arena.NewArray()
		arrB.SetArrayItem(0, arena.NewString("value1"))
		arrB.SetArrayItem(1, arena.NewNumberFloat64(777))
		b.Set("key6", arrB)

		objB := arena.NewObject()
		objB.Set("nestedKey1", arena.NewString("value2"))
		objB.Set("nestedKey2", arena.NewNumberFloat64(123))
		b.Set("key7", objB)

		diff, err := DiffAnyEnc(a, b)
		require.NoError(t, err)

		want := []AnyEncDiff{
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key1",
				Value: arena.NewString("value2"),
			},
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key2",
				Value: arena.NewNumberFloat64(43),
			},
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key3",
				Value: arena.NewFalse(),
			},
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key4",
				Value: arena.NewTrue(),
			},
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key5",
				Value: arena.NewFalse(),
			},
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key6",
				Value: arrB,
			},
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key7",
				Value: objB,
			},
		}
		for _, d := range want {
			// Hack to fix string type
			d.Value.Type()
		}
		assert.Equal(t, want, diff)
	})

	t.Run("added", func(t *testing.T) {
		a := arena.NewObject()
		b := arena.NewObject()
		b.Set("key1", arena.NewString("value1"))

		diff, err := DiffAnyEnc(a, b)
		require.NoError(t, err)
		assert.Equal(t, []AnyEncDiff{
			{
				Type:  AnyEncDiffTypeAdd,
				Key:   "key1",
				Value: arena.NewString("value1"),
			},
		}, diff)
	})
	t.Run("deleted", func(t *testing.T) {
		a := arena.NewObject()
		a.Set("key1", arena.NewString("value1"))
		b := arena.NewObject()

		diff, err := DiffAnyEnc(a, b)
		require.NoError(t, err)
		assert.Equal(t, []AnyEncDiff{
			{
				Type: AnyEncDiffTypeRemove,
				Key:  "key1",
			},
		}, diff)
	})
}
