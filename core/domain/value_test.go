package domain

import (
	"fmt"
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompareMaps(t *testing.T) {
	for i, tc := range []struct {
		a, b map[string]Value
		want int
	}{
		{
			a:    map[string]Value{"a": Int64(1), "b": String("b")},
			b:    map[string]Value{"a": Int64(1), "b": String("b")},
			want: 0,
		},
		{
			a:    map[string]Value{"a": Int64(1)},
			b:    map[string]Value{"b": String("b")},
			want: -1,
		},
		{
			a:    map[string]Value{"b": String("b")},
			b:    map[string]Value{"a": Int64(1)},
			want: 1,
		},
		{
			a:    map[string]Value{"a": Int64(1), "b": String("b")},
			b:    map[string]Value{"a": Int64(2), "b": String("b")},
			want: -1,
		},
		{
			a:    map[string]Value{"a": Int64(1), "b": String("c")},
			b:    map[string]Value{"a": Int64(1), "b": String("b")},
			want: 1,
		},
		{
			a:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			b:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			want: 0,
		},
		{
			a:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			b:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(2)})},
			want: -1,
		},
		{
			a:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(2)})},
			b:    map[string]Value{"a": NewValueMap(map[string]Value{"a": Int64(1)})},
			want: 1,
		},
	} {
		t.Run(fmt.Sprintf("case %d", i+1), func(t *testing.T) {
			got := compareMaps(&GenericMap[string]{data: (tc.a)}, &GenericMap[string]{data: (tc.b)})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestValue_Match(t *testing.T) {
	matchAll := func(matches *[]string) ValueMatcher {
		return ValueMatcher{
			Null: func() {
				*matches = append(*matches, "null")
			},
			Bool: func(v bool) {
				*matches = append(*matches, "bool")
			},
			Float64: func(v float64) {
				*matches = append(*matches, "float64")
			},
			Int64: func(v int64) {
				*matches = append(*matches, "int64")
			},
			String: func(v string) {
				*matches = append(*matches, "string")
			},
			StringList: func(v []string) {
				*matches = append(*matches, "[]string")
			},
			Float64List: func(v []float64) {
				*matches = append(*matches, "[]float64")
			},
			Int64List: func(v []int64) {
				*matches = append(*matches, "[]int64")
			},
			MapValue: func(valueMap ValueMap) {
				*matches = append(*matches, "map")
			},
			MapList: func(v []ValueMap) {
				*matches = append(*matches, "[]map")
			},
		}
	}

	for _, tc := range []struct {
		value Value
		want  []string
	}{
		{
			value: Invalid(),
			want:  []string{},
		},
		{
			value: Null(),
			want:  []string{"null"},
		},
		{
			value: Bool(false),
			want:  []string{"bool"},
		},
		{
			value: Int64(123),
			want:  []string{"int64", "float64"},
		},
		{
			value: Float64(123.345),
			want:  []string{"int64", "float64"},
		},
		{
			value: String("foo"),
			want:  []string{"string"},
		},
		{
			value: StringList([]string{"foo", "bar"}),
			want:  []string{"[]string"},
		},
		{
			value: Float64List([]float64{123.345}),
			want:  []string{"[]float64", "[]int64"},
		},
		{
			value: Int64List([]int64{123}),
			want:  []string{"[]float64", "[]int64"},
		},
		{
			value: NewValueMap(nil),
			want:  []string{"map"},
		},
		{
			value: MapList([]ValueMap{
				&GenericMap[string]{data: map[string]Value{"a": Int64(1)}},
			}),
			want: []string{"[]map"},
		},
	} {
		var matches []string
		m := matchAll(&matches)
		tc.value.Match(m)
		assert.ElementsMatch(t, tc.want, matches)
	}
}

func TestValue_Empty(t *testing.T) {
	for _, tc := range []struct {
		value Value
		want  bool
	}{
		{
			value: Invalid(),
			want:  true,
		},
		{
			value: Null(),
			want:  true,
		},
		{
			value: Bool(false),
			want:  true,
		},
		{
			value: Bool(true),
			want:  false,
		},
		{
			value: Int64(0),
			want:  true,
		},
		{
			value: Int64(-1),
			want:  false,
		},
		{
			value: Float64(0),
			want:  true,
		},
		{
			value: Float64(0.0001),
			want:  false,
		},
		{
			value: Int64List[int64](nil),
			want:  true,
		},
		{
			value: Int64List[int64]([]int64{}),
			want:  true,
		},
		{
			value: Int64List([]int64{1}),
			want:  false,
		},
		{
			value: Float64List(nil),
			want:  true,
		},
		{
			value: Float64List([]float64{}),
			want:  true,
		},
		{
			value: Float64List([]float64{1}),
			want:  false,
		},
		{
			value: String(""),
			want:  true,
		},
		{
			value: String("1"),
			want:  false,
		},
		{
			value: StringList(nil),
			want:  true,
		},
		{
			value: StringList([]string{}),
			want:  true,
		},
		{
			value: StringList([]string{"a"}),
			want:  false,
		},
		{
			value: NewValueMap(nil),
			want:  true,
		},
		{
			value: NewValueMap(map[string]Value{}),
			want:  true,
		},
		{
			value: NewValueMap(map[string]Value{
				"key": String("value"),
			}),
			want: false,
		},
		{
			value: MapList(nil),
			want:  true,
		},
		{
			value: MapList([]ValueMap{}),
			want:  true,
		},
		{
			value: MapList([]ValueMap{
				&GenericMap[string]{data: map[string]Value{"a": Int64(1)}},
			}),
			want: false,
		},
	} {
		assert.Equal(t, tc.want, tc.value.IsEmpty())
	}
}

func TestTryWrapToStringList(t *testing.T) {
	for i, tc := range []struct {
		in     Value
		want   []string
		wantOk bool
	}{
		{
			in:     String(""),
			want:   []string{},
			wantOk: true,
		},
		{
			in:     String("foo"),
			want:   []string{"foo"},
			wantOk: true,
		},
		{
			in:     StringList([]string{"foo", "bar"}),
			want:   []string{"foo", "bar"},
			wantOk: true,
		},
		{
			in:     Float64(123.456),
			want:   nil,
			wantOk: false,
		},
		{
			in:     Invalid(),
			want:   nil,
			wantOk: false,
		},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, gotOk := tc.in.TryWrapToStringList()
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOk, gotOk)
		})
	}
}

