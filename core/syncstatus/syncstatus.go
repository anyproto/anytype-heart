package syncstatus

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/node"
	"github.com/anyproto/any-sync/nodeconf"
	"golang.org/x/exp/slices"
)

const nodeStatusServiceName = "nodeStatusService"

type nodeStatusService struct {
	sync.Mutex
	configuration nodeconf.NodeConf
	nodeStatus    node.ConnectionStatus
	spaceId       string
}

func NewNodeStatus(spaceId string) node.NodeStatus {
	return &nodeStatusService{spaceId: spaceId}
}

func (s *nodeStatusService) Init(a *app.App) (err error) {
	s.configuration = a.MustComponent(nodeconf.CName).(nodeconf.NodeConf)
	return
}

func (s *nodeStatusService) Name() (name string) {
	return nodeStatusServiceName
}

func (s *nodeStatusService) SetNodesStatus(senderId string, status node.ConnectionStatus) {
	if !s.isSenderResponsible(senderId) {
		return
	}

	s.Lock()
	defer s.Unlock()

	s.nodeStatus = status
}

func (s *nodeStatusService) isSenderResponsible(senderId string) bool {
	return slices.Contains(s.configuration.NodeIds(s.spaceId), senderId)
}

func (s *nodeStatusService) GetNodeStatus(senderId string) node.ConnectionStatus {
	if !s.isSenderResponsible(senderId) {
		return node.Online
	}
	s.Lock()
	defer s.Unlock()
	return s.nodeStatus
}
