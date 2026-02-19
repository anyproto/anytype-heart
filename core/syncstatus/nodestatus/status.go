package nodestatus

/*
AI generated

Name: Per-Space Node Connection Status Store
Scope: global

## Responsibility
- Thread-safe storage for node connection status per space
- Written by PeerManager when fetching responsible peers
- Read by SpaceSyncStatus to determine overall sync state
*/

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
	TryGetNodeStatus(spaceId string) (ConnectionStatus, bool)
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

func (n *nodeStatus) TryGetNodeStatus(spaceId string) (ConnectionStatus, bool) {
	n.Lock()
	defer n.Unlock()
	status, ok := n.nodeStatus[spaceId]
	return status, ok
}