func TestValue_ToAnyEnc(t *testing.T) {
	t.Run("null", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Null().ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeNull, result.Type())
		assert.Nil(t, result.GoType())
	})

	t.Run("invalid treated as null", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Invalid().ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeNull, result.Type())
	})

	t.Run("bool true", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Bool(true).ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeTrue, result.Type())
		assert.Equal(t, true, result.GoType())
	})

	t.Run("bool false", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Bool(false).ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeFalse, result.Type())
		assert.Equal(t, false, result.GoType())
	})

	t.Run("string", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := String("hello").ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeString, result.Type())
		assert.Equal(t, "hello", result.GoType())
	})

	t.Run("float64", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Float64(42.5).ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeNumber, result.Type())
		assert.Equal(t, 42.5, result.GoType())
	})

	t.Run("int64", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Int64(42).ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeNumber, result.Type())
		assert.Equal(t, float64(42), result.GoType())
	})

	t.Run("string list", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := StringList([]string{"a", "b", "c"}).ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeArray, result.Type())
		want := []any{"a", "b", "c"}
		assert.Equal(t, want, result.GoType())
	})

	t.Run("float64 list", func(t *testing.T) {
		arena := &anyenc.Arena{}
		result := Float64List([]float64{1.1, 2.2}).ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeArray, result.Type())
		want := []any{1.1, 2.2}
		assert.Equal(t, want, result.GoType())
	})

	t.Run("single map", func(t *testing.T) {
		arena := &anyenc.Arena{}
		v := NewValueMap(map[string]Value{
			"name":  String("test"),
			"count": Float64(42),
		})
		result := v.ToAnyEnc(arena)
		assert.Equal(t, anyenc.TypeObject, result.Type())

		got := result.GoType().(map[string]any)
		assert.Equal(t, "test", got["name"])
		assert.Equal(t, float64(42), got["count"])
	})

	t.Run("list of maps", func(t *testing.T) {
		arena := &anyenc.Arena{}
		m1 := &GenericMap[string]{data: map[string]Value{
			"type": Float64(1),
			"name": String("first"),
		}}
		m2 := &GenericMap[string]{data: map[string]Value{
			"type": Float64(2),
			"name": String("second"),
		}}
		v := MapList([]ValueMap{m1, m2})
		result := v.ToAnyEnc(arena)

		assert.Equal(t, anyenc.TypeArray, result.Type())
		arr := result.GetArray()
		require.Len(t, arr, 2)

		assert.Equal(t, anyenc.TypeObject, arr[0].Type())
		assert.Equal(t, float64(1), arr[0].GetFloat64("type"))
		assert.Equal(t, "first", arr[0].GetString("name"))

		assert.Equal(t, anyenc.TypeObject, arr[1].Type())
		assert.Equal(t, float64(2), arr[1].GetFloat64("type"))
		assert.Equal(t, "second", arr[1].GetString("name"))
	})

	t.Run("list of maps roundtrip", func(t *testing.T) {
		arena := &anyenc.Arena{}
		m1 := &GenericMap[string]{data: map[string]Value{
			"type": Float64(1),
			"name": String("first"),
		}}
		m2 := &GenericMap[string]{data: map[string]Value{
			"type": Float64(2),
			"name": String("second"),
		}}
		v := MapList([]ValueMap{m1, m2})
		result := v.ToAnyEnc(arena)

		data := result.MarshalTo(nil)
		parser := &anyenc.Parser{}
		parsed, err := parser.Parse(data)
		require.NoError(t, err)

		assert.Equal(t, anyenc.TypeArray, parsed.Type())
		arr := parsed.GetArray()
		require.Len(t, arr, 2)

		assert.Equal(t, float64(1), arr[0].GetFloat64("type"))
		assert.Equal(t, "first", arr[0].GetString("name"))
		assert.Equal(t, float64(2), arr[1].GetFloat64("type"))
		assert.Equal(t, "second", arr[1].GetString("name"))
	})

	t.Run("large list of maps triggers arena growth", func(t *testing.T) {
		arena := &anyenc.Arena{}
		maps := make([]ValueMap, 20)
		for i := range maps {
			maps[i] = &GenericMap[string]{data: map[string]Value{
				"index": Float64(float64(i)),
				"name":  String(fmt.Sprintf("item_%d", i)),
				"flag":  Bool(i%2 == 0),
			}}
		}
		v := MapList(maps)
		result := v.ToAnyEnc(arena)

		goVal := result.GoType()
		arr, ok := goVal.([]any)
		require.True(t, ok)
		require.Len(t, arr, 20)

		for i, item := range arr {
			m, ok := item.(map[string]any)
			require.True(t, ok, "item %d should be map, got %T", i, item)
			assert.Equal(t, float64(i), m["index"], "item %d index", i)
			assert.Equal(t, fmt.Sprintf("item_%d", i), m["name"], "item %d name", i)
			assert.Equal(t, i%2 == 0, m["flag"], "item %d flag", i)
		}

		// Also verify via roundtrip
		data := result.MarshalTo(nil)
		parser := &anyenc.Parser{}
		parsed, err := parser.Parse(data)
		require.NoError(t, err)

		parsedArr := parsed.GetArray()
		require.Len(t, parsedArr, 20)
		for i, item := range parsedArr {
			assert.Equal(t, anyenc.TypeObject, item.Type(), "item %d type", i)
			assert.Equal(t, float64(i), item.GetFloat64("index"), "item %d index (roundtrip)", i)
			assert.Equal(t, fmt.Sprintf("item_%d", i), item.GetString("name"), "item %d name (roundtrip)", i)
			assert.Equal(t, i%2 == 0, item.GetBool("flag"), "item %d flag (roundtrip)", i)
		}
	})

	t.Run("nested map", func(t *testing.T) {
		arena := &anyenc.Arena{}
		inner := NewValueMap(map[string]Value{
			"x": Float64(10),
			"y": Float64(20),
		})
		v := NewValueMap(map[string]Value{
			"label":    String("point"),
			"position": inner,
		})
		result := v.ToAnyEnc(arena)

		assert.Equal(t, anyenc.TypeObject, result.Type())
		assert.Equal(t, "point", result.GetString("label"))

		pos := result.GetObject("position")
		require.NotNil(t, pos)
		assert.Equal(t, float64(10), pos.Get("x").GetFloat64())
		assert.Equal(t, float64(20), pos.Get("y").GetFloat64())
	})

	t.Run("map with mixed value types", func(t *testing.T) {
		arena := &anyenc.Arena{}
		v := NewValueMap(map[string]Value{
			"str":     String("hello"),
			"num":     Float64(3.14),
			"flag":    Bool(true),
			"tags":    StringList([]string{"a", "b"}),
			"scores":  Float64List([]float64{1, 2, 3}),
			"nothing": Null(),
		})
		result := v.ToAnyEnc(arena)

		got := result.GoType().(map[string]any)
		assert.Equal(t, "hello", got["str"])
		assert.Equal(t, 3.14, got["num"])
		assert.Equal(t, true, got["flag"])
		assert.Equal(t, []any{"a", "b"}, got["tags"])
		assert.Equal(t, []any{float64(1), float64(2), float64(3)}, got["scores"])
		assert.Nil(t, got["nothing"])
	})

	t.Run("multiple conversions on same arena", func(t *testing.T) {
		arena := &anyenc.Arena{}

		v1 := MapList([]ValueMap{
			&GenericMap[string]{data: map[string]Value{"a": Float64(1)}},
			&GenericMap[string]{data: map[string]Value{"a": Float64(2)}},
		})
		r1 := v1.ToAnyEnc(arena)

		v2 := MapList([]ValueMap{
			&GenericMap[string]{data: map[string]Value{"b": String("x")}},
			&GenericMap[string]{data: map[string]Value{"b": String("y")}},
		})
		r2 := v2.ToAnyEnc(arena)

		// Verify r1 is still correct after r2 was built
		arr1 := r1.GetArray()
		require.Len(t, arr1, 2)
		assert.Equal(t, float64(1), arr1[0].GetFloat64("a"))
		assert.Equal(t, float64(2), arr1[1].GetFloat64("a"))

		arr2 := r2.GetArray()
		require.Len(t, arr2, 2)
		assert.Equal(t, "x", arr2[0].GetString("b"))
		assert.Equal(t, "y", arr2[1].GetString("b"))
	})

	t.Run("list of maps with nested map values", func(t *testing.T) {
		arena := &anyenc.Arena{}
		m1 := &GenericMap[string]{data: map[string]Value{
			"id":   Float64(1),
			"meta": NewValueMap(map[string]Value{"color": String("red")}),
		}}
		m2 := &GenericMap[string]{data: map[string]Value{
			"id":   Float64(2),
			"meta": NewValueMap(map[string]Value{"color": String("blue")}),
		}}
		v := MapList([]ValueMap{m1, m2})
		result := v.ToAnyEnc(arena)

		arr := result.GetArray()
		require.Len(t, arr, 2)

		assert.Equal(t, float64(1), arr[0].GetFloat64("id"))
		assert.Equal(t, "red", arr[0].GetString("meta", "color"))

		assert.Equal(t, float64(2), arr[1].GetFloat64("id"))
		assert.Equal(t, "blue", arr[1].GetString("meta", "color"))
	})
}

