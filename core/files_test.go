package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnytype_FileByHash(t *testing.T) {
	s := getRunningService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	nf, err := s.FileAddWithBytes(ctx, []byte("123"), "file.txt")
	require.NoError(t, err)
	require.Len(t, nf.Hash(), 55)

	f, err := s.FileByHash(ctx, nf.Hash())
	require.NoError(t, err)
	require.Equal(t, nf.Hash(), f.Hash())
}
