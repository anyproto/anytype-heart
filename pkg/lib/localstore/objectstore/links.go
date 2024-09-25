package objectstore

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type LinksUpdateInfo struct {
	LinksFromId    string
	Added, Removed []string
}

const linkOutboundField = "o"

func (s *dsObjectStore) GetWithLinksInfoByID(spaceId string, id string) (*model.ObjectInfoWithLinks, error) {
	txn, err := s.links.ReadTx(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("read txn: %w", err)
	}
	commit := func(err error) error {
		return errors.Join(txn.Commit(), err)
	}
	pages, err := s.getObjectsInfo(txn.Context(), spaceId, []string{id})
	if err != nil {
		return nil, commit(err)
	}

	if len(pages) == 0 {
		return nil, commit(fmt.Errorf("page not found"))
	}
	page := pages[0]

	inboundIds, err := s.findInboundLinks(txn.Context(), id)
	if err != nil {
		return nil, commit(fmt.Errorf("find inbound links: %w", err))
	}
	outboundsIds, err := s.findOutboundLinks(txn.Context(), id)
	if err != nil {
		return nil, commit(fmt.Errorf("find outbound links: %w", err))
	}

	inbound, err := s.getObjectsInfo(s.componentCtx, spaceId, inboundIds)
	if err != nil {
		return nil, commit(err)
	}

	outbound, err := s.getObjectsInfo(s.componentCtx, spaceId, outboundsIds)
	if err != nil {
		return nil, commit(err)
	}

	err = txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit txn: %w", err)
	}
	return &model.ObjectInfoWithLinks{
		Id:   id,
		Info: page,
		Links: &model.ObjectLinksInfo{
			Inbound:  inbound,
			Outbound: outbound,
		},
	}, nil
}

func (s *dsObjectStore) GetOutboundLinksByID(spaceId string, id string) ([]string, error) {
	return s.findOutboundLinks(s.componentCtx, id)
}

func (s *dsObjectStore) GetInboundLinksByID(spaceId string, id string) ([]string, error) {
	return s.findInboundLinks(s.componentCtx, id)
}

func (s *dsObjectStore) SubscribeLinksUpdate(callback func(info LinksUpdateInfo)) {
	s.Lock()
	s.onLinksUpdateCallback = callback
	s.Unlock()
}

// Find to which IDs specified one has outbound links.
func (s *dsObjectStore) findOutboundLinks(ctx context.Context, id string) ([]string, error) {
	doc, err := s.links.FindId(ctx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	arr := doc.Value().GetArray(linkOutboundField)
	return pbtypes.JsonArrayToStrings(arr), nil
}

// Find from which IDs specified one has inbound links.
func (s *dsObjectStore) findInboundLinks(ctx context.Context, id string) ([]string, error) {
	iter, err := s.links.Find(query.Key{Path: []string{linkOutboundField}, Filter: query.NewComp(query.CompOpEq, id)}).Iter(ctx)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var links []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), fmt.Errorf("get doc: %w", err))
		}
		links = append(links, string(doc.Value().GetStringBytes("id")))
	}
	return links, iter.Close()
}
