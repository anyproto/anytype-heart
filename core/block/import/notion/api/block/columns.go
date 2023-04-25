package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
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

func (c *ColumnListBlock) GetBlocks(req *MapRequest) *MapResponse {
	columnsList := c.ColumnList.([]interface{})
	var (
		resultResponse = &MapResponse{}
		rowBlock       simple.Block
	)
	if len(columnsList) != 0 {
		req.Blocks = []interface{}{columnsList[0]}
		resp := MapBlocks(req)
		childBlocks := make([]string, 0)
		for _, b := range resp.Blocks {
			if len(b.ChildrenIds) != 0 {
				childBlocks = append(childBlocks, b.ChildrenIds...)
			}
		}
		notChildBlocks := make([]string, 0)
		for _, b := range resp.Blocks {
			var found bool
			for _, blockID := range childBlocks {
				if b.Id == blockID {
					found = true
					break
				}
			}
			if !found {
				notChildBlocks = append(notChildBlocks, b.Id)
			}
		}
		id := bson.NewObjectId().Hex()
		column := simple.New(&model.Block{
			Id:          "ct-" + id,
			ChildrenIds: notChildBlocks,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_Column,
				},
			},
		})
		resultResponse.Blocks = append(resultResponse.Blocks, column.Model())
		resultResponse.BlockIDs = append(resultResponse.BlockIDs, column.Model().Id)
		rowBlock = simple.New(&model.Block{
			Id:          "r-" + id,
			ChildrenIds: []string{column.Model().Id},
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_Row,
				},
			},
		})
		resultResponse.Blocks = append(resultResponse.Blocks, resp.Blocks...)
		resultResponse.BlockIDs = append(resultResponse.BlockIDs, resp.BlockIDs...)
	}
	for i := 1; i < len(columnsList); i++ {
		req.Blocks = []interface{}{columnsList[i]}
		resp := MapBlocks(req)
		childBlocks := make([]string, 0)
		for _, b := range resp.Blocks {
			if len(b.ChildrenIds) != 0 {
				childBlocks = append(childBlocks, b.ChildrenIds...)
			}
		}
		notChildBlocks := make([]string, 0)
		for _, b := range resp.Blocks {
			var found bool
			for _, blockID := range childBlocks {
				if b.Id == blockID {
					found = true
					break
				}
			}
			if !found {
				notChildBlocks = append(notChildBlocks, b.Id)
			}
		}
		id := bson.NewObjectId().Hex()
		column := simple.New(&model.Block{
			Id:          "cd-" + id,
			ChildrenIds: notChildBlocks,
			Content: &model.BlockContentOfLayout{
				Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_Column,
				},
			},
		})
		resultResponse.Blocks = append(resultResponse.Blocks, column.Model())
		resultResponse.BlockIDs = append(resultResponse.BlockIDs, column.Model().Id)
		resultResponse.Blocks = append(resultResponse.Blocks, resp.Blocks...)
		resultResponse.BlockIDs = append(resultResponse.BlockIDs, resp.BlockIDs...)
		rowBlock.Model().ChildrenIds = append(rowBlock.Model().ChildrenIds, column.Model().Id)
	}
	resultResponse.Blocks = append(resultResponse.Blocks, rowBlock.Model())
	resultResponse.BlockIDs = append(resultResponse.BlockIDs, rowBlock.Model().Id)
	return resultResponse
}

type ColumnBlock struct {
	Block
	Column *ColumnObject `json:"column"`
}

type ColumnObject struct {
	Children []interface{} `json:"children"`
}

func (c *ColumnBlock) GetBlocks(req *MapRequest) *MapResponse {
	req.Blocks = c.Column.Children
	resp := MapBlocks(req)
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
