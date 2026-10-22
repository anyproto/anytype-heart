package spacecore

import (
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacedomain"
)

// GO-7492: a cold device has no space storage, so AllSpaceIds is empty and
// the LAN exchange asks about nothing — a peer holding the whole account sits
// idle while everything is pulled from a network node. The tech space is the
// one space that can be asked about before it exists locally: it is DERIVED,
// so both its id and its first ACL read key follow from the account keys.
// The exchange token needs the discovery key, HKDF over that read key, which
// discoveryKeySource normally reads from the space's ACL on disk; here the
// same ACL is built in memory from the derive payload instead.

type consensusRecord = consensusproto.RawRecordWithId

// techSpaceExchange is the derived tech space's id and discovery key,
// computed once per process.
type techSpaceExchange struct {
	id  string
	key []byte
}

// techSpaceExchangeInfo returns the tech space's id and discovery key, seeding
// the discovery key source on first use. Empty on failure: the exchange then
// behaves exactly as before.
func (s *service) techSpaceExchangeInfo() techSpaceExchange {
	s.techSpaceOnce.Do(func() {
		info, err := s.deriveTechSpaceExchange()
		if err != nil {
			log.Warn("derive tech space discovery key", zap.Error(err))
			return
		}
		s.discoveryKeys.seed(info.id, info.key)
		s.techSpace = info
	})
	return s.techSpace
}

func (s *service) deriveTechSpaceExchange() (techSpaceExchange, error) {
	payload, err := s.deriveStoragePayload(spacedomain.SpaceTypeTech)
	if err != nil {
		return techSpaceExchange{}, fmt.Errorf("derive storage payload: %w", err)
	}
	id := payload.SpaceHeaderWithId.Id
	// the same reading discoveryKeySource.derive performs on disk, over the
	// deterministic derived root
	aclStorage, err := list.NewInMemoryStorage(payload.AclWithId.Id, []*consensusRecord{payload.AclWithId})
	if err != nil {
		return techSpaceExchange{}, fmt.Errorf("in-memory acl storage: %w", err)
	}
	aclList, err := list.BuildAclListWithIdentity(s.accountKeys, aclStorage, recordverifier.NewValidateFull())
	if err != nil {
		return techSpaceExchange{}, fmt.Errorf("build acl list: %w", err)
	}
	firstKeys, ok := aclList.AclState().Keys()[aclList.Id()]
	if !ok || firstKeys.ReadKey == nil {
		return techSpaceExchange{}, list.ErrNoReadKey
	}
	key, err := clientspaceproto.DeriveDiscoveryKey(firstKeys.ReadKey, id)
	if err != nil {
		return techSpaceExchange{}, fmt.Errorf("derive discovery key: %w", err)
	}
	return techSpaceExchange{id: id, key: key}, nil
}

// exchangeRequestIds is the space list an OUTBOUND exchange asks about:
// everything on disk plus the derived tech space. Only the request adds it —
// a responder that does not hold the tech space must not claim to share it,
// or two cold devices of one account would "share" a space neither has.
func (s *service) exchangeRequestIds() ([]string, error) {
	ids, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return nil, fmt.Errorf("all space ids: %w", err)
	}
	tech := s.techSpaceExchangeInfo()
	if tech.id == "" || slices.Contains(ids, tech.id) {
		return ids, nil
	}
	return append(ids, tech.id), nil
}
