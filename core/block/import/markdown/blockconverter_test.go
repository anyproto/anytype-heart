package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type MockTempDir struct{}

func (m MockTempDir) TempDir() string {
	return os.TempDir()
}

func Test_processFiles(t *testing.T) {
	t.Run("imported directory include mov and pdf files - md file has file blocks", func(t *testing.T) {
		// given
		converter := newMDConverter(&MockTempDir{})
		pdfFile := filepath.Join("testdata", "test.pdf")
		_, err := os.Create(pdfFile)
		assert.Nil(t, err)
		defer os.Remove(pdfFile)

		movFile := filepath.Join("testdata", "test.mov")
		_, err = os.Create(movFile)
		assert.Nil(t, err)
		defer os.Remove(movFile)

		workingDir, err := os.Getwd()
		absolutePath := filepath.Join(workingDir, "testdata")
		source := source.GetSource(absolutePath)

		// Initialize the source with the path
		err = source.Initialize(absolutePath)
		assert.Nil(t, err)

		// when
		files := converter.processFiles(absolutePath, common.NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS), source)

		// then
		assert.Len(t, files, 22)

		pdfFilePath := filepath.Join(absolutePath, "test.pdf")
		assert.Contains(t, files, pdfFilePath)

		movFilePath := filepath.Join(absolutePath, "test.mov")
		assert.Contains(t, files, movFilePath)

		mdFilePath := filepath.Join(absolutePath, "test.md")
		assert.Contains(t, files, mdFilePath)

		fileBlocks := lo.Filter(files[mdFilePath].ParsedBlocks, func(item *model.Block, index int) bool {
			return item.GetFile() != nil
		})

		assert.Len(t, fileBlocks, 2)
		assert.Equal(t, pdfFilePath, fileBlocks[0].GetFile().Name)
		assert.Equal(t, movFilePath, fileBlocks[1].GetFile().Name)
	})

	t.Run("imported directory include without mov and pdf files - no file blocks", func(t *testing.T) {
		// given
		converter := newMDConverter(&MockTempDir{})
		workingDir, err := os.Getwd()
		assert.Nil(t, err)
		absolutePath := filepath.Join(workingDir, "testdata")
		source := source.GetSource(absolutePath)

		// Initialize the source with the path
		err = source.Initialize(absolutePath)
		assert.Nil(t, err)

		// when
		files := converter.processFiles(absolutePath, common.NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS), source)

		// then
		assert.Len(t, files, 20)

		pdfFilePath := filepath.Join(absolutePath, "test.pdf")
		assert.NotContains(t, files, pdfFilePath)

		movFilePath := filepath.Join(absolutePath, "test.mov")
		assert.NotContains(t, files, movFilePath)

		mdFilePath := filepath.Join(absolutePath, "test.md")
		assert.Contains(t, files, mdFilePath)

		fileBlocks := lo.Filter(files[mdFilePath].ParsedBlocks, func(item *model.Block, index int) bool {
			return item.GetFile() != nil
		})

		assert.Len(t, fileBlocks, 0)
	})
}

func TestCreateDirectoryPages_EmptyRootName(t *testing.T) {
	// Test that empty root directory names fallback to rootCollectionName
	converter := newMDConverter(&MockTempDir{})

	// Create test files
	files := map[string]*FileInfo{
		"doc1.md":        {OriginalPath: "doc1.md", Title: "Document 1"},
		"subdir/doc2.md": {OriginalPath: "subdir/doc2.md", Title: "Document 2"},
	}

	// Test with empty root path (simulating zip import)
	converter.createDirectoryPages("", files)

	// Check that root directory page was created with fallback name
	rootPage, exists := files[""]
	assert.True(t, exists, "Root directory page should exist")
	assert.Equal(t, rootCollectionName, rootPage.Title, "Empty root should use rootCollectionName")
	assert.True(t, rootPage.IsRootDirPage, "Root directory page should have IsRootDirPage set")

	// Test with "." as root path
	files2 := map[string]*FileInfo{
		"doc1.md": {OriginalPath: "doc1.md", Title: "Document 1"},
	}
	converter.createDirectoryPages(".", files2)

	rootPage2, exists := files2[""]
	assert.True(t, exists, "Root directory page should exist for '.'")
	assert.Equal(t, rootCollectionName, rootPage2.Title, "Root '.' should use rootCollectionName")
}

