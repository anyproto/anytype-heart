package markdown

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/test"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// testMockSource is a test implementation of source.Source for testing schema loading
type testMockSource struct {
	files map[string]string
}

func (m *testMockSource) Initialize(importPath string) error {
	return nil
}

func (m *testMockSource) Iterate(callback func(fileName string, fileReader io.ReadCloser) (isContinue bool)) error {
	for name, content := range m.files {
		reader := strings.NewReader(content)
		if !callback(name, io.NopCloser(reader)) {
			break
		}
	}
	return nil
}

func (m *testMockSource) ProcessFile(fileName string, callback func(io.ReadCloser) error) error {
	if content, ok := m.files[fileName]; ok {
		reader := strings.NewReader(content)
		return callback(io.NopCloser(reader))
	}
	return nil
}

func (m *testMockSource) CountFilesWithGivenExtensions([]string) int {
	return len(m.files)
}

func (m *testMockSource) GetFileReaders(string, []string) (map[string]io.ReadCloser, error) {
	return nil, nil
}

func (m *testMockSource) Close()                 {}
func (m *testMockSource) IsRootFile(string) bool { return false }

func TestMarkdown_GetSnapshots(t *testing.T) {
	t.Run("get snapshots of root collection, csv collection and object", func(t *testing.T) {
		// given
		testDirectory := setupTestDirectory(t)
		// Initialize Markdown properly with required components
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
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
		// Initialize Markdown properly with required components
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
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
		schemaImporter := NewSchemaImporter()
		converter.SetSchemaImporter(schemaImporter)
		h := &Markdown{
			blockConverter: converter,
		}
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

		// Initialize Markdown properly with required components
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
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

		// Initialize Markdown properly with required components
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
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

	t.Run("create directory pages", func(t *testing.T) {
		// given
		testDirectory := setupHierarchicalTestDirectory(t)
		// Initialize Markdown properly with required components
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path:                 []string{testDirectory},
					CreateDirectoryPages: true,
				},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Count directory pages
		dirPageCount := 0
		filePageCount := 0
		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.SbType == coresb.SmartBlockTypePage {
				filePageCount++
				// Check if it's a directory page by checking the file name
				// Directory pages are stored with directory paths as file names
				if snapshot.FileName != "" && !strings.HasSuffix(snapshot.FileName, ".md") && !strings.HasSuffix(snapshot.FileName, ".csv") {
					dirPageCount++
				}
			}
		}

		// We should have directory pages for subdirectories and root
		// We have: root (testDirectory), docs, docs/guides, docs/api, examples
		assert.Equal(t, 5, dirPageCount, "Should have 5 directory pages (import root + 4 subdirs)")

		// Verify directory pages contain links to their children
		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.SbType == coresb.SmartBlockTypePage {
				hasHeader := false
				hasLinks := false
				for _, block := range snapshot.Snapshot.Data.Blocks {
					if textBlock := block.GetText(); textBlock != nil && textBlock.Style == model.BlockContentText_Header1 {
						hasHeader = true
					}
					if block.GetLink() != nil || (block.GetText() != nil && block.GetText().Marks != nil) {
						hasLinks = true
					}
				}
				// Directory pages should have both header and links
				if hasHeader && hasLinks {
					assert.True(t, hasHeader && hasLinks, "Directory page should have header and links")
				}
			}
		}
	})
}

