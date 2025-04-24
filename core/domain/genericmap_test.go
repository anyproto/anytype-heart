package domain

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func givenValues() []Value {
	return []Value{
		Null(),
		String("hi"),
		Int64(123),
		Float64(123.456),
		Bool(true),
		Bool(false),
		StringList([]string{"1", "2", "3"}),
		Float64List([]float64{1.1, 2.2, 3.3, 4.4}),
		Int64List([]int64{1, 2, 3}),
		NewValueMap(map[string]Value{
			"int64":  Int64(101010),
			"string": String("string"),
		}),
	}
}

func TestGenericMap_Set(t *testing.T) {
	for _, val := range givenValues() {
		m := NewGenericMap[string]()
		key := "only"
		m.Set(key, val)

		assert.True(t, m.Has(key))
		assert.Equal(t, val, m.Get(key))
	}
}

func TestGenericMap_Delete(t *testing.T) {
	m := NewGenericMap[string]()
	key := "only"
	m.Set(key, Int64(123))
	assert.True(t, m.Has(key))
	m.Delete(key)
	assert.False(t, m.Has(key))
}

func TestGenericMap_Iterate(t *testing.T) {
	m := NewGenericMap[string]()
	m.Set("key1", String("value1"))
	m.Set("key2", String("value2"))
	m.Set("key3", String("value3"))

	collected := map[string]Value{}
	for k, v := range m.Iterate() {
		collected[k] = v
	}

	want := map[string]Value{
		"key1": String("value1"),
		"key2": String("value2"),
		"key3": String("value3"),
	}
	assert.Equal(t, want, collected)
}

func TestGenericMap_IterateSorted(t *testing.T) {
	m := NewGenericMap[string]()

	var (
		wantKeys   []string
		wantValues []Value
	)
	for i := 0; i < 100; i++ {
		val := fmt.Sprintf("%d", i)
		key := fmt.Sprintf("key_%02d", i)
		m.Set(key, String(val))
		wantKeys = append(wantKeys, key)
		wantValues = append(wantValues, String(val))
	}

	var (
		gotKeys   []string
		gotValues []Value
	)
	for k, v := range m.IterateSorted() {
		gotKeys = append(gotKeys, k)
		gotValues = append(gotValues, v)
	}

	assert.Equal(t, wantKeys, gotKeys)
	assert.Equal(t, wantValues, gotValues)
}

func TestGenericMap_IterateKeys(t *testing.T) {
	m := NewGenericMap[string]()
	m.Set("key1", String("value1"))
	m.Set("key2", String("value2"))
	m.Set("key3", String("value3"))

	var collected []string
	for k := range m.IterateKeys() {
		collected = append(collected, k)
	}
	slices.Sort(collected)

	want := []string{"key1", "key2", "key3"}
	assert.Equal(t, want, collected)
}

func TestGenericMap_Copy(t *testing.T) {
	m1 := NewGenericMap[string]()
	m1.Set("key1", String("value1"))

	m2 := m1.Copy()
	assertCopyMutations(t, m1, m2)
}

func TestGenericMap_CopyOnlyKeys(t *testing.T) {
	m1 := NewGenericMap[string]()
	m1.Set("key1", String("value1"))
	m1.Set("key3", String("value3"))

	m2 := m1.CopyOnlyKeys("key1")

	assert.NotEqual(t, m1, m2)

	want2 := NewGenericMap[string]()
	want2.Set("key1", String("value1"))

	assert.Equal(t, want2, m2)
	assertCopyMutations(t, m1, m2)
}

func TestGenericMap_CopyWithoutKeys(t *testing.T) {
	m1 := NewGenericMap[string]()
	m1.Set("key1", String("value1"))
	m1.Set("key2", String("value2"))

	m2 := m1.CopyWithoutKeys("key2")

	want2 := NewGenericMap[string]()
	want2.Set("key1", String("value1"))

	assert.Equal(t, want2, m2)
	assertCopyMutations(t, m2, m1)
}

func assertCopyMutations(t *testing.T, m1, m2 *GenericMap[string]) {
	// Mutate first
	m1.Set("key1", String("value1_new_new"))
	// Mutate second
	m2.Set("key1", String("value1_new"))
	m2.Set("key2", String("value2"))

	assert.NotEqual(t, m1, m2)

	assert.Equal(t, m1.Get("key1"), String("value1_new_new"))
	assert.False(t, m1.Has("key2"))

	assert.Equal(t, m2.Get("key1"), String("value1_new"))
	assert.Equal(t, m2.Get("key2"), String("value2"))
}

