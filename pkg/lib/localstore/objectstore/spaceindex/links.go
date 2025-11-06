package spaceindex

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	linkOutboundField = "o"
	linkDetailedField = "od" // outgoing detailed
	linkTargetField   = "t"  // target id
	linkBlockField    = "b"  // block id
	linkRelationField = "r"  // relation key
)

func (s *dsObjectStore) GetWithLinksInfoById(id string) (*model.ObjectInfoWithLinks, error) {
	txn, err := s.links.ReadTx(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("read txn: %w", err)
	}
	defer txn.Commit()
	pages, err := s.getObjectsInfo(txn.Context(), []string{id})
	if err != nil {
		return nil, err
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("page not found")
	}
	page := pages[0]

	inboundIds, err := s.findInboundLinks(txn.Context(), id)
	if err != nil {
		return nil, fmt.Errorf("find inbound links: %w", err)
	}
	outboundsIds, err := s.findOutboundLinks(txn.Context(), id)
	if err != nil {
		return nil, fmt.Errorf("find outbound links: %w", err)
	}

	inbound, err := s.getObjectsInfo(txn.Context(), inboundIds)
	if err != nil {
		return nil, err
	}

	outbound, err := s.getObjectsInfo(txn.Context(), outboundsIds)
	if err != nil {
		return nil, err
	}

	inboundProto := make([]*model.ObjectInfo, 0, len(inbound))
	for _, info := range inbound {
		inboundProto = append(inboundProto, info.ToProto())
	}
	outboundProto := make([]*model.ObjectInfo, 0, len(outbound))
	for _, info := range outbound {
		outboundProto = append(outboundProto, info.ToProto())
	}
	return &model.ObjectInfoWithLinks{
		Id:   id,
		Info: page.ToProto(),
		Links: &model.ObjectLinksInfo{
			Inbound:  inboundProto,
			Outbound: outboundProto,
		},
	}, nil
}

func (s *dsObjectStore) GetOutboundLinksById(id string) ([]string, error) {
	return s.findOutboundLinks(s.componentCtx, id)
}

func (s *dsObjectStore) GetInboundLinksById(id string) ([]string, error) {
	return s.findInboundLinks(s.componentCtx, id)
}

func (s *dsObjectStore) GetOutboundLinksDetailedById(id string) ([]OutgoingLink, error) {
	return s.findOutboundLinksDetailed(s.componentCtx, id)
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
	return anyEncArrayToStrings(arr), nil
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
	defer iter.Close()

	var links []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		links = append(links, string(doc.Value().GetStringBytes("id")))
	}
	return links, nil
}

// Find detailed outgoing links with source information
func (s *dsObjectStore) findOutboundLinksDetailed(ctx context.Context, id string) ([]OutgoingLink, error) {
	doc, err := s.links.FindId(ctx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Try to get detailed links first
	detailedLinksArr := doc.Value().GetArray(linkDetailedField)
	if len(detailedLinksArr) > 0 {
		var links []OutgoingLink
		for _, linkVal := range detailedLinksArr {
			link := OutgoingLink{
				TargetID:    string(linkVal.GetStringBytes(linkTargetField)),
				BlockID:     string(linkVal.GetStringBytes(linkBlockField)),
				RelationKey: string(linkVal.GetStringBytes(linkRelationField)),
			}
			links = append(links, link)
		}
		return links, nil
	}

	// Fallback to simple links if detailed links not available
	arr := doc.Value().GetArray(linkOutboundField)
	targetIds := anyEncArrayToStrings(arr)
	var links []OutgoingLink
	for _, targetId := range targetIds {
		links = append(links, OutgoingLink{TargetID: targetId})
	}
	return links, nil
}
