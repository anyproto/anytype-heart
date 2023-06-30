package block

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type DividerBlock struct {
	Block
	Divider struct{} `json:"divider"`
}

func (*DividerBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
	id := bson.NewObjectId().Hex()
	block := &model.Block{
		Id: id,
		Content: &model.BlockContentOfDiv{
			Div: &model.BlockContentDiv{
				Style: 0,
			},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}
