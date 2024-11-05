package domain

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenericMap_Set(t *testing.T) {
	for _, val := range []Value{
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
	} {
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
