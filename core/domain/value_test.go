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
	a := &anyenc.Arena{}
	list := a.NewArray()
	list.Set("0", a.NewString("apple"))
	list.Set("1", a.NewString("banana"))
	assert.Equal(t, list, StringList([]string{"apple", "banana"}).ToAnyEnc(a))
	assert.Equal(t, a.NewNumberInt(42), Int64(42).ToAnyEnc(a))
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