func TestValue_ToAnyEnc_ArenaReuse(t *testing.T) {
	t.Run("values from first cycle survive reset", func(t *testing.T) {
		arena := &anyenc.Arena{}

		// First cycle: create a map list, marshal to bytes
		m1 := &GenericMap[string]{data: map[string]Value{"x": Float64(1)}}
		m2 := &GenericMap[string]{data: map[string]Value{"x": Float64(2)}}
		v := MapList([]ValueMap{m1, m2})
		r1 := v.ToAnyEnc(arena)
		bytes1 := r1.MarshalTo(nil)

		// Reset and reuse for a different structure
		arena.Reset()
		r2 := String("overwrite").ToAnyEnc(arena)
		bytes2 := r2.MarshalTo(nil)

		// Parse bytes1 to verify first cycle's data survived in serialized form
		parser := &anyenc.Parser{}
		parsed1, err := parser.Parse(bytes1)
		require.NoError(t, err)
		arr := parsed1.GetArray()
		require.Len(t, arr, 2)
		assert.Equal(t, float64(1), arr[0].GetFloat64("x"))
		assert.Equal(t, float64(2), arr[1].GetFloat64("x"))

		parsed2, err := parser.Parse(bytes2)
		require.NoError(t, err)
		assert.Equal(t, "overwrite", parsed2.GetString())
	})

	t.Run("serialize before reset pattern", func(t *testing.T) {
		// This mimics the pattern in order.go:getStringVal
		arena := &anyenc.Arena{}

		values := []Value{
			MapList([]ValueMap{
				&GenericMap[string]{data: map[string]Value{"a": Float64(1), "b": String("x")}},
				&GenericMap[string]{data: map[string]Value{"a": Float64(2), "b": String("y")}},
			}),
			Float64(42),
			String("hello"),
			StringList([]string{"one", "two"}),
		}

		serialized := make([]string, len(values))
		for i, v := range values {
			enc := v.ToAnyEnc(arena)
			serialized[i] = string(enc.MarshalTo(nil))
			arena.Reset()
		}

		// Verify each serialized form independently
		parser := &anyenc.Parser{}

		parsed0, err := parser.Parse([]byte(serialized[0]))
		require.NoError(t, err)
		arr := parsed0.GetArray()
		require.Len(t, arr, 2)
		assert.Equal(t, float64(1), arr[0].GetFloat64("a"))
		assert.Equal(t, "x", arr[0].GetString("b"))
		assert.Equal(t, float64(2), arr[1].GetFloat64("a"))
		assert.Equal(t, "y", arr[1].GetString("b"))

		parsed1, err := parser.Parse([]byte(serialized[1]))
		require.NoError(t, err)
		assert.Equal(t, float64(42), parsed1.GetFloat64())

		parsed2, err := parser.Parse([]byte(serialized[2]))
		require.NoError(t, err)
		assert.Equal(t, "hello", parsed2.GetString())

		parsed3, err := parser.Parse([]byte(serialized[3]))
		require.NoError(t, err)
		parsedArr := parsed3.GetArray()
		require.Len(t, parsedArr, 2)
		assert.Equal(t, "one", parsedArr[0].GetString())
		assert.Equal(t, "two", parsedArr[1].GetString())
	})

	t.Run("multiple map lists on same arena without reset", func(t *testing.T) {
		// This mimics filter.go:FilterIn pattern where values are retained
		arena := &anyenc.Arena{}

		var allValues []*anyenc.Value
		for i := 0; i < 10; i++ {
			m := &GenericMap[string]{data: map[string]Value{
				"id":   Float64(float64(i)),
				"name": String(fmt.Sprintf("item_%d", i)),
			}}
			v := NewValueMap(m.data)
			allValues = append(allValues, v.ToAnyEnc(arena))
		}

		// All values should still be valid
		for i, val := range allValues {
			assert.Equal(t, anyenc.TypeObject, val.Type(), "value %d type", i)
			assert.Equal(t, float64(i), val.GetFloat64("id"), "value %d id", i)
			assert.Equal(t, fmt.Sprintf("item_%d", i), val.GetString("name"), "value %d name", i)
		}
	})
}

