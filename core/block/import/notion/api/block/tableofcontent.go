package block

import (
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type TableOfContentsBlock struct {
	Block
	TableOfContent TableOfContentsObject `json:"table_of_contents"`
}

type TableOfContentsObject struct {
	Color string `json:"color"`
}

func (t *TableOfContentsBlock) GetBlocks(req *MapRequest) *MapResponse {
	id := bson.NewObjectId().Hex()
	var color string
	// Anytype Table Of Content doesn't support different colors of text, only background
	if strings.HasSuffix(t.TableOfContent.Color, api.NotionBackgroundColorSuffix) {
		color = api.NotionColorToAnytype[t.TableOfContent.Color]
	}

	block := &model.Block{
		Id:              id,
		BackgroundColor: color,
		Content: &model.BlockContentOfTableOfContents{
			TableOfContents: &model.BlockContentTableOfContents{},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}