func TestCreateDirectoryPages_SkipHiddenDirectories(t *testing.T) {
	// Test that directories starting with "." are skipped
	converter := newMDConverter(&MockTempDir{})

	// Create test files including hidden directories
	files := map[string]*FileInfo{
		"doc1.md":                     {OriginalPath: "doc1.md", Title: "Document 1"},
		".hidden/doc2.md":             {OriginalPath: ".hidden/doc2.md", Title: "Document 2"},
		".git/config":                 {OriginalPath: ".git/config", Title: "Git Config"},
		"visible/doc3.md":             {OriginalPath: "visible/doc3.md", Title: "Document 3"},
		"visible/.obsidian/workspace": {OriginalPath: "visible/.obsidian/workspace", Title: "Obsidian Workspace"},
	}

	// Create directory pages
	converter.createDirectoryPages("", files)

	// Check that hidden directories were not created
	_, hiddenExists := files[".hidden"]
	assert.False(t, hiddenExists, "Hidden directory .hidden should not have a page")

	_, gitExists := files[".git"]
	assert.False(t, gitExists, "Hidden directory .git should not have a page")

	_, obsidianExists := files["visible/.obsidian"]
	assert.False(t, obsidianExists, "Hidden subdirectory .obsidian should not have a page")

	// Check that visible directory was created
	visiblePage, visibleExists := files["visible"]
	assert.True(t, visibleExists, "Visible directory should have a page")

	// Check that visible directory doesn't contain links to hidden subdirectories
	if visibleExists {
		hasHiddenLink := false
		for _, block := range visiblePage.ParsedBlocks {
			if link := block.GetLink(); link != nil {
				if link.TargetBlockId == "visible/.obsidian" {
					hasHiddenLink = true
					break
				}
			}
		}
		assert.False(t, hasHiddenLink, "Visible directory should not contain links to hidden subdirectories")
	}

	// Check that root directory page exists but doesn't contain hidden directories
	rootPage, rootExists := files[""]
	assert.True(t, rootExists, "Root directory page should exist")

	if rootExists {
		hiddenLinkCount := 0
		for _, block := range rootPage.ParsedBlocks {
			if link := block.GetLink(); link != nil {
				if strings.HasPrefix(filepath.Base(link.TargetBlockId), ".") {
					hiddenLinkCount++
				}
			}
		}
		assert.Equal(t, 0, hiddenLinkCount, "Root directory should not contain links to hidden directories")
	}
}

func TestCreateDirectoryPages_PathImport(t *testing.T) {
	// Test Example 1: Regular path import
	converter := newMDConverter(&MockTempDir{})

	// Create test files matching Example 1
	files := map[string]*FileInfo{
		"/home/links/.obsidian/app.json": {
			OriginalPath: "/home/links/.obsidian/app.json",
			Title:        "app",
		},
		"/home/links/.obsidian/workspace.json": {
			OriginalPath: "/home/links/.obsidian/workspace.json",
			Title:        "workspace",
		},
		"/home/links/01. Test.md": {
			OriginalPath: "/home/links/01. Test.md",
			Title:        "01. Test",
		},
		"/home/links/Z.md": {
			OriginalPath: "/home/links/Z.md",
			Title:        "Index",
		},
		"/home/links/X.md": {
			OriginalPath: "/home/links/X.md",
			Title:        "Рецепты",
		},
		"/home/links/Y.md": {
			OriginalPath: "/home/links/Y.md",
			Title:        "Стейки",
		},
	}

	// Create directory pages
	converter.createDirectoryPages("/home/links", files)

	// Check that root directory page was created
	rootPage, exists := files["/home/links"]
	assert.True(t, exists, "Root directory page should exist")
	assert.Equal(t, "links", rootPage.Title)
	assert.True(t, rootPage.IsRootDirPage)

	// Check that .obsidian directory was NOT created
	_, obsidianExists := files["/home/links/.obsidian"]
	assert.False(t, obsidianExists, "Hidden .obsidian directory should not have a page")

	// Check root page has links to markdown files but not to hidden directories
	assert.True(t, len(rootPage.ParsedBlocks) > 0, "Root should have links")

	// Count links to markdown files
	mdLinkCount := 0
	for _, block := range rootPage.ParsedBlocks {
		if link := block.GetLink(); link != nil {
			target := link.TargetBlockId
			if strings.HasSuffix(target, ".md") {
				mdLinkCount++
			}
			// Ensure no links to hidden directories
			assert.False(t, strings.Contains(target, ".obsidian"), "Should not link to hidden directories")
		}
	}
	assert.Equal(t, 4, mdLinkCount, "Should have 4 markdown file links")
}

