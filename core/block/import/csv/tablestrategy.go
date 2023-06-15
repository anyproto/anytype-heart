package csv

import (
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	te "github.com/anyproto/anytype-heart/core/block/editor/table"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
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

func (c *TableStrategy) CreateObjects(path string, csvTable [][]string, useFirstRowForHeader bool, progress process.Progress) (string, []*converter.Snapshot, error) {
	st := state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{
			Content: &model.BlockContentOfSmartblock{
				Smartblock: &model.BlockContentSmartblock{},
			},
		}),
	}).NewState()

	if len(csvTable) != 0 {
		err := c.createTable(st, csvTable, useFirstRowForHeader)
		if err != nil {
			return "", nil, err
		}
	}

	details := converter.GetCommonDetails(path, "", "")
	sn := &model.SmartBlockSnapshotBase{
		Blocks:        st.Blocks(),
		Details:       details,
		ObjectTypes:   []string{bundle.TypeKeyPage.URL()},
		Collections:   st.Store(),
		RelationLinks: st.GetRelationLinks(),
	}

	snapshot := &converter.Snapshot{
		Id:       uuid.New().String(),
		SbType:   smartblock.SmartBlockTypePage,
		FileName: path,
		Snapshot: &pb.ChangeSnapshot{Data: sn},
	}
	progress.AddDone(1)
	return snapshot.Id, []*converter.Snapshot{snapshot}, nil
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
	for i, columns := range csvTable {
		rowID, err := c.createRow(st, tableID, i == 0, useFirstRowForHeader)
		if err != nil {
			return err
		}

		err = c.createCells(columns, st, rowID, columnIDs)
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
	for j := range columnIDs {
		textBlock := &model.Block{
			Id: uuid.New().String(),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: ""},
			},
		}
		_, err = c.tableEditor.CellCreate(st, rowID, columnIDs[j], textBlock)
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
	for j, column := range columns {
		textBlock := &model.Block{
			Id: uuid.New().String(),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: column},
			},
		}
		_, err := c.tableEditor.CellCreate(st, rowID, columnIDs[j], textBlock)
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
	for range csvTable[0] {
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
