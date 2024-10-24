package csv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestCsv_GetSnapshotsEmptyFile(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{Path: []string{"testdata/test.csv"}},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 2) // test + root collection
	assert.Contains(t, sn.Snapshots[0].FileName, "test.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 0)
	assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes) // empty collection
	assert.Equal(t, sn.Snapshots[0].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())

	assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
	assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
	assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[1].Snapshot.Data.Collections, template.CollectionStoreKey), 1)
	assert.Nil(t, err)
}

func TestCsv_GetSnapshots(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/Journal.csv"},
				UseFirstRowForRelations: true},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 6) // Journal.csv + root collection + 2 objects in Journal.csv + 2 relations (Created, Tags)
	assert.Contains(t, sn.Snapshots[0].FileName, "Journal.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 2) // 2 objects

	var found bool
	for _, snapshot := range sn.Snapshots {
		if strings.Contains(snapshot.FileName, rootCollectionName) {
			found = true
			assert.NotEmpty(t, snapshot.Snapshot.Data.ObjectTypes)
			assert.Equal(t, snapshot.Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())
			assert.Len(t, pbtypes.GetStringList(snapshot.Snapshot.Data.Collections, template.CollectionStoreKey), 1) // only Journal.csv collection
		}
	}

	assert.True(t, found)
}

func TestCsv_GetSnapshotsTable(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path: []string{"testdata/Journal.csv"},
				Mode: pb.RpcObjectImportRequestCsvParams_TABLE,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 2) // 1 page with table + root collection
	assert.Contains(t, sn.Snapshots[0].FileName, "Journal.csv")

	var found bool
	for _, bl := range sn.Snapshots[0].Snapshot.Data.Blocks {
		if _, ok := bl.Content.(*model.BlockContentOfTable); ok {
			found = true
		}
	}
	assert.True(t, found)
}

func TestCsv_GetSnapshotsTableUseFirstColumnForRelationsOn(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/Journal.csv"},
				Mode:                    pb.RpcObjectImportRequestCsvParams_TABLE,
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 2) // 1 page with table + root collection
	assert.Contains(t, sn.Snapshots[0].FileName, "Journal.csv")

	var rowsID []string
	for _, bl := range sn.Snapshots[0].Snapshot.Data.Blocks {
		if blockContent, ok := bl.Content.(*model.BlockContentOfLayout); ok && blockContent.Layout.Style == model.BlockContentLayout_TableRows {
			rowsID = bl.GetChildrenIds()
		}
	}
	assert.NotNil(t, rowsID)
	for _, bl := range sn.Snapshots[0].Snapshot.Data.Blocks {
		if blockContent, ok := bl.Content.(*model.BlockContentOfTableRow); ok && bl.Id == rowsID[0] {
			assert.True(t, blockContent.TableRow.IsHeader)
		}
	}

	for _, bl := range sn.Snapshots[0].Snapshot.Data.Blocks {
		if strings.Contains(bl.Id, rowsID[0]) && bl.GetText() != nil {
			assert.True(t, bl.GetText().Text == "Name" || bl.GetText().Text == "Created" || bl.GetText().Text == "Tags")
		}
	}
}

func TestCsv_GetSnapshotsSemiColon(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{Path: []string{"testdata/semicolon.csv"}, Delimiter: ";", UseFirstRowForRelations: true},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 16) // 8 objects + root collection + semicolon collection + 5 relations
	assert.Contains(t, sn.Snapshots[0].FileName, "semicolon.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 8)
	assert.Equal(t, sn.Snapshots[0].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())
}

