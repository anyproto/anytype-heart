package block

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// MapRequest is a data object with all necessary structures for blocks
type MapRequest struct {
	Blocks []interface{}
	// Need all these maps for correct mapping of pages and databases from notion to anytype
	// for such blocks as mentions or links to objects
	NotionPageIdsToAnytype     map[string]string
	NotionDatabaseIdsToAnytype map[string]string
	PageNameToID               map[string]string
	DatabaseNameToID           map[string]string
}

type MapResponse struct {
	Blocks    []*model.Block
	Relations []*converter.Relation
	Details   map[string]*types.Value
	BlockIDs  []string
}

func (m *MapResponse) Merge(mergedResp *MapResponse) {
	if mergedResp != nil {
		m.BlockIDs = append(m.BlockIDs, mergedResp.BlockIDs...)
		m.Relations = append(m.Relations, mergedResp.Relations...)
		m.Blocks = append(m.Blocks, mergedResp.Blocks...)
		m.MergeDetails(mergedResp.Details)
	}
}

func (m *MapResponse) MergeDetails(mergeDetails map[string]*types.Value) {
	if m.Details == nil {
		m.Details = make(map[string]*types.Value, 0)
	}
	for k, v := range mergeDetails {
		m.Details[k] = v
	}
}

func MapBlocks(req *MapRequest) *MapResponse {
	resp := &MapResponse{}
	for _, bl := range req.Blocks {
		if ba, ok := bl.(Getter); ok {
			textResp := ba.GetBlocks(req)
			resp.Merge(textResp)
			continue
		}
	}
	return resp
}
