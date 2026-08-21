package acl

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/crypto"
)

type aclGetter struct {
	mu          sync.Mutex // guards currentAcls (Leave runs without aclClientLock and may run concurrently)
	currentAcls map[string]list.AclList
	aclClient   aclclient.AclJoiningClient
	keys        *accountdata.AccountKeys
	nodeConf    nodeconf.NodeConf
}

func newAclGetter(aclClient aclclient.AclJoiningClient, keys *accountdata.AccountKeys, nodeConf nodeconf.NodeConf) *aclGetter {
	return &aclGetter{
		currentAcls: make(map[string]list.AclList),
		aclClient:   aclClient,
		keys:        keys,
		nodeConf:    nodeConf,
	}
}

func (g *aclGetter) RemoveAcl(ctx context.Context, spaceId string) error {
	g.mu.Lock()
	delete(g.currentAcls, spaceId)
	g.mu.Unlock()
	return nil
}

func (g *aclGetter) GetOrRefreshAcl(ctx context.Context, spaceId string) (aclList list.AclList, err error) {
	g.mu.Lock()
	aclList, ok := g.currentAcls[spaceId]
	g.mu.Unlock()
	if ok {
		if err := g.refresh(ctx, spaceId, aclList); err != nil {
			return nil, err
		}
		return aclList, nil
	}
	// fetch outside the lock (network call), then publish, double-checking another goroutine didn't win.
	fetched, err := g.getAcl(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	if existing, ok := g.currentAcls[spaceId]; ok {
		fetched = existing
	} else {
		g.currentAcls[spaceId] = fetched
	}
	g.mu.Unlock()
	return fetched, nil
}

func (g *aclGetter) getAcl(ctx context.Context, spaceId string) (l list.AclList, err error) {
	res, err := g.aclClient.AclGetRecords(ctx, spaceId, "")
	if err != nil {
		return
	}
	if len(res) == 0 {
		err = fmt.Errorf("acl not found")
		return
	}
	storage, err := list.NewInMemoryStorage(res[0].Id, res)
	if err != nil {
		return
	}
	verifier := recordverifier.NewValidateFull()
	if networkId := g.nodeConf.Configuration().NetworkId; networkId != "" {
		netKey, decodeErr := crypto.DecodeNetworkId(networkId)
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid networkId: %w", decodeErr)
		}
		verifier = recordverifier.New(netKey)
	}
	return list.BuildAclListWithIdentity(g.keys, storage, verifier)
}

func (g *aclGetter) refresh(ctx context.Context, spaceId string, aclList list.AclList) (err error) {
	res, err := g.aclClient.AclGetRecords(ctx, spaceId, aclList.Head().Id)
	if err != nil {
		return
	}
	if len(res) == 0 {
		return
	}
	return aclList.AddRawRecords(res)
}
