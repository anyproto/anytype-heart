package pubsub

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/util/crypto"
)

// The engine dep implementations below back pubsub with the same primitives
// the sync path uses: the space ACL for keys and membership, and the shared
// peer pool for connectivity.

var errNoReadKey = errors.New("no read key for key id")

// Encrypt implements anysyncpubsub.Crypto: payloads are encrypted with the
// space's current ACL read key — the same key object-tree changes are
// encrypted with — carrying its id for the receiver's key lookup.
func (s *service) Encrypt(spaceId string, payload []byte) (keyId string, encrypted []byte, err error) {
	sp, err := s.spaceCore.Get(context.Background(), spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	acl := sp.Acl()
	acl.RLock()
	defer acl.RUnlock()
	state := acl.AclState()
	keyId = state.CurrentReadKeyId()
	key, err := state.CurrentReadKey()
	if err != nil {
		return "", nil, fmt.Errorf("current read key: %w", err)
	}
	encrypted, err = key.Encrypt(payload)
	if err != nil {
		return "", nil, fmt.Errorf("encrypt payload: %w", err)
	}
	return keyId, encrypted, nil
}

// Decrypt implements anysyncpubsub.Crypto: keyId resolves to a historical read
// key held in the ACL state, so messages encrypted just before a key rotation
// still decrypt.
func (s *service) Decrypt(spaceId, keyId string, encrypted []byte) ([]byte, error) {
	sp, err := s.spaceCore.Get(context.Background(), spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	acl := sp.Acl()
	acl.RLock()
	defer acl.RUnlock()
	keys, ok := acl.AclState().Keys()[keyId]
	if !ok || keys.ReadKey == nil {
		return nil, fmt.Errorf("resolve key %s: %w", keyId, errNoReadKey)
	}
	decrypted, err := keys.ReadKey.Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return decrypted, nil
}

// CheckMember implements anysyncpubsub.MembershipChecker, gating the inbound
// LAN subscribes and publishes we serve. Only spaces already loaded are
// served (Pick, no load), so LAN peers can't make us load arbitrary spaces.
func (s *service) CheckMember(ctx context.Context, spaceId string, identity crypto.PubKey) error {
	sp, err := s.spaceCore.Pick(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("pick space: %w", err)
	}
	acl := sp.Acl()
	acl.RLock()
	defer acl.RUnlock()
	if acl.AclState().Permissions(identity).NoPermissions() {
		return list.ErrNoSuchAccount
	}
	return nil
}

// SpacePeers implements anysyncpubsub.PeerProvider: publishes and interest go
// to the space's responsible sync node plus every LAN-discovered peer — the
// same peer set the sync stream path uses.
func (s *service) SpacePeers(ctx context.Context, spaceId string) (peers []peer.Peer, err error) {
	if nodeIds := s.peerStore.ResponsibleNodeIds(spaceId); len(nodeIds) > 0 {
		nodePeer, nodeErr := s.pool.GetOneOf(ctx, nodeIds)
		if nodeErr != nil {
			log.With("spaceId", spaceId).Debug("pubsub: no responsible node reachable")
		} else {
			peers = append(peers, nodePeer)
		}
	}
	for _, peerId := range s.peerStore.LocalPeerIds(spaceId) {
		localPeer, localErr := s.pool.Get(ctx, peerId)
		if localErr != nil {
			continue
		}
		peers = append(peers, localPeer)
	}
	if len(peers) == 0 {
		return nil, fmt.Errorf("space %s: no pubsub peers", spaceId)
	}
	return peers, nil
}
