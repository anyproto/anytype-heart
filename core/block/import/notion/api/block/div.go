package block

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
)

type DividerBlock struct {
	Block
	Divider struct{} `json:"divider"`
}

func (*DividerBlock) GetDivBlock() (*model.Block, string) {
	id := bson.NewObjectId().Hex()
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfDiv{
			Div: &model.BlockContentDiv{
				Style: 0,
			},
		},
	}, id
}
