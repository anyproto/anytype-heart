package csv

import (
	"github.com/globalsign/mgo/bson"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	te "github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type TableStrategy struct {
	tableEditor te.TableEditor
}

func NewTableStrategy(tableEditor te.TableEditor) *TableStrategy {
	return &TableStrategy{tableEditor: tableEditor}
}

func (c *TableStrategy) CreateObjects(path string, csvTable [][]string, params *pb.RpcObjectImportRequestCsvParams, progress process.Progress) (string, []*common.Snapshot, error) {
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Content: &model.BlockContentOfSmartblock{
				Smartblock: &model.BlockContentSmartblock{},
			},
		}),
	}).NewState()

	if len(csvTable) != 0 {
		err := c.createTable(st, csvTable, params.UseFirstRowForRelations)
		if err != nil {
			return "", nil, err
		}
	}

	details := common.GetCommonDetails(path, "", "", model.ObjectType_basic)
	sn := &common.StateSnapshot{
		Blocks:      st.Blocks(),
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyPage.String()},
		Collections: st.Store(),
	}

	snapshot := &common.Snapshot{
		Id:       uuid.New().String(),
		FileName: path,
		Snapshot: &common.SnapshotModel{
			SbType: smartblock.SmartBlockTypePage,
			Data:   sn,
		},
	}
	progress.AddDone(1)
	return snapshot.Id, []*common.Snapshot{snapshot}, nil
}

func (c *TableStrategy) createTable(st *state.State, csvTable [][]string, useFirstRowForHeader bool) error {
	tableID, err := c.tableEditor.TableCreate(st, pb.RpcBlockTableCreateRequest{})
	if err != nil {
		return err
	}

	columnIDs, err := c.createColumns(csvTable, st, tableID)
	if err != nil {
		return err
	}
	if !useFirstRowForHeader {
		err = c.createEmptyHeader(st, tableID, columnIDs)
		if err != nil {
			return err
		}
	}
	for i := 0; i < len(csvTable); i++ {
		rowID, err := c.createRow(st, tableID, i == 0, useFirstRowForHeader)
		if err != nil {
			return err
		}

		err = c.createCells(csvTable[i], st, rowID, columnIDs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *TableStrategy) createEmptyHeader(st *state.State, tableID string, columnIDs []string) error {
	rowID, err := c.tableEditor.RowCreate(st, pb.RpcBlockTableRowCreateRequest{
		Position: model.Block_Inner,
		TargetId: tableID,
	})
	if err != nil {
		return err
	}
	for _, colID := range columnIDs {
		textBlock := &model.Block{
			Id: bson.NewObjectId().Hex(),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: ""},
			},
		}
		_, err = c.tableEditor.CellCreate(st, rowID, colID, textBlock)
		if err != nil {
			return err
		}
	}
	err = c.tableEditor.RowSetHeader(st, pb.RpcBlockTableRowSetHeaderRequest{
		IsHeader: true,
		TargetId: rowID,
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *TableStrategy) createCells(columns []string, st *state.State, rowID string, columnIDs []string) error {
	for i := 0; i < len(columns); i++ {
		if i >= len(columnIDs) {
			continue
		}
		textBlock := &model.Block{
			Id: bson.NewObjectId().Hex(),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: columns[i]},
			},
		}
		_, err := c.tableEditor.CellCreate(st, rowID, columnIDs[i], textBlock)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *TableStrategy) createRow(st *state.State, tableID string, isFirstRow bool, useFirstRowForHeader bool) (string, error) {
	rowID, err := c.tableEditor.RowCreate(st, pb.RpcBlockTableRowCreateRequest{
		Position: model.Block_Inner,
		TargetId: tableID,
	})
	if err != nil {
		return "", err
	}

	if isFirstRow && useFirstRowForHeader {
		err = c.tableEditor.RowSetHeader(st, pb.RpcBlockTableRowSetHeaderRequest{
			IsHeader: true,
			TargetId: rowID,
		})
		if err != nil {
			return "", err
		}
	}
	return rowID, nil
}

func (c *TableStrategy) createColumns(csvTable [][]string, st *state.State, tableID string) ([]string, error) {
	columnIDs := make([]string, 0, len(csvTable[0]))
	for i := 0; i < len(csvTable[0]); i++ {
		colID, err := c.tableEditor.ColumnCreate(st, pb.RpcBlockTableColumnCreateRequest{
			Position: model.Block_Inner,
			TargetId: tableID,
		})
		if err != nil {
			return nil, err
		}
		columnIDs = append(columnIDs, colID)
	}
	return columnIDs, nil
}
