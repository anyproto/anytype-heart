package csv

import (
	"errors"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestCsv_GetSnapshotsEmptyFile(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{Path: []string{"testdata/test.csv"}},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 2) // test + root collection
	assert.Contains(t, sn.Snapshots[0].FileName, "test.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 0)
	assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes) // empty collection
	assert.Equal(t, sn.Snapshots[0].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())

	assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
	assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
	assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[1].Snapshot.Data.Collections, template.CollectionStoreKey), 1)
	assert.Nil(t, err)
}

func TestCsv_GetSnapshots(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/Journal.csv"},
				UseFirstRowForRelations: true},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 6) // Journal.csv + root collection + 2 objects in Journal.csv + 2 relations (Created, Tags)
	assert.Contains(t, sn.Snapshots[0].FileName, "Journal.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 2) // 2 objects

	var found bool
	for _, snapshot := range sn.Snapshots {
		if snapshot.FileName == rootCollectionName {
			found = true
			assert.NotEmpty(t, snapshot.Snapshot.Data.ObjectTypes)
			assert.Equal(t, snapshot.Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())
			assert.Len(t, pbtypes.GetStringList(snapshot.Snapshot.Data.Collections, template.CollectionStoreKey), 1) // only Journal.csv collection
		}
	}

	assert.True(t, found)
}

func TestCsv_GetSnapshotsTable(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path: []string{"testdata/Journal.csv"},
				Mode: pb.RpcObjectImportRequestCsvParams_TABLE,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
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
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/Journal.csv"},
				Mode:                    pb.RpcObjectImportRequestCsvParams_TABLE,
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
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
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{Path: []string{"testdata/semicolon.csv"}, Delimiter: ";", UseFirstRowForRelations: true},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 12) // 8 objects + root collection + semicolon collection + 2 relations
	assert.Contains(t, sn.Snapshots[0].FileName, "semicolon.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 8)
	assert.Equal(t, sn.Snapshots[0].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())
}

func TestCsv_GetSnapshotsTranspose(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/transpose.csv"},
				Delimiter:               ";",
				TransposeRowsAndColumns: true,
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 4) // 2 object + root collection + transpose collection + 1 relations

	for _, snapshot := range sn.Snapshots {
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			name := pbtypes.GetString(snapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyName.String())
			assert.True(t, name == "name" || name == "price")
		}
	}
}

func TestCsv_GetSnapshotsTransposeUseFirstRowForRelationsOff(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/transpose.csv"},
				Delimiter:               ";",
				TransposeRowsAndColumns: true,
				UseFirstRowForRelations: false,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 5) // 2 object + root collection + transpose collection + 1 relations

	for _, snapshot := range sn.Snapshots {
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			name := pbtypes.GetString(snapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyName.String())
			assert.True(t, name == "Field 1" || name == "Field 2")
		}
	}
}

func TestCsv_GetSnapshotsUseFirstColumnForRelationsOn(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/Journal.csv"},
				Delimiter:               ",",
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 6) // Journal.csv collection, root collection + 2 objects in Journal.csv + 2 relations (Created, Tags)

	var rowsObjects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.SbType != sb.SmartBlockTypeSubObject &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
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

func assertSnapshotsHaveDetails(t *testing.T, want []string, objects *converter.Snapshot) {
	for _, value := range objects.Snapshot.Data.Details.Fields {
		assert.Contains(t, want, value.GetStringValue())
	}
}

func TestCsv_GetSnapshotsUseFirstColumnForRelationsOff(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:      []string{"testdata/Journal.csv"},
				Delimiter: ",",
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 7) // Journal.csv collection, root collection + 3 objects in Journal.csv + 2 relations (Created, Tags)

	var objects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.SbType != sb.SmartBlockTypeSubObject &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
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

	var subObjects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			subObjects = append(subObjects, snapshot)
		}
	}

	assert.Len(t, subObjects, 2)

	name := pbtypes.GetString(subObjects[0].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 1")

	name = pbtypes.GetString(subObjects[1].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 2")
}

func TestCsv_GetSnapshotsQuotedStrings(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/quotedstrings.csv"},
				Delimiter:               ",",
				TransposeRowsAndColumns: true,
				UseFirstRowForRelations: true,
				Mode:                    pb.RpcObjectImportRequestCsvParams_TABLE,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
}

func TestCsv_GetSnapshotsBigFile(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/bigfile.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, err)
	assert.True(t, errors.Is(err.GetResultError(pb.RpcObjectImportRequest_Csv), converter.ErrLimitExceeded))
	assert.NotNil(t, sn)

	var objects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.SbType != sb.SmartBlockTypeSubObject &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
			objects = append(objects, snapshot)
		}
	}

	assert.Len(t, objects, limitForRows)
	for _, object := range objects {
		assert.Len(t, object.Snapshot.Data.Details.Fields, limitForColumns)
	}
}

