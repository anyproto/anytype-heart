package markdown

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
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
	t.Run("import file with links", func(t *testing.T) {
		// given
		converter := newMDConverter(&MockTempDir{})
		h := &Markdown{blockConverter: converter}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{"testdata"}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 4)

		var found bool
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == "testdata/links.md" {
				found = true
				assert.Len(t, snapshot.Snapshot.Data.Blocks, 14)
				assertLinkBlocks(t, snapshot)
			}
		}
		assert.True(t, found)
	})
}

func assertLinkBlocks(t *testing.T, snapshot *common.Snapshot) {
	assert.Equal(t, "File does not exist test1", snapshot.Snapshot.Data.Blocks[0].GetText().GetText())
	assert.Len(t, snapshot.Snapshot.Data.Blocks[0].GetText().GetMarks().GetMarks(), 1)
	assert.Equal(t, model.BlockContentTextMark_Link, snapshot.Snapshot.Data.Blocks[0].GetText().GetMarks().GetMarks()[0].GetType())

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[1].GetText().GetText(), "Test link to page test2")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[1].GetText().GetMarks().GetMarks(), 1)
	assert.Equal(t, model.BlockContentTextMark_Mention, snapshot.Snapshot.Data.Blocks[1].GetText().GetMarks().GetMarks()[0].GetType())

	assert.NotNil(t, snapshot.Snapshot.Data.Blocks[2].GetFile())
	assert.Contains(t, snapshot.Snapshot.Data.Blocks[2].GetFile().GetName(), "test.txt")

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[3].GetText().GetText(), "Test link to csv test4")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[3].GetText().GetMarks().GetMarks(), 1)
	assert.Equal(t, model.BlockContentTextMark_Mention, snapshot.Snapshot.Data.Blocks[3].GetText().GetMarks().GetMarks()[0].GetType())

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[4].GetText().GetText(), "File does not exist with bold mark test1")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[4].GetText().GetMarks().GetMarks(), 2)
	assert.Equal(t, model.BlockContentTextMark_Link, snapshot.Snapshot.Data.Blocks[4].GetText().GetMarks().GetMarks()[0].GetType())
	assert.Equal(t, model.BlockContentTextMark_Bold, snapshot.Snapshot.Data.Blocks[4].GetText().GetMarks().GetMarks()[1].GetType())

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[5].GetText().GetText(), "Test link to page with bold mark test2")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[5].GetText().GetMarks().GetMarks(), 2)
	assert.Equal(t, model.BlockContentTextMark_Object, snapshot.Snapshot.Data.Blocks[5].GetText().GetMarks().GetMarks()[0].GetType())
	assert.Equal(t, model.BlockContentTextMark_Bold, snapshot.Snapshot.Data.Blocks[5].GetText().GetMarks().GetMarks()[1].GetType())

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[6].GetText().GetText(), "Test file block with bold mark test3")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[6].GetText().GetMarks().GetMarks(), 2)
	assert.Equal(t, model.BlockContentTextMark_Link, snapshot.Snapshot.Data.Blocks[6].GetText().GetMarks().GetMarks()[0].GetType())
	assert.Equal(t, model.BlockContentTextMark_Bold, snapshot.Snapshot.Data.Blocks[6].GetText().GetMarks().GetMarks()[1].GetType())

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[7].GetText().GetText(), "Test link to csv with bold mark test4")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[7].GetText().GetMarks().GetMarks(), 2)
	assert.Equal(t, model.BlockContentTextMark_Object, snapshot.Snapshot.Data.Blocks[7].GetText().GetMarks().GetMarks()[0].GetType())
	assert.Equal(t, model.BlockContentTextMark_Bold, snapshot.Snapshot.Data.Blocks[7].GetText().GetMarks().GetMarks()[1].GetType())

	assert.NotNil(t, snapshot.Snapshot.Data.Blocks[8].GetBookmark())
	assert.Equal(t, "testdata/file.md", snapshot.Snapshot.Data.Blocks[8].GetBookmark().GetUrl())

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[9].GetText().GetText(), "test2")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[9].GetText().GetMarks().GetMarks(), 2)
	assert.Equal(t, model.BlockContentTextMark_Object, snapshot.Snapshot.Data.Blocks[9].GetText().GetMarks().GetMarks()[0].GetType())
	assert.Equal(t, model.BlockContentTextMark_Bold, snapshot.Snapshot.Data.Blocks[9].GetText().GetMarks().GetMarks()[1].GetType())

	assert.NotNil(t, snapshot.Snapshot.Data.Blocks[10].GetFile())
	assert.Contains(t, snapshot.Snapshot.Data.Blocks[10].GetFile().GetName(), "test.txt")

	assert.Equal(t, snapshot.Snapshot.Data.Blocks[11].GetText().GetText(), "test4")
	assert.Len(t, snapshot.Snapshot.Data.Blocks[11].GetText().GetMarks().GetMarks(), 2)
	assert.Equal(t, model.BlockContentTextMark_Object, snapshot.Snapshot.Data.Blocks[11].GetText().GetMarks().GetMarks()[0].GetType())
	assert.Equal(t, model.BlockContentTextMark_Bold, snapshot.Snapshot.Data.Blocks[11].GetText().GetMarks().GetMarks()[1].GetType())
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
