package acl

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/crypto"
)

type aclGetter struct {
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
	delete(g.currentAcls, spaceId)
	return nil
}

func (g *aclGetter) GetOrRefreshAcl(ctx context.Context, spaceId string) (aclList list.AclList, err error) {
	aclList, ok := g.currentAcls[spaceId]
	if !ok {
		aclList, err = g.getAcl(ctx, spaceId)
		if err != nil {
			return nil, err
		}
		g.currentAcls[spaceId] = aclList
	} else {
		if err := g.refresh(ctx, spaceId, aclList); err != nil {
			return nil, err
		}
	}
	return aclList, nil
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
	verifier := recordverifier.AcceptorVerifier(recordverifier.NewValidateFull())
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