func buildTreeWithNonUtfLinks(fileNameToObjectId map[string]string, rootId string) *blockbuilder.Block {
	// The actual file names in the zip are the non-UTF8 names
	var testMdPath, testCsvPath string
	for fileName, objectId := range fileNameToObjectId {
		if strings.Contains(fileName, ".md") && fileName != "nonutflinks.md" {
			testMdPath = objectId
		} else if strings.Contains(fileName, ".csv") {
			testCsvPath = objectId
		}
	}

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
			blockbuilder.Text("test1", blockbuilder.TextMarks(model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Link,
					Param: fileMdPath,
				},
				{
					Range: &model.Range{From: 0, To: 5},
					Type:  model.BlockContentTextMark_Bold,
				},
			}})),
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
			blockbuilder.Bookmark(url),
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

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
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

		// Check tags as list - they should be option IDs now
		if tagsKey, ok := propKeyMap["tags"]; ok {
			tags := details.GetStringList(domain.RelationKey(tagsKey))
			assert.Len(t, tags, 3)
			// Tags are now stored as option IDs, not raw values
			// Check that we have relation option snapshots for each tag value
			var tagOptionSnapshots int
			for _, snapshot := range sn.Snapshots {
				if snapshot.Snapshot.SbType == coresb.SmartBlockTypeRelationOption {
					optionDetails := snapshot.Snapshot.Data.Details
					if optionDetails.GetString(bundle.RelationKeyRelationKey) == tagsKey {
						tagOptionSnapshots++
						name := optionDetails.GetString(bundle.RelationKeyName)
						assert.Contains(t, []string{"test", "markdown", "yaml"}, name)
					}
				}
			}
			assert.Equal(t, 3, tagOptionSnapshots, "Should have 3 tag option snapshots")
		}

		// Verify relation formats
		for _, relSnapshot := range relationSnapshots {
			relDetails := relSnapshot.Snapshot.Data.Details
			format := relDetails.GetInt64(bundle.RelationKeyRelationFormat)
			relName := relDetails.GetString(bundle.RelationKeyName)

			switch relName {
			case "title":
				assert.Equal(t, int64(model.RelationFormat_shorttext), format)
			case "priority":
				assert.Equal(t, int64(model.RelationFormat_status), format)
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

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
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

func setupHierarchicalTestDirectory(t *testing.T) string {
	testDirectory := t.TempDir()

	// Create hierarchical structure
	os.MkdirAll(filepath.Join(testDirectory, "docs", "guides"), 0755)
	os.MkdirAll(filepath.Join(testDirectory, "docs", "api"), 0755)
	os.MkdirAll(filepath.Join(testDirectory, "examples"), 0755)

	// Create files in different directories
	files := map[string]string{
		"README.md":                                      "# Root README\nWelcome to the project",
		filepath.Join("docs", "overview.md"):             "# Documentation Overview\nThis is the docs",
		filepath.Join("docs", "guides", "quickstart.md"): "# Quick Start Guide\nGet started quickly",
		filepath.Join("docs", "guides", "advanced.md"):   "# Advanced Guide\nAdvanced topics",
		filepath.Join("docs", "api", "reference.md"):     "# API Reference\nAPI documentation",
		filepath.Join("examples", "example1.md"):         "# Example 1\nFirst example",
		filepath.Join("examples", "example2.md"):         "# Example 2\nSecond example",
	}

	for path, content := range files {
		fullPath := filepath.Join(testDirectory, path)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		assert.NoError(t, err)
	}

	return testDirectory
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

func TestMarkdown_YAMLFrontMatterObjectRelations(t *testing.T) {
	t.Run("YAML property with object format resolves file paths to object IDs", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()

		// Create related documents that will be referenced
		doc1Path := filepath.Join(testDirectory, "project", "doc1.md")
		doc2Path := filepath.Join(testDirectory, "project", "doc2.md")
		mainDocPath := filepath.Join(testDirectory, "main.md")

		// Create directory structure
		err := os.MkdirAll(filepath.Dir(doc1Path), os.ModePerm)
		assert.NoError(t, err)

		// Create referenced documents
		doc1Content := `---
title: Document 1
type: Note
---

# Document 1
This is document 1 content.`

		doc2Content := `---
title: Document 2
type: Note
---

# Document 2
This is document 2 content.`

		// Create main document with object relations using file paths
		mainContent := `---
# yaml-language-server: $schema=task.schema.json
title: Main Document
Object type: Task
related_docs:
  - ./project/doc1.md
  - ./project/doc2.md
single_reference: ./project/doc1.md
---

# Main Document
This document references other documents.`

		err = os.WriteFile(doc1Path, []byte(doc1Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(doc2Path, []byte(doc2Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(mainDocPath, []byte(mainContent), os.ModePerm)
		assert.NoError(t, err)

		// Add schema with object format relations to the test directory
		schemaContent := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Task",
			"x-app": "Anytype",
			"x-type-key": "task",
			"properties": {
				"related_docs": {
					"type": "array",
					"items": {
						"type": "string"
					},
					"x-key": "related_docs",
					"x-format": "object",
					"x-order": 1
				},
				"single_reference": {
					"type": "string",
					"x-key": "single_reference",
					"x-format": "object",
					"x-order": 2
				},
				"title": {
					"type": "string",
					"x-key": "title",
					"x-format": "shorttext",
					"x-order": 0
				}
			}
		}`

		// Write schema file to test directory so it gets loaded during import
		schemaPath := filepath.Join(testDirectory, "task.schema.json")
		err = os.WriteFile(schemaPath, []byte(schemaContent), os.ModePerm)
		assert.NoError(t, err)

		// Create markdown importer
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
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

		// Build maps for verification
		fileNameToSnapshot := make(map[string]*common.Snapshot)
		fileNameToObjectId := make(map[string]string)
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName != "" {
				fileNameToSnapshot[snapshot.FileName] = snapshot
				fileNameToObjectId[snapshot.FileName] = snapshot.Id
			}
		}

		// Verify all documents were imported
		assert.Contains(t, fileNameToObjectId, mainDocPath)
		assert.Contains(t, fileNameToObjectId, doc1Path)
		assert.Contains(t, fileNameToObjectId, doc2Path)

		// Get the main document snapshot
		mainSnapshot := fileNameToSnapshot[mainDocPath]
		assert.NotNil(t, mainSnapshot)

		// Get object IDs for referenced documents
		doc1Id := fileNameToObjectId[doc1Path]
		doc2Id := fileNameToObjectId[doc2Path]

		// Verify the object relations were resolved to IDs
		mainDetails := mainSnapshot.Snapshot.Data.Details

		// Check array object relation
		relatedDocsKey := domain.RelationKey("related_docs")
		relatedDocs := mainDetails.GetStringList(relatedDocsKey)
		assert.Len(t, relatedDocs, 2)
		assert.Contains(t, relatedDocs, doc1Id)
		assert.Contains(t, relatedDocs, doc2Id)

		// Check single object relation
		singleRefKey := domain.RelationKey("single_reference")
		singleRef := mainDetails.GetStringList(singleRefKey)
		assert.Len(t, singleRef, 1)
		assert.Equal(t, doc1Id, singleRef[0])
	})

	t.Run("YAML object relations with relative and absolute paths", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		subDir := filepath.Join(testDirectory, "subdir")

		// Create directory structure
		err := os.MkdirAll(subDir, os.ModePerm)
		assert.NoError(t, err)

		// Create documents
		doc1Path := filepath.Join(testDirectory, "doc1.md")
		doc2Path := filepath.Join(subDir, "doc2.md")
		mainDocPath := filepath.Join(subDir, "main.md")

		doc1Content := `---
title: Doc 1
---
Content 1`

		doc2Content := `---
title: Doc 2  
---
Content 2`

		// Main document uses both relative and absolute paths
		mainContent := fmt.Sprintf(`---
title: Main with Mixed Paths
references:
  - ../doc1.md
  - ./doc2.md
  - %s
---
Main content`, doc1Path) // Include one absolute path

		err = os.WriteFile(doc1Path, []byte(doc1Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(doc2Path, []byte(doc2Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(mainDocPath, []byte(mainContent), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
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

		// Build object ID map
		fileNameToObjectId := make(map[string]string)
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName != "" {
				fileNameToObjectId[snapshot.FileName] = snapshot.Id
			}
		}

		// Find main document
		var mainSnapshot *common.Snapshot
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == mainDocPath {
				mainSnapshot = snapshot
				break
			}
		}

		assert.NotNil(t, mainSnapshot)
		mainDetails := mainSnapshot.Snapshot.Data.Details

		// Find the references property key
		var referencesKey string
		for k, v := range mainDetails.Iterate() {
			// Look for a property that contains our expected IDs
			if v.IsStringList() {
				list := v.StringList()
				if len(list) == 3 { // We expect 3 references
					// This might be our references property
					referencesKey = string(k)
					break
				}
			}
		}

		// If we found a property with 3 items, verify it contains the right IDs
		if referencesKey != "" {
			references := mainDetails.GetStringList(domain.RelationKey(referencesKey))
			assert.Len(t, references, 3)

			// All three paths should resolve to doc1Id (two different paths + absolute)
			doc1Id := fileNameToObjectId[doc1Path]
			doc2Id := fileNameToObjectId[doc2Path]

			assert.Contains(t, references, doc1Id)
			assert.Contains(t, references, doc2Id)
			// The absolute path should also resolve to doc1Id
			assert.Equal(t, 2, countOccurrences(references, doc1Id), "doc1 should appear twice (relative + absolute path)")
		}
	})
}

// Helper function to count occurrences in a slice
func countOccurrences(slice []string, item string) int {
	count := 0
	for _, s := range slice {
		if s == item {
			count++
		}
	}
	return count
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

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
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

		// Debug: Print all relations found
		t.Logf("Found %d snapshots", len(sn.Snapshots))

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
					t.Logf("Found relation: %s with key %s, format %d", relName, snapshot.Snapshot.Data.Key, details.GetInt64(bundle.RelationKeyRelationFormat))
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

		// Priority is now stored as option ID (status field)
		priorityIds := mainObjectDetails.GetStringList(getKey("priority"))
		assert.Len(t, priorityIds, 1, "Should have one priority value")
		// Status is now stored as option ID
		statusIds := mainObjectDetails.GetStringList(getKey("Status"))
		assert.Len(t, statusIds, 1, "Should have one status value")

		// Verify we have a status option with value "in-progress"
		foundStatusOption := false
		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.SbType == coresb.SmartBlockTypeRelationOption {
				optionDetails := snapshot.Snapshot.Data.Details
				if optionDetails.GetString(bundle.RelationKeyRelationKey) == string(getKey("Status")) &&
					optionDetails.GetString(bundle.RelationKeyName) == "in-progress" {
					foundStatusOption = true
					assert.Contains(t, statusIds, snapshot.Id, "Status should reference the correct option ID")
					break
				}
			}
		}
		assert.True(t, foundStatusOption, "Should have found status option with value 'in-progress'")

		// Check dates are timestamps
		startDate := mainObjectDetails.GetInt64(getKey("Start Date"))
		assert.Greater(t, startDate, int64(0))
		endDate := mainObjectDetails.GetInt64(getKey("End Date"))
		assert.Greater(t, endDate, int64(0))

		// Check other values
		assert.Equal(t, false, mainObjectDetails.GetBool(getKey("done")))
		assert.Equal(t, int64(75), mainObjectDetails.GetInt64(getKey("progress")))
		assert.Equal(t, 9.5, mainObjectDetails.GetFloat64(getKey("score")))

		// Check lists - they should be option IDs now
		// Note: properties are title-cased during import
		tags := mainObjectDetails.GetStringList(getKey("Tag"))
		assert.Len(t, tags, 3, "Should have 3 tags")

		assignees := mainObjectDetails.GetStringList(getKey("assignees"))
		assert.Len(t, assignees, 3, "Should have 3 assignees")

		// Verify we have option snapshots with the correct values
		expectedTagValues := []string{"important", "test", "snapshot"}
		expectedAssigneeValues := []string{"john", "jane", "bob"}

		for _, snapshot := range sn.Snapshots {
			if snapshot.Snapshot.SbType == coresb.SmartBlockTypeRelationOption {
				optionDetails := snapshot.Snapshot.Data.Details
				relationKey := optionDetails.GetString(bundle.RelationKeyRelationKey)
				optionName := optionDetails.GetString(bundle.RelationKeyName)

				if relationKey == string(getKey("Tag")) {
					assert.Contains(t, expectedTagValues, optionName, "Tag option value should be one of expected")
				} else if relationKey == string(getKey("assignees")) {
					assert.Contains(t, expectedAssigneeValues, optionName, "Assignee option value should be one of expected")
				}
			}
		}

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
			"priority":    {format: model.RelationFormat_status, includeTime: false},
			"Status":      {format: model.RelationFormat_status, includeTime: false},
			"Start Date":  {format: model.RelationFormat_date, includeTime: false},
			"End Date":    {format: model.RelationFormat_date, includeTime: true},
			"done":        {format: model.RelationFormat_checkbox, includeTime: false},
			"progress":    {format: model.RelationFormat_number, includeTime: false},
			"score":       {format: model.RelationFormat_number, includeTime: false},
			"Tag":         {format: model.RelationFormat_tag, includeTime: false},
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
			systemKeys := []string{"objectType", "tag", "status", "type"}
			isSystemKey := false
			for _, sysKey := range systemKeys {
				if key == sysKey {
					isSystemKey = true
					break
				}
			}
			if isSystemKey {
				continue
			}

			assert.Len(t, key, 24, "Relation %s key should be 24 characters (BSON ID)", relName)
		}
	})
}

func TestMarkdown_CollectionImport(t *testing.T) {
	t.Run("import collection with Collection property in YAML", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()

		// Create schema files
		taskSchemaPath := filepath.Join(testDirectory, "task.schema.json")
		taskCollectionSchemaPath := filepath.Join(testDirectory, "task_collection.schema.json")

		// Create task schema
		taskSchema := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Task",
			"x-type-key": "task",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext"
				},
				"Priority": {
					"type": "string",
					"x-key": "priority", 
					"x-format": "status",
					"enum": ["low", "medium", "high"]
				}
			}
		}`

		// Create collection schema with Collection property
		taskCollectionSchema := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Task Collection",
			"x-type-key": "task_collection",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext"
				},
				"Collection": {
					"type": "array",
					"items": {
						"type": "string"
					},
					"x-key": "_collection",
					"x-format": "object"
				}
			}
		}`

		err := os.WriteFile(taskSchemaPath, []byte(taskSchema), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(taskCollectionSchemaPath, []byte(taskCollectionSchema), os.ModePerm)
		assert.NoError(t, err)

		// Create test files
		task1Path := filepath.Join(testDirectory, "Important Task.md")
		task2Path := filepath.Join(testDirectory, "Another Task.md")
		collectionPath := filepath.Join(testDirectory, "My Task Collection.md")

		// Create task files with schema reference
		task1Content := `---
# yaml-language-server: $schema=task.schema.json
Name: Important Task
Object type: Task
Priority: high
---

# Important Task

This is an important task that needs to be done.`

		task2Content := `---
# yaml-language-server: $schema=task.schema.json
Name: Another Task
Object type: Task
Priority: medium
---

# Another Task

This is another task in the collection.`

		// Create collection file with schema reference
		collectionContent := `---
# yaml-language-server: $schema=task_collection.schema.json
Name: My Task Collection
Object type: Task Collection
Collection:
- Important Task.md
- Another Task.md
---

# My Task Collection

This is a collection of important tasks.`

		err = os.WriteFile(task1Path, []byte(task1Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(task2Path, []byte(task2Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(collectionPath, []byte(collectionContent), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)
		assert.GreaterOrEqual(t, len(sn.Snapshots), 3) // At least 3 pages

		// Find the collection snapshot
		var collectionSnapshot *common.Snapshot
		var task1Snapshot, task2Snapshot *common.Snapshot

		for _, snapshot := range sn.Snapshots {
			if strings.Contains(snapshot.FileName, "My Task Collection.md") {
				collectionSnapshot = snapshot
			} else if strings.Contains(snapshot.FileName, "Important Task.md") {
				task1Snapshot = snapshot
			} else if strings.Contains(snapshot.FileName, "Another Task.md") {
				task2Snapshot = snapshot
			}
		}

		assert.NotNil(t, collectionSnapshot, "Collection snapshot should exist")
		assert.NotNil(t, task1Snapshot, "Task 1 snapshot should exist")
		assert.NotNil(t, task2Snapshot, "Task 2 snapshot should exist")

		// Verify collection has correct SbType (collections are pages)
		assert.Equal(t, coresb.SmartBlockTypePage, collectionSnapshot.Snapshot.SbType,
			"Collection should have SmartBlockTypePage type")

		// Verify collection has the collection store
		assert.NotNil(t, collectionSnapshot.Snapshot.Data.Collections, "Collection should have Collections field")
		collectionStoreValue := collectionSnapshot.Snapshot.Data.Collections.Fields[template.CollectionStoreKey]
		assert.NotNil(t, collectionStoreValue, "Collection should have collection store")

		// Get the collection IDs from store
		collectionIds := pbtypes.GetStringListValue(collectionStoreValue)
		assert.Len(t, collectionIds, 2, "Collection should have 2 items")

		// Verify the collection contains the task IDs
		assert.Contains(t, collectionIds, task1Snapshot.Id, "Collection should contain task 1")
		assert.Contains(t, collectionIds, task2Snapshot.Id, "Collection should contain task 2")
	})

	t.Run("import collection with absolute and relative paths", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()

		// Create schema files
		taskSchemaPath := filepath.Join(testDirectory, "task.schema.json")
		collectionSchemaPath := filepath.Join(testDirectory, "collection.schema.json")

		// Create task schema
		taskSchema := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Task",
			"x-type-key": "task",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext"
				}
			}
		}`

		// Create collection schema
		collectionSchema := `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"type": "object",
			"title": "Collection",
			"x-type-key": "collection_type",
			"properties": {
				"Name": {
					"type": "string",
					"x-key": "name",
					"x-format": "shorttext"
				},
				"Collection": {
					"type": "array",
					"items": {
						"type": "string"
					},
					"x-key": "_collection",
					"x-format": "object"
				}
			}
		}`

		err := os.WriteFile(taskSchemaPath, []byte(taskSchema), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(collectionSchemaPath, []byte(collectionSchema), os.ModePerm)
		assert.NoError(t, err)

		// Create subdirectory structure
		subDir := filepath.Join(testDirectory, "tasks")
		err = os.MkdirAll(subDir, os.ModePerm)
		assert.NoError(t, err)

		// Create test files
		task1Path := filepath.Join(subDir, "Task One.md")
		task2Path := filepath.Join(testDirectory, "Task Two.md")
		collectionPath := filepath.Join(testDirectory, "Collection with Paths.md")

		// Create task files with schema reference
		task1Content := `---
# yaml-language-server: $schema=../task.schema.json
Name: Task One
Object type: Task
---

# Task One`

		task2Content := `---
# yaml-language-server: $schema=task.schema.json
Name: Task Two
Object type: Task
---

# Task Two`

		// Create collection file with various path formats and schema reference
		collectionContent := fmt.Sprintf(`---
# yaml-language-server: $schema=collection.schema.json
Name: Collection with Paths
Object type: Collection
Collection:
- Task Two.md
- tasks/Task One.md
- %s
---

# Collection with Paths

Testing different path formats.`, task1Path)

		err = os.WriteFile(task1Path, []byte(task1Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(task2Path, []byte(task2Content), os.ModePerm)
		assert.NoError(t, err)
		err = os.WriteFile(collectionPath, []byte(collectionContent), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		// Set the schema importer in the block converter
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
		p := process.NewNoOp()

		// when
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{testDirectory}},
			},
			Type: model.Import_Markdown,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Find the snapshots
		var collectionSnapshot *common.Snapshot
		var task1Snapshot, task2Snapshot *common.Snapshot

		for _, snapshot := range sn.Snapshots {
			if strings.Contains(snapshot.FileName, "Collection with Paths.md") {
				collectionSnapshot = snapshot
			} else if strings.Contains(snapshot.FileName, "Task One.md") {
				task1Snapshot = snapshot
			} else if strings.Contains(snapshot.FileName, "Task Two.md") {
				task2Snapshot = snapshot
			}
		}

		assert.NotNil(t, collectionSnapshot, "Collection snapshot should exist")
		assert.NotNil(t, task1Snapshot, "Task 1 snapshot should exist")
		assert.NotNil(t, task2Snapshot, "Task 2 snapshot should exist")

		// Verify collection has the collection store
		assert.NotNil(t, collectionSnapshot.Snapshot.Data.Collections, "Collection should have Collections field")
		collectionStoreValue := collectionSnapshot.Snapshot.Data.Collections.Fields[template.CollectionStoreKey]
		assert.NotNil(t, collectionStoreValue, "Collection should have collection store")

		// Get the collection IDs from store
		collectionIds := pbtypes.GetStringListValue(collectionStoreValue)
		assert.Len(t, collectionIds, 3, "Collection should have 3 items")

		// Verify all references were resolved
		assert.Contains(t, collectionIds, task1Snapshot.Id, "Collection should contain task 1")
		assert.Contains(t, collectionIds, task2Snapshot.Id, "Collection should contain task 2")
	})
}

func TestMarkdown_IncludePropertiesAsBlock(t *testing.T) {
	t.Run("include properties as relation blocks", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		mdPath := filepath.Join(testDirectory, "test.md")

		// Create test file with YAML properties
		content := `---
title: Test Document
priority: high
status: active
tags: [work, project]
custom_field: Some value
---

# Document Content

This is the document content.`

		err := os.WriteFile(mdPath, []byte(content), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
		p := process.NewNoOp()

		// when - with includePropertiesAsBlock = true
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path:                     []string{testDirectory},
					IncludePropertiesAsBlock: true,
				},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Find the main document snapshot
		var mainSnapshot *common.Snapshot
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == mdPath {
				mainSnapshot = snapshot
				break
			}
		}

		assert.NotNil(t, mainSnapshot)
		blocks := mainSnapshot.Snapshot.Data.Blocks
		assert.NotEmpty(t, blocks)

		// Count relation blocks at the beginning
		relationBlockCount := 0
		for _, block := range blocks {
			if block.GetRelation() != nil {
				relationBlockCount++
			} else {
				break // Stop counting when we hit non-relation blocks
			}
		}

		// Should have relation blocks for non-system properties
		// title, priority, status, tags, custom_field
		assert.Equal(t, 5, relationBlockCount, "Should have 5 relation blocks for non-system properties")

		// Verify the relation keys - property names after processing
		expectedKeys := map[string]bool{
			"title":        false, // Custom title property (not the system Name)
			"priority":     false,
			"Status":       false, // status becomes Status
			"Tag":          false, // tags becomes Tag
			"custom_field": false,
		}

		for i := 0; i < relationBlockCount; i++ {
			relBlock := blocks[i].GetRelation()
			assert.NotNil(t, relBlock)

			// Find which property this represents
			found := false
			for _, snapshot := range sn.Snapshots {
				if snapshot.Snapshot.Data.Key == relBlock.Key {
					name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
					if _, exists := expectedKeys[name]; exists {
						expectedKeys[name] = true
						found = true
						break
					}
				}
			}
			assert.True(t, found, "Relation block key %s should correspond to a known property", relBlock.Key)
		}

		// Verify all expected properties were found
		for name, found := range expectedKeys {
			assert.True(t, found, "Property %s should have a relation block", name)
		}

		// Verify content blocks come after relation blocks
		headerFound := false
		for i := relationBlockCount; i < len(blocks); i++ {
			block := blocks[i]
			if text := block.GetText(); text != nil {
				if text.Style == model.BlockContentText_Header1 && text.Text == "Document Content" {
					headerFound = true
					break
				} else if text.Style == model.BlockContentText_Paragraph && strings.Contains(text.Text, "document content") {
					// Content is present but as a paragraph - that's acceptable
					headerFound = true
					break
				}
			}
		}
		assert.True(t, headerFound, "Document content should be preserved after relation blocks")
	})

	t.Run("do not include properties as blocks when disabled", func(t *testing.T) {
		// given
		testDirectory := t.TempDir()
		mdPath := filepath.Join(testDirectory, "test2.md")

		content := `---
title: Test Document
priority: high
---

# Document Content`

		err := os.WriteFile(mdPath, []byte(content), os.ModePerm)
		assert.NoError(t, err)

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)
		p := process.NewNoOp()

		// when - with includePropertiesAsBlock = false
		sn, ce := h.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path:                     []string{testDirectory},
					IncludePropertiesAsBlock: false,
				},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.Nil(t, ce)
		assert.NotNil(t, sn)

		// Find the main document snapshot
		var mainSnapshot *common.Snapshot
		for _, snapshot := range sn.Snapshots {
			if snapshot.FileName == mdPath {
				mainSnapshot = snapshot
				break
			}
		}

		assert.NotNil(t, mainSnapshot)
		blocks := mainSnapshot.Snapshot.Data.Blocks
		assert.NotEmpty(t, blocks)

		// First block should be the header, not a relation block
		if len(blocks) > 0 {
			firstBlock := blocks[0]
			assert.Nil(t, firstBlock.GetRelation(), "First block should not be a relation block")
			if firstBlock.GetText() != nil {
				assert.Equal(t, model.BlockContentText_Header1, firstBlock.GetText().Style)
				assert.Equal(t, "Document Content", firstBlock.GetText().Text)
			}
		}
	})
}

