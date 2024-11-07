package pbtypes

import (
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonToProto(t *testing.T) {
	arena := &anyenc.Arena{}

	t.Run("empty", func(t *testing.T) {
		val := anyenc.MustParseJson(`{}`)

		got, err := AnyEncToProto(val)
		require.NoError(t, err)

		want := &types.Struct{
			Fields: map[string]*types.Value{},
		}
		assert.Equal(t, want, got)

		gotJson := ProtoToAnyEnc(arena, got)
		diff, err := DiffAnyEnc(val, gotJson)
		require.NoError(t, err)
		assert.Empty(t, diff)
	})

	t.Run("all types", func(t *testing.T) {
		val := anyenc.MustParseJson(`
			{
				"key1": "value1",
				"key2": 123,
				"key3": 123.456,
				"key4": true,
				"key5": false,
				"key6": null,
				"key7": [1,2,3],
				"key8":["foo","bar"],
				"key9": {"nestedKey1": "value1", "nestedKey2": 123}
		}`)

		got, err := AnyEncToProto(val)
		require.NoError(t, err)

		want := &types.Struct{
			Fields: map[string]*types.Value{
				"key1": String("value1"),
				"key2": Int64(123),
				"key3": Float64(123.456),
				"key4": Bool(true),
				"key5": Bool(false),
				"key6": Null(),
				"key7": IntList(1, 2, 3),
				"key8": StringList([]string{"foo", "bar"}),
				"key9": Null(),
			},
		}
		assert.Equal(t, want, got)

		gotJson := ProtoToAnyEnc(arena, got)
		diff, err := DiffAnyEnc(val, gotJson)
		require.NoError(t, err)

		// We don't yet support converting nested objects from JSON to proto
		assert.Equal(t, []AnyEncDiff{
			{
				Type:  AnyEncDiffTypeUpdate,
				Key:   "key9",
				Value: arena.NewNull(),
			},
		}, diff)
	})

}

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
