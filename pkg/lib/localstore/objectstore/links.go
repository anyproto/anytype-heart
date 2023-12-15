package objectstore

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type BacklinksUpdateInfo struct {
	Id             string
	Added, Removed []string
}

func (s *dsObjectStore) GetWithLinksInfoByID(spaceID string, id string) (*model.ObjectInfoWithLinks, error) {
	var res *model.ObjectInfoWithLinks
	err := s.db.View(func(txn *badger.Txn) error {
		pages, err := s.getObjectsInfo(txn, spaceID, []string{id})
		if err != nil {
			return err
		}

		if len(pages) == 0 {
			return fmt.Errorf("page not found")
		}
		page := pages[0]

		inboundIds, err := findInboundLinks(txn, id)
		if err != nil {
			return fmt.Errorf("find inbound links: %w", err)
		}
		outboundsIds, err := findOutboundLinks(txn, id)
		if err != nil {
			return fmt.Errorf("find outbound links: %w", err)
		}

		inbound, err := s.getObjectsInfo(txn, spaceID, inboundIds)
		if err != nil {
			return err
		}

		outbound, err := s.getObjectsInfo(txn, spaceID, outboundsIds)
		if err != nil {
			return err
		}

		res = &model.ObjectInfoWithLinks{
			Id:   id,
			Info: page,
			Links: &model.ObjectLinksInfo{
				Inbound:  inbound,
				Outbound: outbound,
			},
		}
		return nil
	})
	return res, err
}

func (s *dsObjectStore) GetOutboundLinksByID(id string) ([]string, error) {
	var links []string
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		links, err = findOutboundLinks(txn, id)
		return err
	})
	return links, err
}

func (s *dsObjectStore) GetInboundLinksByID(id string) ([]string, error) {
	var links []string
	err := s.db.View(func(txn *badger.Txn) error {
		var err error
		links, err = findInboundLinks(txn, id)
		return err
	})
	return links, err
}

func (s *dsObjectStore) SubscribeBacklinksUpdate() <-chan BacklinksUpdateInfo {
	s.backlinksUpdateCh = make(chan BacklinksUpdateInfo)
	return s.backlinksUpdateCh
}

// Find to which IDs specified one has outbound links.
func findOutboundLinks(txn *badger.Txn, id string) ([]string, error) {
	return listIDsByPrefix(txn, pagesOutboundLinksBase.ChildString(id).Bytes())
}

// Find from which IDs specified one has inbound links.
func findInboundLinks(txn *badger.Txn, id string) ([]string, error) {
	return listIDsByPrefix(txn, pagesInboundLinksBase.ChildString(id).Bytes())
}

func pageLinkKeys(id string, out []string) []ds.Key {
	keys := make([]ds.Key, 0, 2*len(out))
	// links outgoing from specified node id
	for _, to := range out {
		keys = append(keys, outgoingLinkKey(id, to), inboundLinkKey(id, to))
	}
	return keys
}

func outgoingLinkKey(from, to string) ds.Key {
	return pagesOutboundLinksBase.ChildString(from).ChildString(to)
}

func inboundLinkKey(from, to string) ds.Key {
	return pagesInboundLinksBase.ChildString(to).ChildString(from)
}
