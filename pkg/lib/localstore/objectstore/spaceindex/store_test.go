package spaceindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewWithError(t *testing.T) {
	spaceIndex := New(context.Background(), "space1", Deps{
		// Crazy path to force an error
		DbPath: "....\\\\\\....///////****",
	})

	_, err := spaceIndex.ListIds()
	require.Error(t, err)
}
