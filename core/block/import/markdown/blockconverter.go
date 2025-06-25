package markdown

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
	"github.com/anyproto/anytype-heart/util/uri"
)

type mdConverter struct {
	tempDirProvider core.TempDirProvider
	schemaImporter  *SchemaImporter       // Optional schema importer for property resolution
	yamlResolver    *YAMLPropertyResolver // Resolver for consistent property keys when no schema
}

type FileInfo struct {
	os.FileInfo
	HasInboundLinks       bool
	PageID                string
	IsRootFile            bool
	Title                 string
	TitleUnique           string
	ParsedBlocks          []*model.Block
	CollectionsObjectsIds []string
	YAMLDetails           *domain.Details
	YAMLProperties        []yaml.Property
	ObjectTypeName        string // Name of the object type from YAML "type" property
}

func newMDConverter(tempDirProvider core.TempDirProvider) *mdConverter {
	return &mdConverter{
		tempDirProvider: tempDirProvider,
		yamlResolver:    NewYAMLPropertyResolver(),
	}
}

// SetSchemaImporter sets the schema importer for property resolution
func (m *mdConverter) SetSchemaImporter(si *SchemaImporter) {
	m.schemaImporter = si
}

// GetYAMLResolver returns the YAML property resolver
func (m *mdConverter) GetYAMLResolver() *YAMLPropertyResolver {
	return m.yamlResolver
}

func (m *mdConverter) markdownToBlocks(importPath string, importSource source.Source, allErrors *common.ConvertError, createDirectoryPages bool) map[string]*FileInfo {
	files := m.processFiles(importPath, allErrors, importSource)

	// Create directory pages if requested
	if createDirectoryPages {
		m.createDirectoryPages(importPath, files)
	}

	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	return files
}

func (m *mdConverter) processFiles(importPath string, allErrors *common.ConvertError, importSource source.Source) map[string]*FileInfo {
	if importSource.CountFilesWithGivenExtensions([]string{".md"}) == 0 {
		allErrors.Add(common.ErrorBySourceType(importSource))
		return nil
	}
	fileInfo := m.getFileInfo(importSource, allErrors)
	for name, file := range fileInfo {
		m.processBlocks(name, file, fileInfo, importSource)
		for _, b := range file.ParsedBlocks {
			m.processFileBlock(b, importSource, importPath)
		}
	}
	return fileInfo
}

func (m *mdConverter) getFileInfo(importSource source.Source, allErrors *common.ConvertError) map[string]*FileInfo {
	fileInfo := make(map[string]*FileInfo, 0)
	if iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		if err := m.fillFilesInfo(importSource, fileInfo, fileName, fileReader); err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(0, model.Import_Markdown) {
				return false
			}
		}
		return true
	}); iterateErr != nil {
		allErrors.Add(iterateErr)
	}
	return fileInfo
}

func (m *mdConverter) fillFilesInfo(importSource source.Source, fileInfo map[string]*FileInfo, path string, rc io.ReadCloser) error {
	fileInfo[path] = &FileInfo{}
	if err := m.createBlocksFromFile(importSource, path, rc, fileInfo); err != nil {
		log.Errorf("failed to create blocks from file: %s", err)
		return err
	}
	return nil
}

func (m *mdConverter) processBlocks(shortPath string, file *FileInfo, files map[string]*FileInfo, importSource source.Source) {
	for _, block := range file.ParsedBlocks {
		m.processTextBlock(block, files, importSource)
	}
	m.processLinkBlock(shortPath, file, files)
}

func (m *mdConverter) processTextBlock(block *model.Block, files map[string]*FileInfo, importSource source.Source) {
	txt := block.GetText()
	if txt != nil && txt.Marks != nil {
		if len(txt.Marks.Marks) == 1 && txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {
			m.handleSingleMark(block, files, importSource)
		} else {
			m.handleMultipleMarks(block, files, importSource)
		}
	}
}