func TestGenericMap_Merge(t *testing.T) {
	m1 := NewGenericMap[string]()
	m1.Set("key1", String("value1"))
	m1.Set("key2", String("value2"))

	m2 := NewGenericMap[string]()
	m2.Set("key2", String("value2_new"))
	m2.Set("key3", String("value3"))

	want := NewGenericMap[string]()
	want.Set("key1", String("value1"))
	want.Set("key2", String("value2_new"))
	want.Set("key3", String("value3"))

	got := m1.Merge(m2)
	assert.Equal(t, want, got)
}

type maybe[T any] struct {
	val *T
}

func some[T any](v T) *maybe[T] {
	return &maybe[T]{&v}
}
func (m *maybe[T]) value() T {
	return *m.val
}

func TestGenericMap_TryGet(t *testing.T) {
	for _, tc := range []struct {
		value       Value
		null        *maybe[nullValue]
		bool        *maybe[bool]
		int64       *maybe[int64]
		float64     *maybe[float64]
		string      *maybe[string]
		stringList  *maybe[[]string]
		float64List *maybe[[]float64]
		int64List   *maybe[[]int64]
		mapValue    *maybe[ValueMap]
	}{
		{
			value: Null(),
			null:  some(nullValue{}),
		},
		{
			value: Bool(false),
			bool:  some(false),
		},
		{
			value: Bool(true),
			bool:  some(true),
		},
		{
			value:   Int64(1024),
			int64:   some(int64(1024)),
			float64: some(float64(1024)),
		},
		{
			value:   Float64(123),
			int64:   some(int64(123)),
			float64: some(float64(123)),
		},
		{
			value:  String("foo"),
			string: some("foo"),
		},
		{
			value:      StringList([]string{"foo", "bar"}),
			stringList: some([]string{"foo", "bar"}),
		},
		{
			value:       Float64List([]float64{1.1, 2.2, 3.3, 4.4}),
			float64List: some([]float64{1.1, 2.2, 3.3, 4.4}),
			int64List:   some([]int64{1, 2, 3, 4}),
		},
		{
			value:       Int64List([]int64{1, 2, 3}),
			int64List:   some([]int64{1, 2, 3}),
			float64List: some([]float64{1, 2, 3}),
		},
		{
			value: NewValueMap(map[string]Value{
				"key1": String("value1"),
			}),
			mapValue: some(NewGenericMap[string]().Set("key1", String("value1"))),
		},
	} {
		key := "key"
		m := NewGenericMap[string]()
		m.Set(key, tc.value)

		if tc.null != nil {
			ok := m.GetNull(key)
			assert.True(t, ok)
		} else {
			ok := m.GetNull(key)
			assert.False(t, ok)
		}
		if tc.bool != nil {
			v, ok := m.TryBool(key)
			assert.True(t, ok)
			assert.Equal(t, tc.bool.value(), v)
			assert.True(t, m.Get(key).IsBool())
		} else {
			_, ok := m.TryBool(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsBool())
		}
		if tc.int64 != nil {
			v, ok := m.TryInt64(key)
			assert.True(t, ok)
			assert.Equal(t, tc.int64.value(), v)
			assert.True(t, m.Get(key).IsInt64())
		} else {
			_, ok := m.TryInt64(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsInt64())
		}
		if tc.float64 != nil {
			v, ok := m.TryFloat64(key)
			assert.True(t, ok)
			assert.Equal(t, tc.float64.value(), v)
			assert.True(t, m.Get(key).IsFloat64())
		} else {
			_, ok := m.TryFloat64(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsFloat64())
		}
		if tc.string != nil {
			v, ok := m.TryString(key)
			assert.True(t, ok)
			assert.Equal(t, tc.string.value(), v)
			assert.True(t, m.Get(key).IsString())
		} else {
			_, ok := m.TryString(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsString())
		}
		if tc.stringList != nil {
			v, ok := m.TryStringList(key)
			assert.True(t, ok)
			assert.Equal(t, tc.stringList.value(), v)
			assert.True(t, m.Get(key).IsStringList())
		} else {
			_, ok := m.TryStringList(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsStringList())
		}
		if tc.float64List != nil {
			v, ok := m.TryFloat64List(key)
			assert.True(t, ok)
			assert.Equal(t, tc.float64List.value(), v)
			assert.True(t, m.Get(key).IsFloat64List())
		} else {
			_, ok := m.TryFloat64List(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsFloat64List())
		}
		if tc.int64List != nil {
			v, ok := m.TryInt64List(key)
			assert.True(t, ok)
			assert.Equal(t, tc.int64List.value(), v)
			assert.True(t, m.Get(key).IsInt64List())
		} else {
			_, ok := m.TryInt64List(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsInt64List())
		}
		if tc.mapValue != nil {
			v, ok := m.TryMapValue(key)
			assert.True(t, ok)
			assert.Equal(t, tc.mapValue.value(), v)
			assert.True(t, m.Get(key).IsMapValue())
		} else {
			_, ok := m.TryMapValue(key)
			assert.False(t, ok)
			assert.False(t, m.Get(key).IsMapValue())
		}
	}
}

