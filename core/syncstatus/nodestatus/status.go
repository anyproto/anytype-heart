package nodestatus

import (
	"sync"

	"github.com/anyproto/any-sync/app"
)

const CName = "core.syncstatus.nodestatus"

type nodeStatus struct {
	sync.Mutex
	nodeStatus map[string]ConnectionStatus
}

type ConnectionStatus int

const (
	Online ConnectionStatus = iota
	ConnectionError
	RemovedFromNetwork
)

type NodeStatus interface {
	app.Component
	SetNodesStatus(spaceId string, status ConnectionStatus)
	GetNodeStatus(spaceId string) ConnectionStatus
}

func NewNodeStatus() NodeStatus {
	return &nodeStatus{nodeStatus: make(map[string]ConnectionStatus)}
}

func (n *nodeStatus) Init(a *app.App) (err error) {
	return
}

func (n *nodeStatus) Name() (name string) {
	return CName
}

func (n *nodeStatus) GetNodeStatus(spaceId string) ConnectionStatus {
	n.Lock()
	defer n.Unlock()
	return n.nodeStatus[spaceId]
}

func (n *nodeStatus) SetNodesStatus(spaceId string, status ConnectionStatus) {
	n.Lock()
	defer n.Unlock()
	n.nodeStatus[spaceId] = status
}
