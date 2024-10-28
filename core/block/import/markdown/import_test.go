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
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
)

func TestMarkdown_GetSnapshots(t *testing.T) {
	t.Run("get snapshots of root collection, csv collection and object", func(t *testing.T) {
		// given
		testDirectory := setupTestDirectory(t)
		h := &Markdown{}
		p := process.NewNoOp()

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
		p := process.NewNoOp()

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
		tempDirProvider := &MockTempDir{}
		converter := newMDConverter(tempDirProvider)
		h := &Markdown{blockConverter: converter}
		p := process.NewNoOp()

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

		fileNameToObjectId := make(map[string]string, len(sn.Snapshots))
		for _, snapshot := range sn.Snapshots {
			fileNameToObjectId[snapshot.FileName] = snapshot.Id
		}
		var found bool
		expectedPath := filepath.Join("testdata", "links.md")
		rootId := fileNameToObjectId[expectedPath]
		want := buildExpectedTree(fileNameToObjectId, tempDirProvider, rootId)
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == expectedPath {
				found = true
				blockbuilder.AssertTreesEqual(t, want.Build(), snapshot.Snapshot.Data.Blocks)
			}
		}
		assert.True(t, found)
	})
}

func buildExpectedTree(fileNameToObjectId map[string]string, provider *MockTempDir, rootId string) *blockbuilder.Block {
	fileMdPath := filepath.Join("testdata", "file.md")
	testMdPath := filepath.Join("testdata", "test.md")
	testCsvPath := filepath.Join("testdata", "test.csv")
	testTxtPath := filepath.Join("testdata", "test.txt")
	want := blockbuilder.Root(
		blockbuilder.ID(rootId),
		blockbuilder.Children(
			blockbuilder.Text("File does not exist test1", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 20, To: 25},
					Type:  model.BlockContentTextMark_Link,
					Param: fileMdPath,
				},
			}})),
			blockbuilder.Text("Test link to page test2", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 18, To: 23},
					Type:  model.BlockContentTextMark_Mention,
					Param: fileNameToObjectId[testMdPath],
				},
			}})),
			blockbuilder.File("", blockbuilder.FileName(filepath.Join(provider.TempDir(), testTxtPath)), blockbuilder.FileType(model.BlockContentFile_File)),
			blockbuilder.Text("Test link to csv test4", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 17, To: 22},
					Type:  model.BlockContentTextMark_Mention,
					Param: fileNameToObjectId[testCsvPath],
				},
			}})),
			blockbuilder.Text("File does not exist with bold mark test1", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 35, To: 40},
					Type:  model.BlockContentTextMark_Link,
					Param: fileMdPath,
				},
				{
					Range: &model.Range{From: 35, To: 40},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Text("Test link to page with bold mark test2", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 33, To: 38},
					Type:  model.BlockContentTextMark_Mention,
					Param: fileNameToObjectId[testMdPath],
				},
				{
					Range: &model.Range{From: 33, To: 38},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Text("Test file block with bold mark test3", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 31, To: 36},
					Type:  model.BlockContentTextMark_Link,
					Param: testTxtPath,
				},
				{
					Range: &model.Range{From: 31, To: 36},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Text("Test link to csv with bold mark test4", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 32, To: 37},
					Type:  model.BlockContentTextMark_Mention,
					Param: fileNameToObjectId[testCsvPath],
				},
				{
					Range: &model.Range{From: 32, To: 37},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Bookmark(fileMdPath),
			blockbuilder.Text("test2", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Mention,
					Param: fileNameToObjectId[testMdPath],
				},
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.File("", blockbuilder.FileName(filepath.Join(provider.TempDir(), testTxtPath)), blockbuilder.FileType(model.BlockContentFile_File)),
			blockbuilder.Text("test4", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Mention,
					Param: fileNameToObjectId[testCsvPath],
				},
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Link(rootId),
		))
	return want
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