func TestGenericMap_ToAnyEnc(t *testing.T) {
	t.Run("simple map", func(t *testing.T) {
		arena := &anyenc.Arena{}
		m := &GenericMap[string]{data: map[string]Value{
			"name": String("test"),
			"age":  Float64(25),
		}}
		result := m.ToAnyEnc(arena)

		assert.Equal(t, anyenc.TypeObject, result.Type())
		assert.Equal(t, "test", result.GetString("name"))
		assert.Equal(t, float64(25), result.GetFloat64("age"))
	})

	t.Run("consistent with Value.ToAnyEnc", func(t *testing.T) {
		data := map[string]Value{
			"key1": String("val1"),
			"key2": Float64(99),
		}
		m := &GenericMap[string]{data: data}

		arena1 := &anyenc.Arena{}
		fromGenericMap := m.ToAnyEnc(arena1)

		arena2 := &anyenc.Arena{}
		fromValue := NewValueMap(data).ToAnyEnc(arena2)

		// Both should produce the same serialized form
		bytes1 := fromGenericMap.MarshalTo(nil)
		bytes2 := fromValue.MarshalTo(nil)

		// Use separate parsers because Parse reuses internal buffers
		parser1 := &anyenc.Parser{}
		parsed1, err := parser1.Parse(bytes1)
		require.NoError(t, err)
		parser2 := &anyenc.Parser{}
		parsed2, err := parser2.Parse(bytes2)
		require.NoError(t, err)

		assert.Equal(t, parsed1.GetString("key1"), parsed2.GetString("key1"))
		assert.Equal(t, parsed1.GetFloat64("key2"), parsed2.GetFloat64("key2"))
	})

	t.Run("map with string list values", func(t *testing.T) {
		arena := &anyenc.Arena{}
		m := &GenericMap[string]{data: map[string]Value{
			"tags":   StringList([]string{"go", "rust"}),
			"scores": Float64List([]float64{1.5, 2.5}),
		}}
		result := m.ToAnyEnc(arena)

		got := result.GoType().(map[string]any)
		assert.Equal(t, []any{"go", "rust"}, got["tags"])
		assert.Equal(t, []any{1.5, 2.5}, got["scores"])
	})
}

