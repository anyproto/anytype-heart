package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestMarkdown_GetSnapshots(t *testing.T) {
	t.Run("get snapshots of root collection, csv collection and object", func(t *testing.T) {
		// given
		testDirectory := setupTestDirectory(t)
		h := &Markdown{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 3)
		var (
			found     bool
			subPageId string
		)
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == filepath.Join(testDirectory, "test_database/test.md") {
				subPageId = snapshot.Id
				break
			}
		}
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == filepath.Join(testDirectory, "test_database.csv") {
				found = true
				assert.NotEmpty(t, snapshot.Snapshot.Data.Collections.Fields["objects"])
				assert.Len(t, snapshot.Snapshot.Data.Collections.Fields["objects"].GetListValue().GetValues(), 1)
				assert.Equal(t, subPageId, snapshot.Snapshot.Data.Collections.Fields["objects"].GetListValue().GetValues()[0].GetStringValue())
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("no object error", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		h := &Markdown{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.NotNil(t, err)
		assert.Nil(t, sn)
		assert.True(t, err.IsNoObjectToImportError(1))
	})
}

func setupTestDirectory(t *testing.T) string {
	tmpDir := t.TempDir()

	testdataDir := filepath.Join(tmpDir, "testdata")
	testDatabaseDir := filepath.Join(testdataDir, "test_database")
	csvFilePath := filepath.Join(testdataDir, "test_database.csv")
	mdFilePath := filepath.Join(testDatabaseDir, "test.md")

	err := os.MkdirAll(testDatabaseDir, os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	err = os.WriteFile(csvFilePath, []byte("Name,Tags\nTest\n"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create test_database.csv: %v", err)
	}

	err = os.WriteFile(mdFilePath, []byte("# Sample Markdown File\n"), os.ModePerm)
	if err != nil {
		t.Fatalf("Failed to create test.md: %v", err)
	}

	return testdataDir
}
