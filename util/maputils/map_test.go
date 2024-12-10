package maputils

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyMap(t *testing.T) {
	t.Run("CopyMap with string keys and int values", func(t *testing.T) {
		// given
		original := map[string]int{
			"one":   1,
			"two":   2,
			"three": 3,
		}

		// when
		copied := CopyMap(original)

		// then
		assert.True(t, reflect.DeepEqual(original, copied))
		copied["one"] = 42
		assert.True(t, original["one"] != 42)
	})

	t.Run("CopyMap with int keys and string values", func(t *testing.T) {
		// given
		original := map[int]string{
			1: "apple",
			2: "banana",
			3: "cherry",
		}

		// when
		copied := CopyMap(original)

		// then
		assert.True(t, reflect.DeepEqual(original, copied))
		copied[1] = "grape"
		assert.True(t, original[1] != "grape")
	})
	t.Run("CopyMap with empty map", func(t *testing.T) {
		// given
		original := map[string]int{}

		// when
		copied := CopyMap(original)

		// then
		assert.Empty(t, copied)
	})
	t.Run("CopyMap with nil map", func(t *testing.T) {
		// given
		var original map[string]int

		// when
		copied := CopyMap(original)

		// then
		assert.Empty(t, copied)
	})
	t.Run("CopyMap with string keys and interface{} values", func(t *testing.T) {
		// given
		original := map[string]interface{}{
			"name": "John",
			"age":  30,
			"city": "New York",
		}

		// when
		copied := CopyMap(original)

		// then
		assert.True(t, reflect.DeepEqual(original, copied))
		copied["name"] = "Jane"
		assert.True(t, original["name"] != "Jane")
	})
}
