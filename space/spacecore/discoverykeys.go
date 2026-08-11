package spacecore

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

// negativeRetryAfter bounds how often a space whose key could not be derived
// is retried. Without it every LAN handshake rebuilds the ACL of every space
// we cannot read. Successful derivations are cached for the process lifetime:
// the first read key never rotates.
const negativeRetryAfter = time.Minute

// discoveryKeySource derives and caches the per-space LAN discovery keys the
// SpaceExchangeV2 tokens are built from. The key is HKDF over the space's
// first ACL read key, so deriving it requires the space's ACL: spaces not
// pulled yet, or whose join is not accepted, have no key and are simply left
// out of the exchange until one becomes available.
type discoveryKeySource struct {
	storage storage.ClientStorage
	keys    *accountdata.AccountKeys

	mu       sync.Mutex
	derived  map[string][]byte
	failedAt map[string]time.Time
}

func newDiscoveryKeySource(store storage.ClientStorage, keys *accountdata.AccountKeys) *discoveryKeySource {
	return &discoveryKeySource{
		storage:  store,
		keys:     keys,
		derived:  map[string][]byte{},
		failedAt: map[string]time.Time{},
	}
}

// DiscoveryKeys returns the discovery keys for the given spaces, keyed by
// spaceId. Spaces without a derivable key are absent from the result.
func (d *discoveryKeySource) DiscoveryKeys(ctx context.Context, spaceIds []string) map[string][]byte {
	out := make(map[string][]byte, len(spaceIds))
	for _, id := range spaceIds {
		if key := d.get(ctx, id); key != nil {
			out[id] = key
		}
	}
	return out
}

func (d *discoveryKeySource) get(ctx context.Context, spaceId string) []byte {
	d.mu.Lock()
	if key, ok := d.derived[spaceId]; ok {
		d.mu.Unlock()
		return key
	}
	if at, ok := d.failedAt[spaceId]; ok && time.Since(at) < negativeRetryAfter {
		d.mu.Unlock()
		return nil
	}
	d.mu.Unlock()

	key, err := d.derive(ctx, spaceId)

	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		log.Debug("discovery key unavailable", zap.String("spaceId", spaceId), zap.Error(err))
		d.failedAt[spaceId] = time.Now()
		return nil
	}
	delete(d.failedAt, spaceId)
	d.derived[spaceId] = key
	return key
}

// derive reads the space's ACL from storage and takes the first read key — the
// one minted at space creation, held by every member and never rotated.
func (d *discoveryKeySource) derive(ctx context.Context, spaceId string) ([]byte, error) {
	st, err := d.storage.WaitSpaceStorage(ctx, spaceId)
	if err != nil {
		return nil, fmt.Errorf("wait space storage: %w", err)
	}
	aclStorage, err := st.AclStorage()
	if err != nil {
		return nil, fmt.Errorf("acl storage: %w", err)
	}
	aclList, err := list.BuildAclListWithIdentity(d.keys, aclStorage, recordverifier.NewValidateFull())
	if err != nil {
		return nil, fmt.Errorf("build acl list: %w", err)
	}
	firstKeys, ok := aclList.AclState().Keys()[aclList.Id()]
	if !ok || firstKeys.ReadKey == nil {
		return nil, list.ErrNoReadKey
	}
	key, err := clientspaceproto.DeriveDiscoveryKey(firstKeys.ReadKey, spaceId)
	if err != nil {
		return nil, fmt.Errorf("derive discovery key: %w", err)
	}
	return key, nil
}
