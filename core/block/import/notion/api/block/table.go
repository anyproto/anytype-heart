package block

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// columnLayoutBlock + tableRowLayoutBlock + table
const numberOfDefaultTableBlocks = 3

type TableBlock struct {
	Block
	Table TableObject `json:"table"`
}

type TableObject struct {
	Width           int64            `json:"table_width"`
	HasColumnHeader bool             `json:"has_column_header"`
	HasRowHeader    bool             `json:"has_row_header"`
	Children        []*TableRowBlock `json:"children"`
}

type TableRowBlock struct {
	Block
	TableRowObject TableRowObject `json:"table_row"`
}

type TableRowObject struct {
	Cells [][]api.RichText `json:"cells"`
}

func (t *TableBlock) GetID() string {
	return t.ID
}

func (t *TableBlock) HasChild() bool {
	return t.HasChildren
}

func (t *TableBlock) SetChildren(children []interface{}) {
	t.Table.Children = make([]*TableRowBlock, 0, len(children))
	for _, ch := range children {
		t.Table.Children = append(t.Table.Children, ch.(*TableRowBlock))
	}
}

func (t *TableBlock) GetBlocks(req *MapRequest) *MapResponse {
	columnsBlocks, columnsBlocksIDs, columnLayoutBlockID, columnLayoutBlock := t.getColumns()

	tableResponse := &MapResponse{}
	tableRowBlocks, tableRowBlocksIDs, rowTextBlocks := t.getRows(req, columnsBlocksIDs, tableResponse)

	tableRowBlockID, tableRowLayoutBlock := t.getLayoutRowBlock(tableRowBlocksIDs)

	rootID, table := t.getTableBlock(columnLayoutBlockID, tableRowBlockID)

	resultNumberOfBlocks := len(columnsBlocks) + len(rowTextBlocks) + len(tableRowBlocks) + numberOfDefaultTableBlocks
	allBlocks := make([]*model.Block, 0, resultNumberOfBlocks)
	allBlocks = append(allBlocks, table)
	allBlocks = append(allBlocks, columnLayoutBlock)
	allBlocks = append(allBlocks, columnsBlocks...)
	allBlocks = append(allBlocks, tableRowLayoutBlock)
	allBlocks = append(allBlocks, tableRowBlocks...)
	allBlocks = append(allBlocks, rowTextBlocks...)

	allBlocksIDs := make([]string, 0, resultNumberOfBlocks)
	allBlocksIDs = append(allBlocksIDs, rootID)
	allBlocksIDs = append(allBlocksIDs, columnLayoutBlockID)
	allBlocksIDs = append(allBlocksIDs, columnsBlocksIDs...)
	allBlocksIDs = append(allBlocksIDs, tableRowBlockID)
	for _, b := range rowTextBlocks {
		allBlocksIDs = append(allBlocksIDs, b.Id)
	}

	tableResponse.BlockIDs = allBlocksIDs
	tableResponse.Blocks = allBlocks
	return tableResponse
}

func (*TableBlock) getTableBlock(columnLayoutBlockID string, tableRowBlockID string) (string, *model.Block) {
	rootID := bson.NewObjectId().Hex()
	table := &model.Block{
		Id:          rootID,
		ChildrenIds: []string{columnLayoutBlockID, tableRowBlockID},
		Content: &model.BlockContentOfTable{
			Table: &model.BlockContentTable{},
		},
	}
	return rootID, table
}

func (*TableBlock) getLayoutRowBlock(children []string) (string, *model.Block) {
	tableRowBlockID := bson.NewObjectId().Hex()
	tableRowLayoutBlock := &model.Block{
		Id:          tableRowBlockID,
		ChildrenIds: children,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows,
			},
		},
	}
	return tableRowBlockID, tableRowLayoutBlock
}

func (t *TableBlock) getRows(req *MapRequest,
	columnsBlocksIDs []string,
	tableResponse *MapResponse) ([]*model.Block, []string, []*model.Block) {
	var (
		needHeader        = true
		tableRowBlocks    = make([]*model.Block, 0, len(t.Table.Children))
		tableRowBlocksIDs = make([]string, 0, len(t.Table.Children))
		rowTextBlocks     = make([]*model.Block, 0)
	)
	for _, trb := range t.Table.Children {
		var childBlockIDsCurrRow []string
		id := bson.NewObjectId().Hex()
		for i, c := range trb.TableRowObject.Cells {
			to := &TextObject{
				RichText: c,
			}
			resp := to.GetTextBlocks(model.BlockContentText_Paragraph, nil, req)

			resp.BlockIDs = make([]string, 0, len(c))
			for _, b := range resp.Blocks {
				b.Id = table.MakeCellID(id, columnsBlocksIDs[i])
				resp.BlockIDs = append(resp.BlockIDs, b.Id)
			}
			rowTextBlocks = append(rowTextBlocks, resp.Blocks...)
			childBlockIDsCurrRow = append(childBlockIDsCurrRow, resp.BlockIDs...)
		}

		var isHeader bool
		if needHeader {
			isHeader = t.Table.HasRowHeader
			needHeader = false
		}

		tableRowBlocks = append(tableRowBlocks, &model.Block{
			Id:          id,
			ChildrenIds: childBlockIDsCurrRow,
			Content: &model.BlockContentOfTableRow{
				TableRow: &model.BlockContentTableRow{
					IsHeader: isHeader,
				},
			},
		})
		tableRowBlocksIDs = append(tableRowBlocksIDs, id)
	}

	return tableRowBlocks, tableRowBlocksIDs, rowTextBlocks
}

func (t *TableBlock) getColumns() ([]*model.Block, []string, string, *model.Block) {
	columnsBlocks := make([]*model.Block, 0, t.Table.Width)
	columnsBlocksIDs := make([]string, 0, t.Table.Width)
	for i := 0; i < int(t.Table.Width); i++ {
		id := bson.NewObjectId().Hex()
		columnsBlocks = append(columnsBlocks, &model.Block{
			Id:          id,
			ChildrenIds: []string{},
			Content: &model.BlockContentOfTableColumn{
				TableColumn: &model.BlockContentTableColumn{},
			},
		})
		columnsBlocksIDs = append(columnsBlocksIDs, id)
	}

	columnLayoutBlockID := bson.NewObjectId().Hex()
	columnLayoutBlock := &model.Block{
		Id:          columnLayoutBlockID,
		ChildrenIds: columnsBlocksIDs,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableColumns,
			},
		},
	}
	return columnsBlocks, columnsBlocksIDs, columnLayoutBlockID, columnLayoutBlock
}
