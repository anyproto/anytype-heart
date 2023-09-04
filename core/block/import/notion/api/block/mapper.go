package block

import (
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type MapResponse struct {
	Blocks   []*model.Block
	BlockIDs []string
}

func (m *MapResponse) Merge(mergedResp *MapResponse) {
	if mergedResp != nil {
		m.BlockIDs = append(m.BlockIDs, mergedResp.BlockIDs...)
		m.Blocks = append(m.Blocks, mergedResp.Blocks...)
	}
}

func MapBlocks(req *api.NotionImportContext, blocks []interface{}, pageID string) *MapResponse {
	resp := &MapResponse{}
	for _, bl := range blocks {
		if ba, ok := bl.(Getter); ok {
			textResp := ba.GetBlocks(req, pageID)
			resp.Merge(textResp)
			continue
		}
	}
	return resp
}
