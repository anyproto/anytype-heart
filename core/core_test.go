package core

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var doOnce sync.Once
var s Service

func getRunningService(t *testing.T) Service {
	doOnce.Do(func() {
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

	fmt.Printf("profile: %s\n", s.PredefinedBlocks().Profile)
	fmt.Printf("home: %s\n", s.PredefinedBlocks().Home)

	require.Len(t, s.PredefinedBlocks().Home, 57)
}