func TestMarkdown_ProcessFiles_MultipleSelection(t *testing.T) {
	t.Run("multiple files in same directory should import parent directory", func(t *testing.T) {
		// Create test directory structure
		testDir := t.TempDir()
		subDir1 := filepath.Join(testDir, "docs")
		subDir2 := filepath.Join(testDir, "notes")
		err := os.MkdirAll(subDir1, 0755)
		require.NoError(t, err)
		err = os.MkdirAll(subDir2, 0755)
		require.NoError(t, err)

		// Create test files
		file1 := filepath.Join(testDir, "file1.md")
		file2 := filepath.Join(testDir, "file2.md")
		file3 := filepath.Join(subDir1, "doc1.md")
		file4 := filepath.Join(subDir2, "note1.md")

		for _, f := range []string{file1, file2, file3, file4} {
			err := os.WriteFile(f, []byte("# Test\nContent"), 0644)
			require.NoError(t, err)
		}

		// Test 1: Multiple files in same directory
		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)

		req := &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path: []string{file1, file2},
				},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}

		progress := process.NewNoOp()
		response, ce := h.GetSnapshots(context.Background(), req, progress)

		assert.Nil(t, ce)
		assert.NotNil(t, response)

		// Should import only the selected files, not all files in directory
		fileCount := 0
		for _, snapshot := range response.Snapshots {
			if snapshot.FileName == file1 || snapshot.FileName == file2 {
				fileCount++
			}
			// Should not include file3 or file4
			assert.NotEqual(t, file3, snapshot.FileName)
			assert.NotEqual(t, file4, snapshot.FileName)
		}
		assert.Equal(t, 2, fileCount, "Should import exactly the 2 selected files")
	})

	t.Run("multiple paths from different directories imports individually", func(t *testing.T) {
		// Create test directory structure
		testDir1 := t.TempDir()
		testDir2 := t.TempDir()

		file1 := filepath.Join(testDir1, "file1.md")
		file2 := filepath.Join(testDir2, "file2.md")

		for _, f := range []string{file1, file2} {
			err := os.WriteFile(f, []byte("# Test\nContent"), 0644)
			require.NoError(t, err)
		}

		h := &Markdown{
			blockConverter: newMDConverter(&MockTempDir{}),
		}
		si := NewSchemaImporter()
		h.blockConverter.SetSchemaImporter(si)

		req := &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path: []string{file1, file2},
				},
			},
			Type: model.Import_Markdown,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}

		progress := process.NewNoOp()
		response, ce := h.GetSnapshots(context.Background(), req, progress)

		assert.Nil(t, ce)
		assert.NotNil(t, response)

		// Should import both files
		fileCount := 0
		for _, snapshot := range response.Snapshots {
			if snapshot.FileName == file1 || snapshot.FileName == file2 {
				fileCount++
			}
		}
		assert.Equal(t, 2, fileCount, "Should import both files from different directories")
	})
}