func TestCsv_GetSnapshotsTranspose(t *testing.T) {
	t.Run("number of columns equal", func(t *testing.T) {
		csv := CSV{}
		p := process.NewProgress(pb.ModelProcess_Import)
		sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
				CsvParams: &pb.RpcObjectImportRequestCsvParams{
					Path:                    []string{"testdata/transpose.csv"},
					Delimiter:               ";",
					TransposeRowsAndColumns: true,
					UseFirstRowForRelations: true,
				},
			},
			Type: model.Import_Csv,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		assert.Nil(t, err)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 4) // 2 object + root collection + transpose collection + 1 relations

		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.SbType == sb.SmartBlockTypeRelation {
				name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
				assert.True(t, name == "price")
			}
		}

		var collection *common.Snapshot
		for _, snapshot := range sn.Snapshots {
			// only objects created from rows
			if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
				lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) &&
				snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName) == "transpose Transpose" {
				collection = snapshot
			}
		}

		assert.NotNil(t, collection)
	})

	t.Run("number of columns is not equal", func(t *testing.T) {
		// given
		csv := CSV{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
				CsvParams: &pb.RpcObjectImportRequestCsvParams{
					Path:                    []string{"testdata/transpose_not_matrix.csv"},
					Delimiter:               ";",
					TransposeRowsAndColumns: true,
					UseFirstRowForRelations: true,
				},
			},
			Type: model.Import_Csv,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 4)

		for _, snapshot := range sn.Snapshots {
			if snapshot.SbType == sb.SmartBlockTypeRelation {
				name := pbtypes.GetString(snapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyName.String())
				assert.True(t, name == "price123")
			}
		}
	})
}

func TestCsv_GetSnapshotsTransposeUseFirstRowForRelationsOff(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/transpose.csv"},
				Delimiter:               ";",
				TransposeRowsAndColumns: true,
				UseFirstRowForRelations: false,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 5) // 2 object + root collection + transpose collection + 1 relations

	for _, snapshot := range sn.Snapshots {
		if snapshot.Snapshot.SbType == sb.SmartBlockTypeRelation {
			name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
			assert.True(t, name == "Field 1" || name == "Field 2")
		}
	}
}

func TestCsv_GetSnapshotsUseFirstColumnForRelationsOn(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/Journal.csv"},
				Delimiter:               ",",
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 6) // Journal.csv collection, root collection + 2 objects in Journal.csv + 2 relations (Created, Tags)

	var rowsObjects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
			rowsObjects = append(rowsObjects, snapshot)
		}
	}

	assert.Len(t, rowsObjects, 2)

	want := [][]string{
		{"Hawaii Vacation", "July 13, 2022 8:54 AM", "Special Event"},
		{"Just another day", "July 13, 2022 8:54 AM", "Daily"},
	}
	assertSnapshotsHaveDetails(t, want[0], rowsObjects[0])
	assertSnapshotsHaveDetails(t, want[1], rowsObjects[1])
}

func assertSnapshotsHaveDetails(t *testing.T, want []string, objects *common.Snapshot) {
	objects.Snapshot.Data.Details.Iterate(func(key domain.RelationKey, value domain.Value) bool {
		if key == bundle.RelationKeySourceFilePath || key == bundle.RelationKeyLayout {
			return true
		}
		assert.Contains(t, want, value.String())
		return true
	})
}

func TestCsv_GetSnapshotsUseFirstColumnForRelationsOff(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:      []string{"testdata/Journal.csv"},
				Delimiter: ",",
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 7) // Journal.csv collection, root collection + 3 objects in Journal.csv + 2 relations (Created, Tags)

	var objects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
			objects = append(objects, snapshot)
		}
	}

	assert.Len(t, objects, 3) // first row is also an object

	want := [][]string{
		{"Name", "Created", "Tags"},
		{"Hawaii Vacation", "July 13, 2022 8:54 AM", "Special Event"},
		{"Just another day", "July 13, 2022 8:54 AM", "Daily"},
	}
	assertSnapshotsHaveDetails(t, want[0], objects[0])
	assertSnapshotsHaveDetails(t, want[1], objects[1])
	assertSnapshotsHaveDetails(t, want[2], objects[2])

	var subObjects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType == sb.SmartBlockTypeRelation {
			subObjects = append(subObjects, snapshot)
		}
	}

	assert.Len(t, subObjects, 2)

	name := subObjects[0].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 1")

	name = subObjects[1].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 2")
}

func TestCsv_GetSnapshotsQuotedStrings(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/quotedstrings.csv"},
				Delimiter:               ",",
				TransposeRowsAndColumns: true,
				UseFirstRowForRelations: true,
				Mode:                    pb.RpcObjectImportRequestCsvParams_TABLE,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
}

