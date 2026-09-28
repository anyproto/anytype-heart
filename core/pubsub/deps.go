package pubsub

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/pubsub/pubsubproto"
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
	sp, err := s.spaceCore.Get(s.ctx, spaceId)
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
	// The engine checks plaintext on publish but ciphertext on receipt.
	// Reject oversized ciphertext before it can be echoed or queued for send.
	if len(encrypted) > maxEncryptedPayloadSize {
		return "", nil, pubsubproto.ErrInvalidMessage
	}
	return keyId, encrypted, nil
}

// Decrypt implements anysyncpubsub.Crypto: keyId resolves to a historical read
// key held in the ACL state, so messages encrypted just before a key rotation
// still decrypt.
func (s *service) Decrypt(spaceId, keyId string, encrypted []byte) ([]byte, error) {
	sp, err := s.spaceCore.Get(s.ctx, spaceId)
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

// SpacePeers implements anysyncpubsub.PeerProvider using the shared sync peer
// pool. Connected node and LAN routes take precedence over pending dials.
func (s *service) SpacePeers(ctx context.Context, spaceId string) (peers []peer.Peer, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	nodeIds := s.peerStore.ResponsibleNodeIds(spaceId)
	localIds := s.peerStore.LocalPeerIds(spaceId)
	// Pick can wait behind a dial already in flight. A canceled context makes
	// this a nonblocking scan; completed cache loads still win in ocache.Pick.
	pickCtx, cancelPick := context.WithCancel(ctx)
	cancelPick()
	seen := make(map[string]bool)
	for _, id := range nodeIds {
		if p, e := s.pool.Pick(pickCtx, id); e == nil {
			peers = append(peers, p)
			seen[p.Id()] = true
			break
		}
	}
	for _, id := range localIds {
		if p, e := s.pool.Pick(pickCtx, id); e == nil && !seen[p.Id()] {
			peers = append(peers, p)
			seen[p.Id()] = true
		}
	}
	if len(peers) > 0 {
		// The Space's sync peer manager keeps dialing other candidates in the
		// shared pool. Ephemeral messages need not wait for those connections.
		return peers, nil
	}

	// With no cached route, race node and LAN discovery. A stalled node must
	// not prevent LAN delivery (or vice versa), or occupy a dial worker forever.
	// This deadline is only for lookup, never for the persistent pubsub stream.
	dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelDial()
	stop := context.AfterFunc(s.ctx, cancelDial)
	defer stop()
	results := make(chan peer.Peer, 2)
	go func() {
		var p peer.Peer
		if len(nodeIds) > 0 {
			p, _ = s.pool.GetOneOf(dialCtx, nodeIds)
		}
		results <- p
	}()
	go func() {
		for _, id := range localIds {
			p, e := s.pool.Get(dialCtx, id)
			if e == nil {
				results <- p
				return
			}
		}
		results <- nil
	}()
	for range 2 {
		select {
		case p := <-results:
			if p != nil {
				return []peer.Peer{p}, nil
			}
		case <-dialCtx.Done():
			return nil, dialCtx.Err()
		}
	}
	return nil, fmt.Errorf("space %s: no pubsub peers", spaceId)
}