func (m *mdConverter) handleSingleMark(block *model.Block, files map[string]*FileInfo, importSource source.Source) {
	txt := block.GetText()
	wholeLineLink := m.isWholeLineLink(txt.Text, txt.Marks.Marks[0])
	ext := filepath.Ext(txt.Marks.Marks[0].Param)
	link := m.getOriginalName(txt.Marks.Marks[0].Param, importSource)
	if file := files[link]; file != nil {
		if strings.EqualFold(ext, ".csv") {
			txt.Marks.Marks[0].Param = link
			m.processCSVFileLink(block, files, link, wholeLineLink)
			return
		}
		if strings.EqualFold(ext, ".md") {
			// only convert if this is the only link in the row
			txt.Marks.Marks[0].Param = link
			m.convertToAnytypeLinkBlock(block, wholeLineLink)
		} else {
			block.Content = anymark.ConvertTextToFile(txt.Marks.Marks[0].Param)
		}
		file.HasInboundLinks = true
	} else if wholeLineLink {
		m.convertTextToBookmark(txt.Marks.Marks[0].Param, block)
	}
}

func (m *mdConverter) handleMultipleMarks(block *model.Block, files map[string]*FileInfo, importSource source.Source) {
	txt := block.GetText()
	for _, mark := range txt.Marks.Marks {
		if mark.Type == model.BlockContentTextMark_Link {
			if stop := m.handleSingleLinkMark(block, files, mark, txt, importSource); stop {
				return
			}
		}
	}
}

func (m *mdConverter) handleSingleLinkMark(block *model.Block, files map[string]*FileInfo, mark *model.BlockContentTextMark, txt *model.BlockContentText, importSource source.Source) bool {
	isWholeLink := m.isWholeLineLink(txt.Text, mark)
	link := m.getOriginalName(mark.Param, importSource)
	ext := filepath.Ext(link)
	if file := files[link]; file != nil {
		file.HasInboundLinks = true
		if strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".csv") {
			mark.Type = model.BlockContentTextMark_Mention
			mark.Param = link
			return false
		}
		if isWholeLink {
			block.Content = anymark.ConvertTextToFile(mark.Param)
			return true
		}
	} else if isWholeLink {
		m.convertTextToBookmark(mark.Param, block)
		return true
	}
	return false
}

func (m *mdConverter) isWholeLineLink(text string, marks *model.BlockContentTextMark) bool {
	var wholeLineLink bool
	textRunes := []rune(text)
	var from, to = int(marks.Range.From), int(marks.Range.To)
	if from == 0 || (from < len(textRunes) && len(strings.TrimSpace(string(textRunes[0:from]))) == 0) {
		if to >= len(textRunes) || len(strings.TrimSpace(string(textRunes[to:]))) == 0 {
			wholeLineLink = true
		}
	}
	return wholeLineLink
}

func (m *mdConverter) convertToAnytypeLinkBlock(block *model.Block, wholeLineLink bool) {
	if wholeLineLink {
		m.convertTextToPageLink(block)
	} else {
		m.convertTextToPageMention(block)
	}
}

func (m *mdConverter) processCSVFileLink(block *model.Block, files map[string]*FileInfo, link string, wholeLineLink bool) {
	csvDir := strings.TrimSuffix(link, ".csv")
	for name, file := range files {
		// set HasInboundLinks for all CSV-origin md files
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.HasInboundLinks = true
		}
	}
	m.convertToAnytypeLinkBlock(block, wholeLineLink)
	files[link].HasInboundLinks = true
}

func (m *mdConverter) processFileBlock(block *model.Block, importedSource source.Source, importPath string) {
	if f := block.GetFile(); f != nil {
		if block.Id == "" {
			block.Id = bson.NewObjectId().Hex()
		}
		name, _, err := common.ProvideFileName(block.GetFile().Name, importedSource, importPath, m.tempDirProvider)
		if err != nil {
			log.Errorf("failed to update file block, %v", err)
		}
		block.GetFile().Name = name
	}
}

