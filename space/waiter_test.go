package space

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

func TestWaiter_Wait(t *testing.T) {
	t.Run("space exists", func(t *testing.T) {
		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockClientSpace := mock_clientspace.NewMockSpace(t)
		retryDelay := time.Millisecond
		stub := &waiterStub{
			clientSpace: mockClientSpace,
			techSpace:   mockTechSpace,
			exists:      []bool{true},
		}
		wtr := newSpaceWaiter(stub, ctx, retryDelay)
		mockTechSpace.EXPECT().TechSpaceId().Return("techSpaceId")
		mockTechSpace.EXPECT().WaitViews().Return(nil)
		mockTechSpace.EXPECT().SpaceViewExists(ctx, "spaceId").Return(true, nil)
		res, err := wtr.waitSpace(ctx, "spaceId")
		require.NoError(t, err)
		require.NotNil(t, res)
	})
	t.Run("wait multiple times", func(t *testing.T) {
		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockClientSpace := mock_clientspace.NewMockSpace(t)
		retryDelay := time.Millisecond
		stub := &waiterStub{
			clientSpace: mockClientSpace,
			techSpace:   mockTechSpace,
			exists:      []bool{false, false, true},
		}
		wtr := newSpaceWaiter(stub, ctx, retryDelay)
		mockTechSpace.EXPECT().TechSpaceId().Return("techSpaceId")
		mockTechSpace.EXPECT().WaitViews().Return(nil)
		mockTechSpace.EXPECT().SpaceViewExists(ctx, "spaceId").Return(true, nil)
		res, err := wtr.waitSpace(ctx, "spaceId")
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, 3, stub.cntr)
	})
	t.Run("cancel wait, service closed", func(t *testing.T) {
		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockClientSpace := mock_clientspace.NewMockSpace(t)
		retryDelay := time.Second
		stub := &waiterStub{
			clientSpace: mockClientSpace,
			techSpace:   mockTechSpace,
			exists:      []bool{false, false, true},
		}
		cancelCtx, cancel := context.WithCancel(context.Background())
		wtr := newSpaceWaiter(stub, cancelCtx, retryDelay)
		mockTechSpace.EXPECT().TechSpaceId().Return("techSpaceId")
		mockTechSpace.EXPECT().WaitViews().Return(nil)
		mockTechSpace.EXPECT().SpaceViewExists(ctx, "spaceId").Return(true, nil)
		cancel()
		res, err := wtr.waitSpace(ctx, "spaceId")
		require.Error(t, err)
		require.Nil(t, res)
		require.Equal(t, 1, stub.cntr)
	})
	t.Run("wait failed", func(t *testing.T) {
		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockClientSpace := mock_clientspace.NewMockSpace(t)
		retryDelay := time.Millisecond
		stub := &waiterStub{
			clientSpace: mockClientSpace,
			techSpace:   mockTechSpace,
		}
		wtr := newSpaceWaiter(stub, ctx, retryDelay)
		mockTechSpace.EXPECT().TechSpaceId().Return("techSpaceId")
		mockTechSpace.EXPECT().WaitViews().Return(fmt.Errorf("error"))
		_, err := wtr.waitSpace(ctx, "spaceId")
		require.Error(t, err)
	})
	t.Run("space view not exists", func(t *testing.T) {
		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockClientSpace := mock_clientspace.NewMockSpace(t)
		retryDelay := time.Millisecond
		stub := &waiterStub{
			clientSpace: mockClientSpace,
			techSpace:   mockTechSpace,
			exists:      []bool{true},
		}
		// cancelCtx, cancel := context.WithCancel(context.Background())
		wtr := newSpaceWaiter(stub, ctx, retryDelay)
		mockTechSpace.EXPECT().TechSpaceId().Return("techSpaceId")
		mockTechSpace.EXPECT().WaitViews().Return(nil)
		mockTechSpace.EXPECT().SpaceViewExists(ctx, "spaceId").Return(false, nil)
		_, err := wtr.waitSpace(ctx, "spaceId")
		require.Equal(t, ErrSpaceNotExists, err)
	})
}

type waiterStub struct {
	techSpace   techspace.TechSpace
	clientSpace clientspace.Space
	err         error
	exists      []bool
	cntr        int
}

func (w *waiterStub) TechSpace() *clientspace.TechSpace {
	return &clientspace.TechSpace{
		TechSpace: w.techSpace,
	}
}

func (w *waiterStub) Get(ctx context.Context, spaceId string) (clientspace.Space, error) {
	if w.err != nil {
		return nil, w.err
	}
	return w.clientSpace, nil
}

func (w *waiterStub) checkControllerExists(spaceId string) bool {
	defer func() {
		w.cntr++
	}()
	return w.exists[w.cntr]
}
