package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) NavigationListPages(req *pb.RpcNavigationListPagesRequest) *pb.RpcNavigationListPagesResponse {
	response := func(code pb.RpcNavigationListPagesResponseErrorCode, pages []*model.PageInfo, err error) *pb.RpcNavigationListPagesResponse {
		m := &pb.RpcNavigationListPagesResponse{Error: &pb.RpcNavigationListPagesResponseError{Code: code}, Pages: pages}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	pages, err := mw.Anytype.PageList()
	if err != nil {
		return response(pb.RpcNavigationListPagesResponseError_UNKNOWN_ERROR, nil, err)
	}

	var pagesFiltered []*model.PageInfo
	for _, page := range pages {
		if req.Context != pb.RpcNavigation_Navigation && (page.PageType == model.PageInfo_Set || page.PageType == model.PageInfo_Archive) {
			continue
		}

		pagesFiltered = append(pagesFiltered, page)
	}

	return response(pb.RpcNavigationListPagesResponseError_NULL, pagesFiltered, nil)
}

func (mw *Middleware) NavigationGetPageInfoWithLinks(req *pb.RpcNavigationGetPageInfoWithLinksRequest) *pb.RpcNavigationGetPageInfoWithLinksResponse {
	response := func(code pb.RpcNavigationGetPageInfoWithLinksResponseErrorCode, page *model.PageInfoWithLinks, err error) *pb.RpcNavigationGetPageInfoWithLinksResponse {
		m := &pb.RpcNavigationGetPageInfoWithLinksResponse{Error: &pb.RpcNavigationGetPageInfoWithLinksResponseError{Code: code}, Page: page}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	filter := func(pages []*model.PageInfo) []*model.PageInfo {
		var filtered []*model.PageInfo
		for _, page := range pages {
			if page.PageType == model.PageInfo_Set || page.PageType == model.PageInfo_Archive {
				continue
			}

			filtered = append(filtered, page)
		}
		return filtered
	}

	page, err := mw.Anytype.PageInfoWithLinks(req.PageId)
	if err != nil {
		return response(pb.RpcNavigationGetPageInfoWithLinksResponseError_UNKNOWN_ERROR, nil, err)
	}

	if req.Context != pb.RpcNavigation_Navigation && page.Links != nil {
		page.Links.Inbound = filter(page.Links.Inbound)
		page.Links.Outbound = filter(page.Links.Outbound)
	}

	return response(pb.RpcNavigationGetPageInfoWithLinksResponseError_NULL, page, nil)
}