func (m *mdConverter) processLinkBlock(shortPath string, file *FileInfo, files map[string]*FileInfo) {
	ext := filepath.Ext(shortPath)
	if !strings.EqualFold(ext, ".csv") {
		return
	}
	dependentFilesDir := strings.TrimSuffix(shortPath, ext)
	for targetName, targetFile := range files {
		fileExt := filepath.Ext(targetName)
		if filepath.Dir(targetName) == dependentFilesDir && strings.EqualFold(fileExt, ".md") {
			if !targetFile.HasInboundLinks {
				file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
					Id: bson.NewObjectId().Hex(),
					Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
						TargetBlockId: targetName,
						Style:         model.BlockContentLink_Page,
					}},
				})
				targetFile.HasInboundLinks = true
			}
		}
	}
}

func (m *mdConverter) convertTextToPageLink(block *model.Block) {
	block.Content = &model.BlockContentOfLink{
		Link: &model.BlockContentLink{
			TargetBlockId: block.GetText().Marks.Marks[0].Param,
			Style:         model.BlockContentLink_Page,
		},
	}
}

func (m *mdConverter) convertTextToBookmark(url string, block *model.Block) {
	if err := uri.ValidateURI(url); err != nil {
		return
	}

	block.Content = &model.BlockContentOfBookmark{
		Bookmark: &model.BlockContentBookmark{
			Url: url,
		},
	}
}

func (m *mdConverter) convertTextToPageMention(block *model.Block) {
	for _, mark := range block.GetText().Marks.Marks {
		if mark.Type != model.BlockContentTextMark_Link {
			continue
		}
		mark.Type = model.BlockContentTextMark_Mention
	}
}

func (m *mdConverter) createBlocksFromFile(importSource source.Source, filePath string, f io.ReadCloser, files map[string]*FileInfo) error {
	if importSource.IsRootFile(filePath) {
		files[filePath].IsRootFile = true
	}
	if filepath.Ext(filePath) == ".md" {
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}

		// Extract and parse YAML front matter
		frontMatter, markdownContent, err := yaml.ExtractYAMLFrontMatter(b)
		if err != nil {
			log.Warnf("failed to extract YAML front matter: %s", err)
			// Continue with original content
			markdownContent = b
		}

		// Parse YAML front matter if present
		if len(frontMatter) > 0 {
			var yamlResult *yaml.ParseResult
			var err error

			// Get base directory of the file for relative path resolution
			baseDir := filepath.Dir(filePath)

			// Use appropriate resolver based on schema availability
			if m.schemaImporter != nil && m.schemaImporter.HasSchemas() {
				// Use schema importer as resolver
				yamlResult, err = yaml.ParseYAMLFrontMatterWithResolverAndPath(frontMatter, m.schemaImporter, baseDir)
			} else {
				// Use YAML resolver for consistent property keys across files
				yamlResult, err = yaml.ParseYAMLFrontMatterWithResolverAndPath(frontMatter, m.yamlResolver, baseDir)
			}

			if err != nil {
				log.Warnf("failed to parse YAML front matter: %s", err)
			} else if yamlResult != nil {
				files[filePath].YAMLDetails = yamlResult.Details
				files[filePath].YAMLProperties = yamlResult.Properties
				files[filePath].ObjectTypeName = yamlResult.ObjectType
			}
		}

		files[filePath].ParsedBlocks, _, err = anymark.MarkdownToBlocks(markdownContent, filepath.Dir(filePath), nil)
		if err != nil {
			log.Errorf("failed to read blocks: %s", err)
		}
	}
	return nil
}

func (m *mdConverter) getOriginalName(link string, importSource source.Source) string {
	if originalFileNameGetter, ok := importSource.(source.OriginalFileNameGetter); ok {
		return originalFileNameGetter.GetFileOriginalName(link)
	}
	return link
}