func TestCsv_GetSnapshotsBigFile(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/bigfile.csv", "testdata/transpose.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, err)
	assert.True(t, errors.Is(err.GetResultError(model.Import_Csv), common.ErrLimitExceeded))
	assert.Nil(t, sn)
}

func TestCsv_GetSnapshotsEmptyFirstLineUseFirstColumnForRelationsOn(t *testing.T) {
	ctx := context.Background()
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(ctx, &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/emptyfirstline.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	var subObjects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		if snapshot.Snapshot.SbType == sb.SmartBlockTypeRelation {
			subObjects = append(subObjects, snapshot)
		}
	}
	assert.Len(t, subObjects, 6)
}

func TestCsv_GetSnapshotsEmptyFirstLineUseFirstColumnForRelationsOff(t *testing.T) {
	ctx := context.Background()
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(ctx, &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/emptyfirstline.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: false,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	var subObjects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		if snapshot.Snapshot.SbType == sb.SmartBlockTypeRelation {
			subObjects = append(subObjects, snapshot)
		}
	}
	assert.Len(t, subObjects, 6)

	name := subObjects[0].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 1")

	name = subObjects[1].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 2")

	name = subObjects[2].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 3")

	name = subObjects[3].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 4")

	name = subObjects[4].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 5")

	name = subObjects[5].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Field 6")
}

func TestCsv_GetSnapshots1000RowsFile(t *testing.T) {
	ctx := context.Background()
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	// UseFirstRowForRelations is off
	sn, _ := csv.GetSnapshots(ctx, &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/1000_rows.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: false,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, sn)

	var objects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
			objects = append(objects, snapshot)
		}
	}

	assert.Len(t, objects, limitForRows)

	// UseFirstRowForRelations is on
	sn, _ = csv.GetSnapshots(ctx, &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/1000_rows.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, sn)

	objects = []*common.Snapshot{}
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
			objects = append(objects, snapshot)
		}
	}

	assert.Len(t, objects, limitForRows-1)
}

