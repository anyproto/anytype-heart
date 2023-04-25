package conc

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapError(t *testing.T) {
	t.Run("without error", func(t *testing.T) {
		in := []string{"1", "2", "3", "4"}

		want := []int{1, 2, 3, 4}

		got, err := MapErr(in, strconv.Atoi)

		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("with error", func(t *testing.T) {
		in := []string{"1", "b", "3", "d"}

		want := []int{1, 0, 3, 0}

		got, err := MapErr(in, strconv.Atoi)

		assert.Error(t, err)
		assert.Equal(t, want, got)
	})

}
