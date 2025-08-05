package spacestatus

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*spaceStatus
	a             *app.App
	mockTechSpace *mock_techspace.MockTechSpace
	mockSpaceView *mock_techspace.MockSpaceView
}

var ctx = context.Background()

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		spaceStatus:   New("spaceId").(*spaceStatus),
		a:             &app.App{},
		mockTechSpace: mock_techspace.NewMockTechSpace(t),
		mockSpaceView: mock_techspace.NewMockSpaceView(t),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockTechSpace)).
		Register(fx.spaceStatus)
	fx.mockTechSpace.EXPECT().GetSpaceView(context.Background(), "spaceId").Return(fx.mockSpaceView, nil)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func TestSpaceStatus_GetLatestAclHeadId(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	info := spaceinfo.NewSpacePersistentInfo("spaceId")
	info.SetAclHeadId("aclHeadId")
	fx.mockSpaceView.EXPECT().GetPersistentInfo().Return(info)
	require.Equal(t, "aclHeadId", fx.GetLatestAclHeadId())
}

func TestSpaceStatus_SetPersistentStatus(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	info := spaceinfo.NewSpacePersistentInfo("spaceId")
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	fx.mockSpaceView.EXPECT().SetSpacePersistentInfo(info).Return(nil)
	require.NoError(t, fx.SetPersistentStatus(spaceinfo.AccountStatusActive))
}

func TestSpaceStatus_SetLocalStatus(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	info := spaceinfo.NewSpaceLocalInfo("spaceId")
	info.SetLocalStatus(spaceinfo.LocalStatusOk)
	fx.mockSpaceView.EXPECT().SetSpaceLocalInfo(info).Return(nil)
	require.NoError(t, fx.SetLocalStatus(spaceinfo.LocalStatusOk))
}

func TestSpaceStatus_SetLocalInfo(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	info := spaceinfo.NewSpaceLocalInfo("spaceId")
	info.SetLocalStatus(spaceinfo.LocalStatusOk)
	fx.mockSpaceView.EXPECT().SetSpaceLocalInfo(info).Return(nil)
	require.NoError(t, fx.SetLocalInfo(info))
}

func TestSpaceStatus_SetPersistentInfo(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	info := spaceinfo.NewSpacePersistentInfo("spaceId")
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	fx.mockSpaceView.EXPECT().SetSpacePersistentInfo(info).Return(nil)
	require.NoError(t, fx.SetPersistentInfo(info))
}

func TestSpaceStatus_GetPersistentStatus(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	info := spaceinfo.NewSpacePersistentInfo("spaceId")
	info.SetAccountStatus(spaceinfo.AccountStatusActive)
	fx.mockSpaceView.EXPECT().GetPersistentInfo().Return(info)
	require.Equal(t, spaceinfo.AccountStatusActive, fx.GetPersistentStatus())
}

func TestSpaceStatus_SetAclInfo(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	fx.mockSpaceView.EXPECT().SetAclInfo(true, nil, nil, mock.Anything).Return(nil)
	require.NoError(t, fx.SetAclInfo(true, nil, nil, time.Now().Unix()))
}

func TestSpaceStatus_SetAccessType(t *testing.T) {
	fx := newFixture(t)
	fx.mockSpaceView.EXPECT().Lock().Return()
	fx.mockSpaceView.EXPECT().Unlock().Return()
	fx.mockSpaceView.EXPECT().SetAccessType(spaceinfo.AccessTypePersonal).Return(nil)
	require.NoError(t, fx.SetAccessType(spaceinfo.AccessTypePersonal))
}
