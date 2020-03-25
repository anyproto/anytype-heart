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

func TestAnytype_DeviceKeyEquals(t *testing.T) {
	s := getRunningService(t)
	require.Equal(t, s.(*Anytype).ts.Host().ID().String(), s.(*Anytype).device.Address())
}

func TestAnytype_PredefinedBlocks(t *testing.T) {
	s := getRunningService(t)
	require.NotNil(t, s)

	err := s.InitPredefinedBlocks(false)
	require.NoError(t, err)

	fmt.Printf("profile: %s\n", s.PredefinedBlocks().Profile)
	fmt.Printf("home: %s\n", s.PredefinedBlocks().Home)

	require.Len(t, s.PredefinedBlocks().Home, 57)
	require.Len(t, s.PredefinedBlocks().Profile, 57)
	require.Len(t, s.PredefinedBlocks().Archive, 57)
}

func TestAnytype_CreateBlock(t *testing.T) {
	s := getRunningService(t)
	block, err := s.CreateBlock(SmartBlockTypePage)
	require.NoError(t, err)
	require.Equal(t, block.Type(), SmartBlockTypePage)
	require.Len(t, block.ID(), 57)
}