func Test_findUniqueRelationAndAddNumber(t *testing.T) {
	t.Run("All relations are unique", func(t *testing.T) {
		relations := []string{"relation", "relation1", "relation2", "relation3"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation", "relation1", "relation2", "relation3"}, result)
	})

	t.Run("1 relation name is not unique", func(t *testing.T) {
		relations := []string{"relation", "relation1", "relation2", "relation3", "relation"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation", "relation1", "relation2", "relation3", "relation 1"}, result)
	})

	t.Run("1 relation is not unique after first iteration", func(t *testing.T) {
		relations := []string{"relation", "relation1", "relation2", "relation3", "relation", "relation 1"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation", "relation1", "relation2", "relation3", "relation 2", "relation 1"}, result)
	})

	t.Run("1 relation name is not unique after first iteration: other order", func(t *testing.T) {
		relations := []string{"relation", "relation1", "relation2", "relation3", "relation 1", "relation"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation", "relation1", "relation2", "relation3", "relation 1", "relation 2"}, result)
	})

	t.Run("1 relation name is not unique after second iteration", func(t *testing.T) {
		relations := []string{"relation", "relation1", "relation2", "relation3", "relation 1", "relation 2", "relation"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation", "relation1", "relation2", "relation3", "relation 1", "relation 2", "relation 3"}, result)
	})

	t.Run("2 relation names are not unique after first iteration", func(t *testing.T) {
		relations := []string{"relation", "relation1", "relation2", "relation3", "relation 1", "relation", "relation", "relation 2"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation", "relation1", "relation2", "relation3", "relation 1", "relation 3", "relation 4", "relation 2"}, result)
	})

	t.Run("2 relation names are not unique", func(t *testing.T) {
		relations := []string{"relation1", "relation2", "relation3", "relation", "relation", "relation"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation1", "relation2", "relation3", "relation", "relation 1", "relation 2"}, result)
	})

	t.Run("empty columns", func(t *testing.T) {
		relations := []string{"relation1", "", "", "relation", "", "relation"}
		result := findUniqueRelationAndAddNumber(relations)
		assert.Equal(t, []string{"relation1", "1", "2", "relation", "3", "relation 1"}, result)
	})
}

func Test_findUniqueRelationWithSpaces(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/relationswithspaces.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	var subObjects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		if snapshot.Snapshot.SbType == sb.SmartBlockTypeRelation {
			subObjects = append(subObjects, snapshot)
		}
	}
	assert.Len(t, subObjects, 5)

	name := subObjects[0].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Text")

	name = subObjects[1].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Text 1")

	name = subObjects[2].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Text 3")

	name = subObjects[3].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Text 2")

	name = subObjects[4].Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	assert.True(t, name == "Text 4")
}

func TestCsv_GetSnapshots10Relations(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	// UseFirstRowForRelations is off
	sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/10_relations.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: false,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	var objects []*common.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
			objects = append(objects, snapshot)
		}
	}

	for _, object := range objects {
		keys := object.Snapshot.Data.Details.Keys()
		numberOfCSVRelations := getRelationsNumber(keys)
		assert.Equal(t, numberOfCSVRelations, limitForColumns)
	}

	// UseFirstRowForRelations is on
	sn, err = csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/10_relations.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: model.Import_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	objects = []*common.Snapshot{}
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
			objects = append(objects, snapshot)
		}
	}

	for _, object := range objects {
		numberOfCSVRelations := getRelationsNumber(object.Snapshot.Data.Details.Keys())
		assert.Equal(t, numberOfCSVRelations, limitForColumns)
	}
}

func TestCsv_GetSnapshotsTableModeDifferentColumnsNumber(t *testing.T) {
	t.Run("test different columns number in file - table mode", func(t *testing.T) {
		// given
		csv := CSV{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
				CsvParams: &pb.RpcObjectImportRequestCsvParams{
					Path:                    []string{"testdata/differentcolumnnumber.csv"},
					Delimiter:               ",",
					UseFirstRowForRelations: true,
					Mode:                    pb.RpcObjectImportRequestCsvParams_TABLE,
				},
			},
			Type: model.Import_Csv,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)

		var objects []*common.Snapshot
		for _, snapshot := range sn.Snapshots {
			// only objects created from rows
			if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
				!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
				objects = append(objects, snapshot)
			}
		}
		assert.Len(t, objects, 1)
		assert.Equal(t, objects[0].Snapshot.Data.Details.GetString(bundle.RelationKeyName), "differentcolumnnumber")
		numberOfCSVColumns := lo.CountBy(objects[0].Snapshot.Data.Blocks, func(item *model.Block) bool { return item.GetTableColumn() != nil })
		assert.Equal(t, 5, numberOfCSVColumns)
		numberOfCSVRows := lo.CountBy(objects[0].Snapshot.Data.Blocks, func(item *model.Block) bool { return item.GetTableRow() != nil })
		assert.Equal(t, 3, numberOfCSVRows)
	})
	t.Run("test different columns number in file - collection mode", func(t *testing.T) {
		// given
		csv := CSV{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := csv.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
				CsvParams: &pb.RpcObjectImportRequestCsvParams{
					Path:                    []string{"testdata/differentcolumnnumber.csv"},
					Delimiter:               ",",
					UseFirstRowForRelations: true,
					Mode:                    pb.RpcObjectImportRequestCsvParams_COLLECTION,
				},
			},
			Type: model.Import_Csv,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)

		var objects []*common.Snapshot
		for _, snapshot := range sn.Snapshots {
			// only objects created from rows
			if snapshot.Snapshot.SbType != sb.SmartBlockTypeRelation &&
				!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.String()) {
				objects = append(objects, snapshot)
			}
		}

		assert.Len(t, objects, 2)
		for _, object := range objects {
			numberOfCSVRelations := getRelationsNumber(object.Snapshot.Data.Details.Keys())
			assert.Equal(t, 5, numberOfCSVRelations)
		}
	})
}

func getRelationsNumber(keys []domain.RelationKey) int {
	return lo.CountBy(keys, func(item domain.RelationKey) bool {
		return item != bundle.RelationKeySourceFilePath && item != bundle.RelationKeyLayout
	})
}