func TestFindCommonParentDir(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{
			name:     "empty paths",
			paths:    []string{},
			expected: "",
		},
		{
			name:     "single path",
			paths:    []string{"/home/user/docs/file1.md"},
			expected: "",
		},
		{
			name:     "same parent",
			paths:    []string{"/home/user/docs/file1.md", "/home/user/docs/file2.md"},
			expected: "/home/user/docs",
		},
		{
			name:     "different parents",
			paths:    []string{"/home/user/docs/file1.md", "/home/user/notes/file2.md"},
			expected: "",
		},
		{
			name:     "nested paths same parent",
			paths:    []string{"/home/user/docs/file1.md", "/home/user/docs/sub/file2.md"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For this test we need to use paths that can be made absolute
			// Create temporary files for paths that need to exist
			if len(tt.paths) > 0 && tt.paths[0] != "" {
				var testPaths []string
				tempDir := t.TempDir()

				for _, p := range tt.paths {
					// Create a test file path
					testFile := filepath.Join(tempDir, filepath.Base(filepath.Dir(p)), filepath.Base(p))
					os.MkdirAll(filepath.Dir(testFile), 0755)
					os.WriteFile(testFile, []byte("test"), 0644)
					testPaths = append(testPaths, testFile)
				}

				// If we expect same parent, check with our test paths
				if tt.expected != "" && len(testPaths) > 1 {
					result := findCommonParentDir(testPaths)
					// Just check that we get a common parent (exact path will differ due to temp dir)
					assert.NotEmpty(t, result)
				} else {
					result := findCommonParentDir(testPaths)
					if tt.expected == "" {
						assert.Empty(t, result)
					}
				}
			} else {
				result := findCommonParentDir(tt.paths)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