func TestCreateDirectoryPages_ZipImport(t *testing.T) {
	// Test Example 2: Zip import with single root directory
	converter := newMDConverter(&MockTempDir{})

	// Create test files matching Example 2
	files := map[string]*FileInfo{
		"links/Index.md": {
			OriginalPath: "links/Index.md",
			Title:        "Index",
		},
		"links/.obsidian/graph.json": {
			OriginalPath: "links/.obsidian/graph.json",
			Title:        "graph",
		},
		"links/.obsidian/workspace.json": {
			OriginalPath: "links/.obsidian/workspace.json",
			Title:        "workspace",
		},
		"links/01. Test.md": {
			OriginalPath: "links/01. Test.md",
			Title:        "01. Test",
		},
		"links/Стейки.md": {
			OriginalPath: "links/Стейки.md",
			Title:        "Стейки",
		},
		"links/Рецепты.md": {
			OriginalPath: "links/Рецепты.md",
			Title:        "Рецепты",
		},
	}

	// Create directory pages
	converter.createDirectoryPages("/home/links.zip", files)

	// Check that the single "links" directory was treated as root
	// The root page should be created at "" (empty path)
	rootPage, exists := files[""]
	assert.True(t, exists, "Root directory page should exist at empty path")
	assert.Equal(t, rootCollectionName, rootPage.Title, "Root should use rootCollectionName")
	assert.True(t, rootPage.IsRootDirPage)

	// Check that .obsidian directory was NOT created
	_, obsidianExists := files["links/.obsidian"]
	assert.False(t, obsidianExists, "Hidden .obsidian directory should not have a page")

	// The original "links" directory should not exist as a separate page
	_, linksExists := files["links"]
	assert.False(t, linksExists, "Single root 'links' directory should be omitted")

	// Check root page has links to markdown files
	assert.True(t, len(rootPage.ParsedBlocks) > 0, "Root should have links")

	// Count links to markdown files
	mdLinkCount := 0
	for _, block := range rootPage.ParsedBlocks {
		if link := block.GetLink(); link != nil {
			target := link.TargetBlockId
			if strings.HasSuffix(target, ".md") {
				mdLinkCount++
			}
			// Ensure no links to hidden directories
			assert.False(t, strings.Contains(target, ".obsidian"), "Should not link to hidden directories")
		}
	}
	assert.Equal(t, 4, mdLinkCount, "Should have 4 markdown file links")
}

func TestCreateDirectoryPages_NestedDirectories(t *testing.T) {
	// Test with nested directory structure
	converter := newMDConverter(&MockTempDir{})

	// Create test files with nested structure
	files := map[string]*FileInfo{
		"root/index.md": {
			OriginalPath: "root/index.md",
			Title:        "Index",
		},
		"root/docs/guide.md": {
			OriginalPath: "root/docs/guide.md",
			Title:        "Guide",
		},
		"root/docs/api/reference.md": {
			OriginalPath: "root/docs/api/reference.md",
			Title:        "API Reference",
		},
		"root/.config/settings.json": {
			OriginalPath: "root/.config/settings.json",
			Title:        "Settings",
		},
	}

	// Create directory pages
	converter.createDirectoryPages("", files)

	// Check that root was properly handled
	rootPage, exists := files[""]
	assert.True(t, exists, "Root directory page should exist")
	assert.Equal(t, rootCollectionName, rootPage.Title)
	assert.True(t, rootPage.IsRootDirPage)

	// Check that nested directories were created
	_, docsExists := files["docs"]
	assert.True(t, docsExists, "docs directory page should exist")

	_, apiExists := files["docs/api"]
	assert.True(t, apiExists, "docs/api directory page should exist")

	// Check that hidden directory was not created
	_, configExists := files[".config"]
	assert.False(t, configExists, ".config hidden directory should not have a page")
}

func TestFindCommonPrefix(t *testing.T) {
	tests := []struct {
		name     string
		path1    string
		path2    string
		expected string
	}{
		{
			name:     "same directory",
			path1:    "/home/links",
			path2:    "/home/links",
			expected: "/home/links",
		},
		{
			name:     "parent and child",
			path1:    "/home",
			path2:    "/home/links",
			expected: "/home",
		},
		{
			name:     "siblings",
			path1:    "/home/links",
			path2:    "/home/docs",
			expected: "/home",
		},
		{
			name:     "no common prefix",
			path1:    "/Users/roman",
			path2:    "/home/user",
			expected: "",
		},
		{
			name:     "relative paths",
			path1:    "links/docs",
			path2:    "links/api",
			expected: "links",
		},
		{
			name:     "empty paths",
			path1:    "",
			path2:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCommonPrefix(tt.path1, tt.path2)
			assert.Equal(t, tt.expected, result)
		})
	}
}
