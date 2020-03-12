package core

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var doOnce sync.Once
var s Service

func getRunningService(t *testing.T) Service{
	doOnce.Do(func(){
		s = createAccount(t)
		err := s.Start()
		require.NoError(t, err)
	})
	return s
}

func TestAnytype_IsStarted(t *testing.T) {
	s := getRunningService(t)
	require.True(t, s.IsStarted())
}

func TestAnytype_PredefinedBlocks(t *testing.T) {
	s := getRunningService(t)
	s.InitPredefinedBlocks(false)
	require.NotNil(t, s)
	require.Len(t, s.PredefinedBlocks().Home,57)
}
