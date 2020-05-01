package core

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var doOnce sync.Once
var s Service

func getRunningServiceB(t *testing.B) Service {
	doOnce.Do(func() {
		s = createAccount(t)
		err := s.Start()
		require.NoError(t, err)
	})
	return s
}

func getRunningService(t *testing.T) Service {
	doOnce.Do(func() {
		s = createAccount(t)
		err := s.Start()
		require.NoError(t, err)

		err = s.InitPredefinedBlocks(false)
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
	require.Equal(t, s.(*Anytype).t.Host().ID().String(), s.(*Anytype).opts.Device.Address())
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

	tid, err := ProfileThreadIDFromAccountAddress(s.Account())
	require.NoError(t, err)

	require.Equal(t, s.PredefinedBlocks().Profile, tid.String())
}

func TestAnytype_CreateBlock(t *testing.T) {
	s := getRunningService(t)
	block, err := s.CreateBlock(SmartBlockTypePage)
	require.NoError(t, err)
	require.Equal(t, block.Type(), SmartBlockTypePage)
	require.Len(t, block.ID(), 57)
}
