package common

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestGetValueAsString(t *testing.T) {
	t.Run("string value from details", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		details.SetString("testKey", "testValue")

		// when
		result := GetValueAsString(details, nil, "testKey")

		// then
		assert.Equal(t, "testValue", result)
	})

	t.Run("fallback to localDetails", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		localDetails := domain.NewDetails()
		localDetails.SetString("testKey", "localValue")

		// when
		result := GetValueAsString(details, localDetails, "testKey")

		// then
		assert.Equal(t, "localValue", result)
	})

	t.Run("bool value conversion", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		details.SetBool("boolKey", true)

		// when
		result := GetValueAsString(details, nil, "boolKey")

		// then
		assert.Equal(t, "true", result)
	})

	t.Run("float value conversion", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		details.SetFloat64("floatKey", 3.14)

		// when
		result := GetValueAsString(details, nil, "floatKey")

		// then
		assert.Equal(t, "3.14", result)
	})

	t.Run("integer value conversion", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		details.SetInt64("intKey", 42)

		// when
		result := GetValueAsString(details, nil, "intKey")

		// then
		assert.Equal(t, "42", result)
	})

	t.Run("string list conversion", func(t *testing.T) {
		// given
		details := domain.NewDetails()
		details.SetStringList("listKey", []string{"one", "two", "three"})

		// when
		result := GetValueAsString(details, nil, "listKey")

		// then
		assert.Equal(t, "one, two, three", result)
	})

	t.Run("missing key returns empty string", func(t *testing.T) {
		// given
		details := domain.NewDetails()

		// when
		result := GetValueAsString(details, nil, "missingKey")

		// then
		assert.Equal(t, "", result)
	})
}