func TestCsv_GetSnapshotsEmptyFirstLineUseFirstColumnForRelationsOn(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/emptyfirstline.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	var subObjects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			subObjects = append(subObjects, snapshot)
		}
	}
	assert.Len(t, subObjects, 0)
}

func TestCsv_GetSnapshotsEmptyFirstLineUseFirstColumnForRelationsOff(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/emptyfirstline.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: false,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)

	var subObjects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		if snapshot.SbType == sb.SmartBlockTypeSubObject {
			subObjects = append(subObjects, snapshot)
		}
	}
	assert.Len(t, subObjects, 6)

	name := pbtypes.GetString(subObjects[0].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 1")

	name = pbtypes.GetString(subObjects[1].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 2")

	name = pbtypes.GetString(subObjects[2].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 3")

	name = pbtypes.GetString(subObjects[3].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 4")

	name = pbtypes.GetString(subObjects[4].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 5")

	name = pbtypes.GetString(subObjects[5].Snapshot.Data.Details, bundle.RelationKeyName.String())
	assert.True(t, name == "Field 6")
}

func TestCsv_GetSnapshotsSameRelations(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/samerelations.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 12) // root collection + samerelations CSV collection + 3 relations + 7 objects

	relations := make([]*converter.Snapshot, 0, 3)
	for _, snapshot := range sn.Snapshots {
		name := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String())
		if name == "Nickname" {
			relations = append(relations, snapshot)
		}
	}

	assert.Len(t, relations, 3)
	assert.Equal(t, pbtypes.GetString(relations[0].Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String()),
		"testdata/samerelations.csv/column/Nickname")
	assert.Equal(t, pbtypes.GetString(relations[1].Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String()),
		"testdata/samerelations.csv/column/Nickname0")
	assert.Equal(t, pbtypes.GetString(relations[2].Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String()),
		"testdata/samerelations.csv/column/Nickname1")
}

func TestCsv_GetSnapshots1000RowsFile(t *testing.T) {
	csv := CSV{}
	p := process.NewProgress(pb.ModelProcess_Import)
	// UseFirstRowForRelations is off
	sn, err := csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/1000_rows.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: false,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, err)
	assert.True(t, errors.Is(err.GetResultError(pb.RpcObjectImportRequest_Csv), converter.ErrLimitExceeded))
	assert.NotNil(t, sn)

	var objects []*converter.Snapshot
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.SbType != sb.SmartBlockTypeSubObject &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
			objects = append(objects, snapshot)
		}
	}

	assert.Len(t, objects, limitForRows)
	for _, object := range objects {
		assert.Len(t, object.Snapshot.Data.Details.Fields, limitForColumns)
	}

	// UseFirstRowForRelations is on
	sn, err = csv.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfCsvParams{
			CsvParams: &pb.RpcObjectImportRequestCsvParams{
				Path:                    []string{"testdata/1000_rows.csv"},
				Delimiter:               ";",
				UseFirstRowForRelations: true,
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, err)
	assert.True(t, errors.Is(err.GetResultError(pb.RpcObjectImportRequest_Csv), converter.ErrLimitExceeded))
	assert.NotNil(t, sn)

	objects = []*converter.Snapshot{}
	for _, snapshot := range sn.Snapshots {
		// only objects created from rows
		if snapshot.SbType != sb.SmartBlockTypeSubObject &&
			!lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
			objects = append(objects, snapshot)
		}
	}

	assert.Len(t, objects, limitForRows-1)
	for _, object := range objects {
		assert.Len(t, object.Snapshot.Data.Details.Fields, limitForColumns)
	}
}
