package markdown

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/test"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 4) // Including objectType relation
		var (
			found     bool
			subPageId string
		)
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == filepath.Join(testDirectory, "test_database", "test.md") {
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
		assert.Greater(t, len(sn.Snapshots), 7) // More snapshots due to YAML properties

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
	t.Run("no object in archive", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		zipPath := filepath.Join(testDirectory, "empty.zip")
		test.CreateEmptyZip(t, zipPath)

		h := &Markdown{}
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{zipPath}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.NotNil(t, ce)
		assert.Nil(t, sn)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Markdown), common.ErrFileImportNoObjectsInZipArchive))
	})
	t.Run("import non utf files", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		zipPath := filepath.Join(testDirectory, "nonutf.zip")
		fileMdName := "こんにちは.md"
		fileCsvName := "你好.csv"
		fileWithLinksName := "nonutflinks.md"

		test.CreateZipWithFiles(t, zipPath, "testdata", []*zip.FileHeader{
			{
				Name:   fileWithLinksName,
				Method: zip.Deflate,
			},
			{
				Name:    fileMdName,
				Method:  zip.Deflate,
				NonUTF8: true,
			},
			{
				Name:    fileCsvName,
				Method:  zip.Deflate,
				NonUTF8: true,
			},
		})

		h := &Markdown{}
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{zipPath}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 5) // Including objectType relation
		fileNameToObjectId := make(map[string]string, len(sn.Snapshots))
		for _, snapshot := range sn.Snapshots {
			fileNameToObjectId[snapshot.FileName] = snapshot.Id
		}
		var found bool
		rootId := fileNameToObjectId[fileWithLinksName]
		want := buildTreeWithNonUtfLinks(fileNameToObjectId, rootId)
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == fileWithLinksName {
				found = true
				blockbuilder.AssertTreesEqual(t, want.Build(), snapshot.Snapshot.Data.Blocks)
			}
		}
		assert.True(t, found)
	})
}

