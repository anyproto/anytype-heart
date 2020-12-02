package symmetric

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRandom(t *testing.T) {
	key, err := NewRandom()
	if err != nil {
		t.Fatal(err)
	}

	require.Len(t, key, KeyBytes)
}