func TestGenericMap_Equal(t *testing.T) {
	t.Run("equal", func(t *testing.T) {
		t.Run("empty", func(t *testing.T) {
			m1 := NewGenericMap[string]()
			m2 := NewGenericMap[string]()

			assert.True(t, m1.Equal(m2))
			assert.True(t, m2.Equal(m1))
		})
		t.Run("both are nils", func(t *testing.T) {
			var (
				m1 *GenericMap[string]
				m2 *GenericMap[string]
			)
			assert.True(t, m1.Equal(m2))
			assert.True(t, m2.Equal(m1))
		})
		t.Run("single value", func(t *testing.T) {
			for _, val := range givenValues() {
				m1 := NewGenericMap[string]()
				m1.Set("key", val)
				m2 := NewGenericMap[string]()
				m2.Set("key", val)

				assert.True(t, m1.Equal(m2))
				assert.True(t, m2.Equal(m1))
			}
		})
		t.Run("many values", func(t *testing.T) {
			m1 := NewGenericMap[string]()
			m2 := NewGenericMap[string]()

			for i, val := range givenValues() {
				m1.Set(strconv.Itoa(i), val)
				m2.Set(strconv.Itoa(i), val)
			}

			assert.True(t, m1.Equal(m2))
			assert.True(t, m2.Equal(m1))
		})
	})

	t.Run("not equal", func(t *testing.T) {
		t.Run("one map is nil", func(t *testing.T) {
			m1 := NewGenericMap[string]()
			var m2 *GenericMap[string]
			assert.False(t, m1.Equal(m2))
			assert.False(t, m2.Equal(m1))
		})

		t.Run("single value", func(t *testing.T) {
			vals := givenValues()
			length := len(vals)
			for i := range vals {
				m1val := vals[i]
				// Next value in list
				m2val := vals[(i+1)%length]

				m1 := NewGenericMap[string]()
				m1.Set("key", m1val)
				m2 := NewGenericMap[string]()
				m2.Set("key", m2val)

				assert.False(t, m1.Equal(m2))
				assert.False(t, m2.Equal(m1))
			}
		})

		t.Run("multiple values", func(t *testing.T) {
			vals := givenValues()
			length := len(vals)

			m1 := NewGenericMap[string]()
			m2 := NewGenericMap[string]()
			for i, val := range vals {
				key := fmt.Sprintf("key-%d", i)
				m1.Set(key, val)
				m2.Set(key, vals[(i+1)%length])
			}

			assert.False(t, m1.Equal(m2))
			assert.False(t, m2.Equal(m1))
		})
	})
}

func TestGenericMap_ToProto(t *testing.T) {
	m := NewGenericMap[string]()
	for i, val := range givenValues() {
		m.Set(fmt.Sprintf("key-%d", i), val)
	}

	got := m.ToProto()

	want := &types.Struct{
		Fields: map[string]*types.Value{
			"key-0": pbtypes.Null(),
			"key-1": pbtypes.String("hi"),
			"key-2": pbtypes.Int64(123),
			"key-3": pbtypes.Float64(123.456),
			"key-4": pbtypes.Bool(true),
			"key-5": pbtypes.Bool(false),
			"key-6": pbtypes.StringList([]string{"1", "2", "3"}),
			"key-7": pbtypes.Float64List([]float64{1.1, 2.2, 3.3, 4.4}),
			"key-8": pbtypes.IntList(1, 2, 3),
			"key-9": pbtypes.Struct(&types.Struct{
				Fields: map[string]*types.Value{
					"int64":  pbtypes.Int64(101010),
					"string": pbtypes.String("string"),
				},
			}),
		},
	}

	assert.Equal(t, want, got)
}
