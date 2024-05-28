package nodestatus

import (
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

type fixture struct {
	*nodeStatus
	nodeConf *mock_nodeconf.MockService
}

func TestNodeStatus_SetNodesStatus(t *testing.T) {
	t.Run("peer is responsible", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.nodeConf.EXPECT().NodeIds("spaceId").Return([]string{"peerId"})

		// when
		f.SetNodesStatus("spaceId", "peerId", Online)

		// then
		assert.Equal(t, Online, f.nodeStatus.nodeStatus)
	})
	t.Run("peer is not responsible", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.nodeConf.EXPECT().NodeIds("spaceId").Return([]string{"peerId2"})

		// when
		f.SetNodesStatus("spaceId", "peerId", ConnectionError)

		// then
		assert.NotEqual(t, ConnectionError, f.nodeStatus.nodeStatus)
	})
}

func TestNodeStatus_GetNodeStatus(t *testing.T) {
	t.Run("get default status", func(t *testing.T) {
		// given
		f := newFixture(t)

		// when
		status := f.GetNodeStatus()

		// then
		assert.Equal(t, Online, status)
	})
	t.Run("get updated status", func(t *testing.T) {
		// given
		f := newFixture(t)
		f.nodeConf.EXPECT().NodeIds("spaceId").Return([]string{"peerId"})

		// when
		f.SetNodesStatus("spaceId", "peerId", ConnectionError)
		status := f.GetNodeStatus()

		// then
		assert.Equal(t, ConnectionError, status)
	})
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	nodeConf := mock_nodeconf.NewMockService(ctrl)
	nodeStatus := &nodeStatus{}
	a := &app.App{}
	a.Register(nodeConf)
	err := nodeStatus.Init(a)
	assert.Nil(t, err)
	return &fixture{
		nodeStatus: nodeStatus,
		nodeConf:   nodeConf,
	}
}
