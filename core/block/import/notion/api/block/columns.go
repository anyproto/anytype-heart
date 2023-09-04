package block

import (
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ColumnListBlock struct {
	Block
	ColumnList interface{} `json:"column_list"`
}

func (c *ColumnListBlock) GetID() string {
	return c.ID
}

func (c *ColumnListBlock) HasChild() bool {
	return c.HasChildren
}

func (c *ColumnListBlock) SetChildren(children []interface{}) {
	c.ColumnList = children
}

func (c *ColumnListBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	columnsList := c.ColumnList.([]interface{})
	var (
		resultResponse = &MapResponse{}
		rowBlock       simple.Block
	)
	if len(columnsList) != 0 {
		rowBlock = c.handleFirstColumn(req, columnsList[0], resultResponse, pageID)
	}
	for i := 1; i < len(columnsList); i++ {
		c.handleColumn(req, columnsList[i], resultResponse, rowBlock, pageID)
	}
	c.addRowBlock(resultResponse, rowBlock)
	return resultResponse
}

func (c *ColumnListBlock) handleColumn(req *api.NotionImportContext, notionColumn interface{}, resultResponse *MapResponse, rowBlock simple.Block, pageID string) {
	column := c.addColumnBlocks("ct-", req, notionColumn, resultResponse, pageID)
	rowBlock.Model().ChildrenIds = append(rowBlock.Model().ChildrenIds, column.Model().Id)
}

func (c *ColumnListBlock) handleFirstColumn(req *api.NotionImportContext, notionColumn interface{}, resultResponse *MapResponse, pageID string) simple.Block {
	column := c.addColumnBlocks("cd-", req, notionColumn, resultResponse, pageID)
	rowBlock := c.getRowBlock(strings.TrimPrefix(column.Model().Id, "cd-"), column.Model().Id)
	return rowBlock
}

func (c *ColumnListBlock) addColumnBlocks(prefix string, req *api.NotionImportContext, notionColumn interface{}, resultResponse *MapResponse, pageID string) simple.Block {
	resp := MapBlocks(req, []interface{}{notionColumn}, pageID)
	childBlocks := c.getChildBlocksForColumn(resp)
	id := bson.NewObjectId().Hex()
	column := c.getColumnBlock(id, prefix, childBlocks, resultResponse)
	resultResponse.Blocks = append(resultResponse.Blocks, resp.Blocks...)
	resultResponse.BlockIDs = append(resultResponse.BlockIDs, resp.BlockIDs...)
	return column
}

func (c *ColumnListBlock) getRowBlock(id string, columnID string) simple.Block {
	rowBlock := simple.New(&model.Block{
		Id:          "r-" + id,
		ChildrenIds: []string{columnID},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		},
	})
	return rowBlock
}

func (c *ColumnListBlock) getColumnBlock(id, prefix string, childBlocks []string, resultResponse *MapResponse) simple.Block {
	column := simple.New(&model.Block{
		Id:          prefix + id,
		ChildrenIds: childBlocks,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	resultResponse.Blocks = append(resultResponse.Blocks, column.Model())
	resultResponse.BlockIDs = append(resultResponse.BlockIDs, column.Model().Id)
	return column
}

func (c *ColumnListBlock) getChildBlocksForColumn(resp *MapResponse) []string {
	childBlocks := make([]string, 0)
	for _, b := range resp.Blocks {
		if len(b.ChildrenIds) != 0 {
			childBlocks = append(childBlocks, b.ChildrenIds...)
		}
	}
	rootChild := make([]string, 0)
	for _, b := range resp.Blocks {
		if !lo.Contains(childBlocks, b.Id) {
			rootChild = append(rootChild, b.Id)
		}
	}
	return rootChild
}

func (c *ColumnListBlock) addRowBlock(resultResponse *MapResponse, rowBlock simple.Block) {
	if rowBlock == nil || rowBlock.Model() == nil {
		return
	}
	resultResponse.Blocks = append(resultResponse.Blocks, rowBlock.Model())
	resultResponse.BlockIDs = append(resultResponse.BlockIDs, rowBlock.Model().Id)
}

type ColumnBlock struct {
	Block
	Column *ColumnObject `json:"column"`
}

type ColumnObject struct {
	Children []interface{} `json:"children"`
}

func (c *ColumnBlock) GetBlocks(req *api.NotionImportContext, pageID string) *MapResponse {
	resp := MapBlocks(req, c.Column.Children, pageID)
	return resp
}

func (c *ColumnBlock) GetID() string {
	return c.ID
}

func (c *ColumnBlock) HasChild() bool {
	return c.HasChildren
}

func (c *ColumnBlock) SetChildren(children []interface{}) {
	c.Column.Children = children
}
