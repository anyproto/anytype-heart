package domain

import (
	"reflect"
	"testing"
)

func TestStructDiff(t *testing.T) {
	type args struct {
		st1 *Details
		st2 *Details
	}
	tests := []struct {
		name string
		args args
		want *Details
	}{
		{"both nil",
			args{nil, nil},
			nil,
		},
		{"equal",
			args{
				NewDetailsFromMap(map[RelationKey]Value{
					"k1": String("v1"),
				}),
				NewDetailsFromMap(map[RelationKey]Value{
					"k1": String("v1"),
				}),
			},
			nil,
		},
		{"nil st1", args{
			nil,
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": String("v1"),
		}),
		},
		{"nil map st1", args{
			NewDetails(),
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": String("v1"),
		}),
		},
		{"empty map st1", args{
			NewDetailsFromMap(map[RelationKey]Value{}),
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": String("v1"),
		}),
		},
		{"nil st2", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
			nil,
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": Null(),
		}),
		},
		{"nil map st2", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
			NewDetails(),
		},
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": Null(),
			})},
		{"empty map st2", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
			}),
			NewDetailsFromMap(map[RelationKey]Value{})}, NewDetailsFromMap(map[RelationKey]Value{
			"k1": Null(),
		}),
		},
		{"complex", args{
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
				"k2": String("v2"),
				"k3": String("v3"),
			}),
			NewDetailsFromMap(map[RelationKey]Value{
				"k1": String("v1"),
				"k3": String("v3_"),
			}),
		}, NewDetailsFromMap(map[RelationKey]Value{
			"k2": Null(),
			"k3": String("v3_"),
		}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StructDiff(tt.args.st1, tt.args.st2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StructDiff() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TODO FIX
// func TestJsonToProto(t *testing.T) {
// 	arena := &fastjson.Arena{}
//
// 	t.Run("empty", func(t *testing.T) {
// 		val := fastjson.MustParse(`{}`)
//
// 		got, err := NewDetailsFromAnyEnc(val)
// 		require.NoError(t, err)
//
// 		want := &types.Struct{
// 			Fields: map[string]*types.Value{},
// 		}
// 		assert.Equal(t, want, got)
//
// 		gotJson := ProtoToJson(arena, got)
// 		diff, err := DiffJson(val, gotJson)
// 		require.NoError(t, err)
// 		assert.Empty(t, diff)
// 	})
//
// 	t.Run("all types", func(t *testing.T) {
// 		val := fastjson.MustParse(`
// 			{
// 				"key1": "value1",
// 				"key2": 123,
// 				"key3": 123.456,
// 				"key4": true,
// 				"key5": false,
// 				"key6": null,
// 				"key7": [1,2,3],
// 				"key8":["foo","bar"],
// 				"key9": {"nestedKey1": "value1", "nestedKey2": 123}
// 		}`)
//
// 		got, err := NewDetailsFromAnyEnc(val)
// 		require.NoError(t, err)
//
// 		want := &types.Struct{
// 			Fields: map[string]*types.Value{
// 				"key1": String("value1"),
// 				"key2": Int64(123),
// 				"key3": Float64(123.456),
// 				"key4": Bool(true),
// 				"key5": Bool(false),
// 				"key6": Null(),
// 				"key7": IntList(1, 2, 3),
// 				"key8": StringList([]string{"foo", "bar"}),
// 				"key9": Null(),
// 			},
// 		}
// 		assert.Equal(t, want, got)
//
// 		gotJson := ProtoToJson(arena, got)
// 		diff, err := DiffJson(val, gotJson)
// 		require.NoError(t, err)
//
// 		// We don't yet support converting nested objects from JSON to proto
// 		assert.Equal(t, []JsonDiff{
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key9",
// 				Value: arena.NewNull(),
// 			},
// 		}, diff)
// 	})
//
// }
//
// func TestDiffJson(t *testing.T) {
// 	arena := &fastjson.Arena{}
// 	t.Run("empty objects -- no changes", func(t *testing.T) {
// 		diff, err := DiffJson(arena.NewObject(), arena.NewObject())
// 		require.NoError(t, err)
// 		assert.Empty(t, diff)
// 	})
// 	t.Run("equal objects", func(t *testing.T) {
// 		fillObject := func(obj *fastjson.Value) {
// 			obj.Set("key1", arena.NewString("value1"))
// 			obj.Set("key2", arena.NewNumberFloat64(42))
// 			obj.Set("key3", arena.NewTrue())
// 			obj.Set("key4", arena.NewFalse())
// 			obj.Set("key5", arena.NewNull())
// 			arrA := arena.NewArray()
// 			arrA.SetArrayItem(0, arena.NewString("value1"))
// 			arrA.SetArrayItem(1, arena.NewNumberFloat64(666))
// 			obj.Set("key6", arrA)
//
// 			objA := arena.NewObject()
// 			objA.Set("nestedKey1", arena.NewString("value1"))
// 			objA.Set("nestedKey2", arena.NewNumberFloat64(123))
// 			obj.Set("key7", objA)
// 		}
// 		a := arena.NewObject()
// 		fillObject(a)
// 		b := arena.NewObject()
// 		fillObject(b)
//
// 		diff, err := DiffJson(a, b)
// 		require.NoError(t, err)
// 		assert.Empty(t, diff)
// 	})
// 	t.Run("all values are differ", func(t *testing.T) {
// 		a := arena.NewObject()
// 		a.Set("key1", arena.NewString("value1"))
// 		a.Set("key2", arena.NewNumberFloat64(42))
// 		a.Set("key3", arena.NewTrue())
// 		a.Set("key4", arena.NewFalse())
// 		a.Set("key5", arena.NewNull())
// 		arrA := arena.NewArray()
// 		arrA.SetArrayItem(0, arena.NewString("value1"))
// 		arrA.SetArrayItem(1, arena.NewNumberFloat64(666))
// 		a.Set("key6", arrA)
//
// 		objA := arena.NewObject()
// 		objA.Set("nestedKey1", arena.NewString("value1"))
// 		objA.Set("nestedKey2", arena.NewNumberFloat64(123))
// 		a.Set("key7", objA)
//
// 		b := arena.NewObject()
// 		b.Set("key1", arena.NewString("value2"))
// 		b.Set("key2", arena.NewNumberFloat64(43))
// 		b.Set("key3", arena.NewFalse())
// 		b.Set("key4", arena.NewTrue())
// 		b.Set("key5", arena.NewFalse())
// 		arrB := arena.NewArray()
// 		arrB.SetArrayItem(0, arena.NewString("value1"))
// 		arrB.SetArrayItem(1, arena.NewNumberFloat64(777))
// 		b.Set("key6", arrB)
//
// 		objB := arena.NewObject()
// 		objB.Set("nestedKey1", arena.NewString("value2"))
// 		objB.Set("nestedKey2", arena.NewNumberFloat64(123))
// 		b.Set("key7", objB)
//
// 		diff, err := DiffJson(a, b)
// 		require.NoError(t, err)
//
// 		want := []JsonDiff{
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key1",
// 				Value: arena.NewString("value2"),
// 			},
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key2",
// 				Value: arena.NewNumberFloat64(43),
// 			},
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key3",
// 				Value: arena.NewFalse(),
// 			},
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key4",
// 				Value: arena.NewTrue(),
// 			},
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key5",
// 				Value: arena.NewFalse(),
// 			},
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key6",
// 				Value: arrB,
// 			},
// 			{
// 				Type:  JsonDiffTypeUpdate,
// 				Key:   "key7",
// 				Value: objB,
// 			},
// 		}
// 		for _, d := range want {
// 			// Hack to fix string type
// 			d.Value.Type()
// 		}
// 		assert.Equal(t, want, diff)
// 	})
//
// 	t.Run("added", func(t *testing.T) {
// 		a := arena.NewObject()
// 		b := arena.NewObject()
// 		b.Set("key1", arena.NewString("value1"))
//
// 		diff, err := DiffJson(a, b)
// 		require.NoError(t, err)
// 		assert.Equal(t, []JsonDiff{
// 			{
// 				Type:  JsonDiffTypeAdd,
// 				Key:   "key1",
// 				Value: arena.NewString("value1"),
// 			},
// 		}, diff)
// 	})
// 	t.Run("deleted", func(t *testing.T) {
// 		a := arena.NewObject()
// 		a.Set("key1", arena.NewString("value1"))
// 		b := arena.NewObject()
//
// 		diff, err := DiffJson(a, b)
// 		require.NoError(t, err)
// 		assert.Equal(t, []JsonDiff{
// 			{
// 				Type: JsonDiffTypeRemove,
// 				Key:  "key1",
// 			},
// 		}, diff)
// 	})
// }
