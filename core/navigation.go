package core

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) NavigationListPages(_ *pb.RpcNavigationListPagesRequest) *pb.RpcNavigationListPagesResponse {
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

	return response(pb.RpcNavigationListPagesResponseError_NULL, pages, nil)
}

func (mw *Middleware) NavigationGetPageInfoWithLinks(req *pb.RpcNavigationGetPageInfoWithLinksRequest) *pb.RpcNavigationGetPageInfoWithLinksResponse {
	response := func(code pb.RpcNavigationGetPageInfoWithLinksResponseErrorCode, page *model.PageInfoWithLinks, err error) *pb.RpcNavigationGetPageInfoWithLinksResponse {
		m := &pb.RpcNavigationGetPageInfoWithLinksResponse{Error: &pb.RpcNavigationGetPageInfoWithLinksResponseError{Code: code}, Page: page}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	page, err := mw.Anytype.PageInfoWithLinks(req.PageId)
	if err != nil {
		return response(pb.RpcNavigationGetPageInfoWithLinksResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcNavigationGetPageInfoWithLinksResponseError_NULL, page, nil)
}
