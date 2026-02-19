package domain

import (
	"fmt"
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
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