func TestMapList(t *testing.T) {
	m1 := &GenericMap[string]{data: map[string]Value{"type": Float64(1)}}
	m2 := &GenericMap[string]{data: map[string]Value{"type": Float64(0), "value": String("obj1")}}

	t.Run("constructor and accessors", func(t *testing.T) {
		v := MapList([]ValueMap{m1, m2})

		assert.True(t, v.Ok())
		assert.True(t, v.IsMapList())
		assert.False(t, v.IsMapValue())
		assert.False(t, v.IsStringList())
		assert.Equal(t, ValueType(ValueTypeMapList), v.Type())

		got, ok := v.TryMapList()
		require.True(t, ok)
		require.Len(t, got, 2)
		assert.True(t, got[0].Equal(m1))
		assert.True(t, got[1].Equal(m2))

		assert.Equal(t, got, v.MapListValue())
	})

	t.Run("nil and empty", func(t *testing.T) {
		assert.True(t, MapList(nil).IsEmpty())
		assert.True(t, MapList([]ValueMap{}).IsEmpty())
		assert.False(t, MapList([]ValueMap{m1}).IsEmpty())
	})

	t.Run("invalid value returns nil", func(t *testing.T) {
		v := Invalid()
		assert.False(t, v.IsMapList())
		assert.Nil(t, v.MapListValue())
		_, ok := v.TryMapList()
		assert.False(t, ok)
	})

	t.Run("proto roundtrip", func(t *testing.T) {
		v := MapList([]ValueMap{m1, m2})
		proto := v.ToProto()
		restored := ValueFromProto(proto)

		assert.True(t, restored.IsMapList())
		got, ok := restored.TryMapList()
		require.True(t, ok)
		require.Len(t, got, 2)
		assert.True(t, got[0].Equal(m1))
		assert.True(t, got[1].Equal(m2))
	})

	t.Run("equal", func(t *testing.T) {
		a := MapList([]ValueMap{m1, m2})
		b := MapList([]ValueMap{m1, m2})
		assert.True(t, a.Equal(b))

		c := MapList([]ValueMap{m2, m1})
		assert.False(t, a.Equal(c))

		d := MapList([]ValueMap{m1})
		assert.False(t, a.Equal(d))
	})

	t.Run("compare", func(t *testing.T) {
		a := MapList([]ValueMap{m1})
		b := MapList([]ValueMap{m1, m2})
		assert.Equal(t, -1, a.Compare(b))
		assert.Equal(t, 1, b.Compare(a))
		assert.Equal(t, 0, a.Compare(a))
	})

	t.Run("TryListValues", func(t *testing.T) {
		v := MapList([]ValueMap{m1, m2})
		list, ok := v.TryListValues()
		require.True(t, ok)
		require.Len(t, list, 2)
		assert.True(t, list[0].IsMapValue())
		assert.True(t, list[1].IsMapValue())
	})
}

func TestValueList(t *testing.T) {
	t.Run("strings produce StringList", func(t *testing.T) {
		v := ValueList([]Value{String("a"), String("b")})
		assert.True(t, v.IsStringList())
		assert.Equal(t, []string{"a", "b"}, v.StringList())
	})

	t.Run("floats produce Float64List", func(t *testing.T) {
		v := ValueList([]Value{Float64(1), Float64(2)})
		assert.True(t, v.IsFloat64List())
		assert.Equal(t, []float64{1, 2}, v.Float64List())
	})

	t.Run("maps produce MapList", func(t *testing.T) {
		m1 := NewValueMap(map[string]Value{"k": Int64(1)})
		m2 := NewValueMap(map[string]Value{"k": Int64(2)})
		v := ValueList([]Value{m1, m2})
		assert.True(t, v.IsMapList())
		got := v.MapListValue()
		require.Len(t, got, 2)
		assert.Equal(t, int64(1), got[0].GetInt64("k"))
		assert.Equal(t, int64(2), got[1].GetInt64("k"))
	})

	t.Run("empty produces empty StringList", func(t *testing.T) {
		v := ValueList([]Value{})
		assert.True(t, v.IsStringList())
		assert.Nil(t, v.StringList())
	})
}
