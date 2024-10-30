package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenericMap_Set(t *testing.T) {
	m := NewGenericMap[string]()

	m.Set("key_string", String("string!"))
	assert.Equal(t, String("string!"), m.Get("key_string"))

}