func buildTreeWithNonUtfLinks(fileNameToObjectId map[string]string, rootId string) *blockbuilder.Block {
	testMdPath := fileNameToObjectId["import file 2.md"]
	testCsvPath := fileNameToObjectId["import file 3.csv"]

	want := blockbuilder.Root(
		blockbuilder.ID(rootId),
		blockbuilder.Children(
			blockbuilder.Text("NonUtf 1 test6", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 9, To: 14},
					Type:  model.BlockContentTextMark_Mention,
					Param: testMdPath,
				},
			}})),
			blockbuilder.Text("NonUtf 2 test7", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 9, To: 14},
					Type:  model.BlockContentTextMark_Mention,
					Param: testCsvPath,
				},
			}})),
			blockbuilder.Text("NonUtf 1 test6", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 9, To: 14},
					Type:  model.BlockContentTextMark_Mention,
					Param: testMdPath,
				},
				{
					Range: &model.Range{From: 9, To: 14},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Text("NonUtf 2 test7", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 9, To: 14},
					Type:  model.BlockContentTextMark_Mention,
					Param: testCsvPath,
				},
				{
					Range: &model.Range{From: 9, To: 14},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Text("test6", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Mention,
					Param: testMdPath,
				},
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
			blockbuilder.Text("test7", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Mention,
					Param: testCsvPath,
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

func buildExpectedTree(fileNameToObjectId map[string]string, provider *MockTempDir, rootId string) *blockbuilder.Block {
	fileMdPath := filepath.Join("testdata", "file.md")
	testMdPath := filepath.Join("testdata", "test.md")
	testCsvPath := filepath.Join("testdata", "test.csv")
	testTxtPath := filepath.Join("testdata", "test.txt")
	url := "http://example.com/%zz"
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
			blockbuilder.Text("Should not panic test5", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 17, To: 22},
					Type:  model.BlockContentTextMark_Link,
					Param: url,
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
			blockbuilder.Text("Should not panic test5", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 17, To: 22},
					Type:  model.BlockContentTextMark_Link,
					Param: url,
				},
				{
					Range: &model.Range{From: 17, To: 22},
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
			blockbuilder.Text("test5", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Link,
					Param: url,
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

func TestMarkdown_YAMLFrontMatterImport(t *testing.T) {
	t.Run("import markdown with YAML front matter creates properties and types", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		yamlMdPath := filepath.Join(testDirectory, "yaml_test.md")

		// Create test file with YAML front matter
		yamlContent := `---
title: Test Document
Object Type: Task
Start Date: 2023-06-01
End Date: 2023-06-01T14:30:00
priority: high
done: true
count: 42
rating: 4.5
tags: [test, markdown, yaml]
website: https://anytype.io
email: test@example.com
description: This is a longer description that contains more details about the test document. It should be imported as a longtext relation.
---

# Test Document

This is the content of the test document.`

		err := os.WriteFile(yamlMdPath, []byte(yamlContent), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{}
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Check that we have snapshots for the object, relations, and types
		var objectSnapshot *common.Snapshot
		var relationSnapshots []*common.Snapshot
		var typeSnapshots []*common.Snapshot

		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == yamlMdPath {
				objectSnapshot = snapshot
			} else if snapshot.Snapshot.SbType == coresb.SmartBlockTypeRelation {
				// This is a relation snapshot
				relationSnapshots = append(relationSnapshots, snapshot)
			} else if snapshot.Snapshot.SbType == coresb.SmartBlockTypeObjectType {
				// This is a type snapshot
				typeSnapshots = append(typeSnapshots, snapshot)
			}
		}

		// Verify object snapshot exists
		assert.NotNil(t, objectSnapshot)

		// Verify we have relation snapshots for all YAML properties
		expectedRelations := []string{
			"title", "Start Date", "End Date", "priority", "done",
			"count", "rating", "tags", "website", "email", "description",
		}
		assert.GreaterOrEqual(t, len(relationSnapshots), len(expectedRelations))

		// Verify the object has the correct details from YAML
		details := objectSnapshot.Snapshot.Data.Details
		assert.NotNil(t, details)

		// Check specific property values by finding their keys from relations
		// Build a map of property name to key from relation snapshots
		propKeyMap := make(map[string]string)
		for _, relSnapshot := range relationSnapshots {
			relDetails := relSnapshot.Snapshot.Data.Details
			name := relDetails.GetString(bundle.RelationKeyName)
			key := relSnapshot.Snapshot.Data.Key
			propKeyMap[name] = key
		}

		// Now check values using the correct keys
		if titleKey, ok := propKeyMap["title"]; ok {
			assert.Equal(t, "Test Document", details.GetString(domain.RelationKey(titleKey)))
		}

		// Check that dates are stored as number values (timestamps)
		if startDateKey, ok := propKeyMap["Start Date"]; ok {
			startDate := details.GetInt64(domain.RelationKey(startDateKey))
			assert.Greater(t, startDate, int64(0))
		}

		// Check boolean value
		if doneKey, ok := propKeyMap["done"]; ok {
			assert.Equal(t, true, details.GetBool(domain.RelationKey(doneKey)))
		}

		// Check number values
		if countKey, ok := propKeyMap["count"]; ok {
			assert.Equal(t, int64(42), details.GetInt64(domain.RelationKey(countKey)))
		}
		if ratingKey, ok := propKeyMap["rating"]; ok {
			assert.Equal(t, 4.5, details.GetFloat64(domain.RelationKey(ratingKey)))
		}

		// Check tags as list
		if tagsKey, ok := propKeyMap["tags"]; ok {
			tags := details.GetStringList(domain.RelationKey(tagsKey))
			assert.Len(t, tags, 3)
			assert.Contains(t, tags, "test")
			assert.Contains(t, tags, "markdown")
			assert.Contains(t, tags, "yaml")
		}

		// Verify relation formats
		for _, relSnapshot := range relationSnapshots {
			relDetails := relSnapshot.Snapshot.Data.Details
			format := relDetails.GetInt64(bundle.RelationKeyRelationFormat)
			relName := relDetails.GetString(bundle.RelationKeyName)

			switch relName {
			case "title", "priority":
				assert.Equal(t, int64(model.RelationFormat_shorttext), format)
			case "description":
				assert.Equal(t, int64(model.RelationFormat_longtext), format)
			case "Start Date", "End Date":
				assert.Equal(t, int64(model.RelationFormat_date), format)
				// Check includeTime for End Date
				if relName == "End Date" {
					includeTime := relDetails.GetBool(bundle.RelationKeyRelationFormatIncludeTime)
					assert.True(t, includeTime)
				}
				if relName == "Start Date" {
					includeTime := relDetails.GetBool(bundle.RelationKeyRelationFormatIncludeTime)
					assert.False(t, includeTime)
				}
			case "done":
				assert.Equal(t, int64(model.RelationFormat_checkbox), format)
			case "count", "rating":
				assert.Equal(t, int64(model.RelationFormat_number), format)
			case "tags":
				assert.Equal(t, int64(model.RelationFormat_tag), format)
			case "website":
				assert.Equal(t, int64(model.RelationFormat_url), format)
			case "email":
				assert.Equal(t, int64(model.RelationFormat_email), format)
			}
		}
	})

	t.Run("import multiple markdown files with shared YAML properties", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()

		// Create first file
		yamlContent1 := `---
title: First Document
Type: Task
priority: high
author: John Doe
---

# First Document`

		// Create second file with overlapping properties
		yamlContent2 := `---
title: Second Document
Type: Note
priority: low
author: Jane Smith
category: Work
---

# Second Document`

		err := os.WriteFile(filepath.Join(testDirectory, "file1.md"), []byte(yamlContent1), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(filepath.Join(testDirectory, "file2.md"), []byte(yamlContent2), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{}
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Count unique relation names
		relationNames := make(map[string]bool)
		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.Data.Key != "" && snapshot.Snapshot.Data.ObjectTypes != nil {
				details := snapshot.Snapshot.Data.Details
				name := details.GetString(bundle.RelationKeyName)
				if name != "" {
					relationNames[name] = true
				}
			}
		}

		// Should have relations for: title, priority, author, category
		assert.Contains(t, relationNames, "title")
		assert.Contains(t, relationNames, "priority")
		assert.Contains(t, relationNames, "author")
		assert.Contains(t, relationNames, "category")
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

func TestMarkdown_YAMLFrontMatterSnapshot(t *testing.T) {
	t.Run("snapshot test for YAML front matter import", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		yamlMdPath := filepath.Join(testDirectory, "snapshot_test.md")

		// Create test file with comprehensive YAML front matter
		yamlContent := `---
title: Snapshot Test Document
Type: Task
author: Test Author
priority: high
status: in-progress
Start Date: 2023-06-01
End Date: 2023-06-01T14:30:00
done: false
progress: 75
score: 9.5
tags: [important, test, snapshot]
assignees: [john, jane, bob]
website: https://example.com
contact: test@example.com
notes: Brief notes about the task
description: This is a much longer description that spans multiple lines and contains detailed information about the task. It should be imported as a longtext relation due to its length exceeding 100 characters.
metadata:
  version: 1.0
  created_by: system
---

# Snapshot Test Document

This document is used for snapshot testing of YAML front matter import.`

		err := os.WriteFile(yamlMdPath, []byte(yamlContent), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{}
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Create a map to store relation details by name for verification
		relationsByName := make(map[string]map[string]any)
		var mainObjectDetails *domain.Details

		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == yamlMdPath {
				mainObjectDetails = snapshot.Snapshot.Data.Details
			} else if snapshot.Snapshot.Data.Key != "" {
				// This is a relation snapshot
				details := snapshot.Snapshot.Data.Details
				relName := details.GetString(bundle.RelationKeyName)
				if relName != "" {
					relationsByName[relName] = map[string]any{
						"format": details.GetInt64(bundle.RelationKeyRelationFormat),
						"key":    snapshot.Snapshot.Data.Key,
					}
					if details.Has(bundle.RelationKeyRelationFormatIncludeTime) {
						relationsByName[relName]["includeTime"] = details.GetBool(bundle.RelationKeyRelationFormatIncludeTime)
					}
				}
			}
		}

		// Verify main object details
		assert.NotNil(t, mainObjectDetails)

		// Verify all expected properties exist with correct values
		// Use the relationsByName map to get the correct keys
		getKey := func(name string) domain.RelationKey {
			if rel, ok := relationsByName[name]; ok {
				return domain.RelationKey(rel["key"].(string))
			}
			return domain.RelationKey(name)
		}

		assert.Equal(t, "Snapshot Test Document", mainObjectDetails.GetString(getKey("title")))
		assert.Equal(t, "Test Author", mainObjectDetails.GetString(getKey("author")))
		assert.Equal(t, "high", mainObjectDetails.GetString(getKey("priority")))
		assert.Equal(t, "in-progress", mainObjectDetails.GetString(getKey("status")))

		// Check dates are timestamps
		startDate := mainObjectDetails.GetInt64(getKey("Start Date"))
		assert.Greater(t, startDate, int64(0))
		endDate := mainObjectDetails.GetInt64(getKey("End Date"))
		assert.Greater(t, endDate, int64(0))

		// Check other values
		assert.Equal(t, false, mainObjectDetails.GetBool(getKey("done")))
		assert.Equal(t, int64(75), mainObjectDetails.GetInt64(getKey("progress")))
		assert.Equal(t, 9.5, mainObjectDetails.GetFloat64(getKey("score")))

		// Check lists
		tags := mainObjectDetails.GetStringList(getKey("tags"))
		assert.Equal(t, []string{"important", "test", "snapshot"}, tags)
		assignees := mainObjectDetails.GetStringList(getKey("assignees"))
		assert.Equal(t, []string{"john", "jane", "bob"}, assignees)

		// Check URLs and emails
		assert.Equal(t, "https://example.com", mainObjectDetails.GetString(getKey("website")))
		assert.Equal(t, "test@example.com", mainObjectDetails.GetString(getKey("contact")))

		// Check text fields
		assert.Equal(t, "Brief notes about the task", mainObjectDetails.GetString(getKey("notes")))
		assert.Equal(t, "This is a much longer description that spans multiple lines and contains detailed information about the task. It should be imported as a longtext relation due to its length exceeding 100 characters.", mainObjectDetails.GetString(getKey("description")))

		// Note: metadata is skipped as YAML maps are not supported
		// Check that metadata key doesn't exist in relationsByName
		_, hasMetadata := relationsByName["metadata"]
		assert.False(t, hasMetadata, "Nested YAML objects should be skipped")

		// Verify relation formats
		expectedFormats := map[string]struct {
			format      model.RelationFormat
			includeTime bool
		}{
			"title":       {format: model.RelationFormat_shorttext, includeTime: false},
			"author":      {format: model.RelationFormat_shorttext, includeTime: false},
			"priority":    {format: model.RelationFormat_shorttext, includeTime: false},
			"status":      {format: model.RelationFormat_status, includeTime: false},
			"Start Date":  {format: model.RelationFormat_date, includeTime: false},
			"End Date":    {format: model.RelationFormat_date, includeTime: true},
			"done":        {format: model.RelationFormat_checkbox, includeTime: false},
			"progress":    {format: model.RelationFormat_number, includeTime: false},
			"score":       {format: model.RelationFormat_number, includeTime: false},
			"tags":        {format: model.RelationFormat_tag, includeTime: false},
			"assignees":   {format: model.RelationFormat_tag, includeTime: false},
			"website":     {format: model.RelationFormat_url, includeTime: false},
			"contact":     {format: model.RelationFormat_email, includeTime: false},
			"notes":       {format: model.RelationFormat_shorttext, includeTime: false},
			"description": {format: model.RelationFormat_longtext, includeTime: false},
		}

		for relName, expected := range expectedFormats {
			rel, ok := relationsByName[relName]
			assert.True(t, ok, "Expected relation %s to exist", relName)

			assert.Equal(t, int64(expected.format), rel["format"], "Wrong format for relation %s", relName)

			if expected.format == model.RelationFormat_date {
				includeTime, hasIncludeTime := rel["includeTime"]
				assert.True(t, hasIncludeTime, "Date relation %s should have includeTime", relName)
				assert.Equal(t, expected.includeTime, includeTime, "Wrong includeTime for relation %s", relName)
			}
		}

		// Verify all relations have BSON-style keys
		for relName, rel := range relationsByName {
			key := rel["key"].(string)
			assert.NotEmpty(t, key, "Relation %s should have a key", relName)
			
			// Skip system relations which don't have BSON IDs
			if key == "objectType" {
				continue
			}
			
			assert.Len(t, key, 24, "Relation %s key should be 24 characters (BSON ID)", relName)
		}
	})
}