// createDirectoryPages creates a page for each directory level (except root) in the import
// Each directory page contains block links to nested pages and subdirectories
func (m *mdConverter) createDirectoryPages(rootPath string, files map[string]*FileInfo) {
	// Build directory structure
	dirStructure := make(map[string][]string) // dir path -> list of direct children (files and subdirs)
	dirPages := make(map[string]*FileInfo)    // dir path -> FileInfo for directory page

	// First, collect all directories and their contents
	for filePath := range files {
		// Include all markdown files, regardless of whether they have PageID yet
		if filepath.Ext(filePath) != ".md" && filepath.Ext(filePath) != ".csv" {
			continue // Skip non-markdown/csv files
		}

		// Get the directory of this file
		dir := filepath.Dir(filePath)

		// Add this file to its parent directory's children
		dirStructure[dir] = append(dirStructure[dir], filePath)

		// Create parent directories up to but not including root
		for dir != "." && dir != "/" && dir != rootPath {
			parentDir := filepath.Dir(dir)
			// Check if we already have this directory in the structure
			found := false
			for _, child := range dirStructure[parentDir] {
				if child == dir {
					found = true
					break
				}
			}
			if !found {
				dirStructure[parentDir] = append(dirStructure[parentDir], dir)
			}
			dir = parentDir
		}
	}

	// Now create directory pages for each directory (except root)
	for dirPath := range dirStructure {
		if dirPath == "." || dirPath == "/" || dirPath == rootPath {
			continue // Skip root directory
		}

		// Check if a directory page already exists (shouldn't happen but just in case)
		if _, exists := dirPages[dirPath]; exists {
			continue
		}

		// Create a new directory page
		dirName := filepath.Base(dirPath)

		// Create blocks for children
		var blocks []*model.Block

		// Sort children for consistent ordering (directories first, then files)
		children := dirStructure[dirPath]
		sort.Slice(children, func(i, j int) bool {
			// Check if items are directories
			iIsDir := false
			jIsDir := false
			if _, exists := dirStructure[children[i]]; exists {
				iIsDir = true
			}
			if _, exists := dirStructure[children[j]]; exists {
				jIsDir = true
			}

			// Directories come first
			if iIsDir && !jIsDir {
				return true
			}
			if !iIsDir && jIsDir {
				return false
			}

			// Otherwise sort alphabetically
			return children[i] < children[j]
		})

		// Add links to children
		for _, childPath := range children {
			var linkBlock *model.Block

			// Check if this is a directory
			if _, isDir := dirStructure[childPath]; isDir {
				// This is a subdirectory - link to its directory page
				linkBlock = &model.Block{
					Id: bson.NewObjectId().Hex(),
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							CardStyle:     model.BlockContentLink_Inline,
							IconSize:      model.BlockContentLink_SizeSmall, // Mark as temporary link
							TargetBlockId: childPath,                        // Use file path as temporary ID
							Style:         model.BlockContentLink_Page,
							Fields:        nil,
						},
					},
				}
			} else {
				// This is a file - link to the page using file path as temporary ID
				if file, exists := files[childPath]; exists {
					fileName := file.Title
					if fileName == "" {
						fileName = filepath.Base(childPath)
						// Remove .md extension for display
						if strings.HasSuffix(fileName, ".md") {
							fileName = fileName[:len(fileName)-3]
						}
					}

					linkBlock = &model.Block{
						Id: bson.NewObjectId().Hex(),
						Content: &model.BlockContentOfLink{
							Link: &model.BlockContentLink{
								TargetBlockId: childPath, // Use file path as temporary ID
								Style:         model.BlockContentLink_Page,
								CardStyle:     model.BlockContentLink_Card,
								Description:   model.BlockContentLink_Content,
								IconSize:      model.BlockContentLink_SizeMedium, // Mark as temporary link
							},
						},
					}
				}
			}

			if linkBlock != nil {
				blocks = append(blocks, linkBlock)
			}
		}

		// Create the directory page FileInfo
		dirFile := &FileInfo{
			Title:           dirName,
			ParsedBlocks:    blocks,
			HasInboundLinks: true,                // Mark as having inbound links so it doesn't get an extra link block
			YAMLDetails:     domain.NewDetails(), // Initialize empty details
		}

		// Initialize YAML details with basic properties
		dirFile.YAMLDetails.SetString(domain.RelationKey(bundle.RelationKeyName.String()), dirName)
		dirFile.YAMLDetails.SetString(domain.RelationKey(bundle.RelationKeyIconEmoji.String()), "📂")

		// Store the directory page
		dirPages[dirPath] = dirFile
		files[dirPath] = dirFile
	}
}
