package nodestatus

import (
	"slices"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
)

const CName = "core.syncstatus.nodestatus"

type nodeStatus struct {
	sync.Mutex
	configuration nodeconf.NodeConf
	nodeStatus    ConnectionStatus
}

type ConnectionStatus int

const (
	Online ConnectionStatus = iota
	ConnectionError
	RemovedFromNetwork
)

type NodeStatus interface {
	app.Component
	SetNodesStatus(spaceId string, senderId string, status ConnectionStatus)
	GetNodeStatus() ConnectionStatus
}

func NewNodeStatus() NodeStatus {
	return &nodeStatus{}
}

func (n *nodeStatus) Init(a *app.App) (err error) {
	n.configuration = app.MustComponent[nodeconf.NodeConf](a)
	return
}

func (n *nodeStatus) Name() (name string) {
	return CName
}

func (n *nodeStatus) GetNodeStatus() ConnectionStatus {
	n.Lock()
	defer n.Unlock()
	return n.nodeStatus
}

func (n *nodeStatus) SetNodesStatus(spaceId string, senderId string, status ConnectionStatus) {
	if !n.isSenderResponsible(senderId, spaceId) {
		return
	}

	n.Lock()
	defer n.Unlock()

	n.nodeStatus = status
}

func (n *nodeStatus) isSenderResponsible(senderId string, spaceId string) bool {
	return slices.Contains(n.configuration.NodeIds(spaceId), senderId)
}
