package csv

import (
	smartblock2 "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
			CsvParams: &pb.RpcObjectImportRequestCsvParams{Path: []string{"testdata/Journal.csv"}},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 7) // test + root collection + 2 objects in Journal.csv + 3 relations (Name, Created, Tags)
	assert.Contains(t, sn.Snapshots[0].FileName, "Journal.csv")
	assert.Len(t, pbtypes.GetStringList(sn.Snapshots[0].Snapshot.Data.Collections, template.CollectionStoreKey), 2) // 2 objects

	var found bool
	for _, snapshot := range sn.Snapshots {
		if snapshot.FileName == rootCollectionName {
			found = true
			assert.NotEmpty(t, snapshot.Snapshot.Data.ObjectTypes)
			assert.Equal(t, snapshot.Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())
			assert.Len(t, pbtypes.GetStringList(snapshot.Snapshot.Data.Collections, template.CollectionStoreKey), 3) // all objects
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
	assert.Len(t, sn.Snapshots, 13) // 8 objects + root collection + semicolon collection + 3 relations
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
	assert.Len(t, sn.Snapshots, 5) // 1 object + root collection + transpose collection + 2 relations

	for _, snapshot := range sn.Snapshots {
		if snapshot.SbType == smartblock2.SmartBlockTypeSubObject {
			name := pbtypes.GetString(snapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyName.String())
			assert.True(t, name == "name" || name == "price")
		}
	}
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
			},
		},
		Type: pb.RpcObjectImportRequest_Csv,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.Nil(t, err)
	assert.NotNil(t, sn)
}
