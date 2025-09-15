package markdown

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
)

type mdConverter struct {
	tempDirProvider core.TempDirProvider
	schemaImporter  *SchemaImporter       // Optional schema importer for property resolution
	yamlResolver    *YAMLPropertyResolver // Resolver for consistent property keys when no schema
}

type FileInfo struct {
	os.FileInfo
	OriginalPath          string // may be not unicode NFC-normalized. use it to get original file name
	HasInboundLinks       bool
	PageID                string
	IsRootFile            bool
	IsRootDirPage         bool
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

func (m *mdConverter) markdownToBlocks(importPath string, importSource source.Source, allErrors *common.ConvertError, createDirectoryPages bool) *fileContainer {
	files := m.processFiles(importPath, allErrors, importSource)

	// Create directory pages if requested
	if createDirectoryPages {
		m.createDirectoryPages(importPath, files)
	}

	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	return files
}

func (m *mdConverter) processFiles(importPath string, allErrors *common.ConvertError, importSource source.Source) *fileContainer {
	if importSource.CountFilesWithGivenExtensions([]string{".md"}) == 0 {
		allErrors.Add(common.ErrorBySourceType(importSource))
		return nil
	}
	fileInfo := m.getFileInfo(importSource, allErrors)
	for name, file := range fileInfo.byPath {
		m.processBlocks(name, file, fileInfo, importSource)
		for _, b := range file.ParsedBlocks {
			m.processFileBlock(b, importSource, importPath, fileInfo)
		}
	}
	return fileInfo
}

type fileContainer struct {
	byName map[string]*FileInfo
	byPath map[string]*FileInfo
}

func (fc *fileContainer) setFile(path string, file *FileInfo) {
	fc.byPath[path] = file
	fc.byName[filepath.Base(path)] = file
	fc.byName[filepath.Base(file.OriginalPath)] = file
}

func (m *mdConverter) getFileInfo(importSource source.Source, allErrors *common.ConvertError) *fileContainer {
	fileInfo := &fileContainer{
		byName: make(map[string]*FileInfo),
		byPath: make(map[string]*FileInfo),
	}
	if iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		if err := m.fillFilesInfo(importSource, fileInfo.byPath, fileName, fileReader); err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(0, model.Import_Markdown) {
				return false
			}
		}
		return true
	}); iterateErr != nil {
		allErrors.Add(iterateErr)
	}

	for path, info := range fileInfo.byPath {
		fileInfo.byName[filepath.Base(info.OriginalPath)] = info
		fileInfo.byName[filepath.Base(path)] = info
	}
	return fileInfo
}

func (m *mdConverter) fillFilesInfo(importSource source.Source, fileInfo map[string]*FileInfo, path string, rc io.ReadCloser) error {
	normalizedPath := normalizePath(path)
	fileInfo[normalizedPath] = &FileInfo{OriginalPath: path}
	if err := m.createBlocksFromFile(importSource, normalizedPath, rc, fileInfo); err != nil {
		log.Errorf("failed to create blocks from file: %s", err)
		return err
	}
	return nil
}

func (m *mdConverter) processBlocks(shortPath string, file *FileInfo, files *fileContainer, importSource source.Source) {
	for _, block := range file.ParsedBlocks {
		m.processTextBlock(block, files, importSource)
	}
	m.processLinkBlock(shortPath, file, files)
}

func (m *mdConverter) processTextBlock(block *model.Block, files *fileContainer, importSource source.Source) {
	txt := block.GetText()
	if txt != nil && txt.Marks != nil {
		if len(txt.Marks.Marks) == 1 && txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {
			m.handleSingleMark(block, files, importSource)
		} else {
			m.handleMultipleMarks(block, files, importSource)
		}
	}
}

func normalizePath(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		// Return URLs as they are, no normalization needed
		return path
	}
	// Normalize the path to ensure consistent formatting
	// This is important for matching file names across different systems
	return norm.NFC.String(filepath.Clean(path))
}

// findCommonPrefix finds the common directory prefix of two paths
func findCommonPrefix(path1, path2 string) string {
	// Handle empty paths
	if path1 == "" || path2 == "" {
		return ""
	}

	// Clean paths
	path1 = filepath.Clean(path1)
	path2 = filepath.Clean(path2)

	// Split paths into components
	sep := string(filepath.Separator)
	parts1 := strings.Split(path1, sep)
	parts2 := strings.Split(path2, sep)

	// Handle absolute paths on Unix-like systems
	if path1 != "" && path1[0] == filepath.Separator && parts1[0] == "" {
		parts1 = parts1[1:]
	}
	if path2 != "" && path2[0] == filepath.Separator && parts2[0] == "" {
		parts2 = parts2[1:]
	}

	// Find common prefix
	var common []string
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		if parts1[i] == parts2[i] && parts1[i] != "" {
			common = append(common, parts1[i])
		} else {
			break
		}
	}

	if len(common) == 0 {
		return ""
	}

	result := filepath.Join(common...)
	// Handle absolute paths
	if filepath.IsAbs(path1) && filepath.IsAbs(path2) {
		result = string(filepath.Separator) + result
	}
	return result
}

func (m *mdConverter) handleSingleMark(block *model.Block, files *fileContainer, importSource source.Source) {
	txt := block.GetText()
	link := normalizePath(txt.Marks.Marks[0].Param)

	wholeLineLink := m.isWholeLineLink(txt.Text, txt.Marks.Marks[0])
	ext := filepath.Ext(link)

	if ext == "" || strings.Contains(ext, " ") {
		link += ".md"
	}

	link = m.getOriginalName(link, importSource)
	if file := findFile(files, link); file != nil {
		txt.Marks.Marks[0].Type = model.BlockContentTextMark_Mention
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
			block.Content = anymark.ConvertTextToFile(link)
		}
		file.HasInboundLinks = true
	} else if wholeLineLink {
		m.convertTextToBookmark(txt.Marks.Marks[0].Param, block)
	}
}

func (m *mdConverter) handleMultipleMarks(block *model.Block, files *fileContainer, importSource source.Source) {
	txt := block.GetText()
	for _, mark := range txt.Marks.Marks {
		if mark.Type == model.BlockContentTextMark_Link {
			if stop := m.handleSingleLinkMark(block, files, mark, txt, importSource); stop {
				return
			}
		}
	}
}

func (m *mdConverter) handleSingleLinkMark(block *model.Block, files *fileContainer, mark *model.BlockContentTextMark, txt *model.BlockContentText, importSource source.Source) bool {
	isWholeLink := m.isWholeLineLink(txt.Text, mark)
	link := normalizePath(mark.Param)
	ext := filepath.Ext(link)
	if ext == "" || strings.Contains(ext, " ") {
		link += ".md"
	}

	link = m.getOriginalName(link, importSource)
	ext = filepath.Ext(link)
	if file := findFile(files, link); file != nil {
		file.HasInboundLinks = true
		if strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".csv") {
			mark.Type = model.BlockContentTextMark_Mention
			mark.Param = link
			return false
		}
		if isWholeLink {
			block.Content = anymark.ConvertTextToFile(link)
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

func (m *mdConverter) processCSVFileLink(block *model.Block, files *fileContainer, link string, wholeLineLink bool) {
	csvDir := strings.TrimSuffix(link, ".csv")
	for name, file := range files.byPath {
		// set HasInboundLinks for all CSV-origin md files
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.HasInboundLinks = true
		}
	}
	m.convertToAnytypeLinkBlock(block, wholeLineLink)
	files.byPath[link].HasInboundLinks = true
}

func findFile(files *fileContainer, name string) *FileInfo {
	if name == "" {
		return nil
	}
	// Check if the file exists in the map by its original path
	if file, exists := files.byPath[name]; exists {
		return file
	}
	if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		// If it's a URL, we can't find it by path, so return nil
		return nil
	}
	// If not found, try to find it by base name
	return findPathByBaseName(files, name)
}

func findPathByBaseName(files *fileContainer, name string) *FileInfo {
	name = filepath.Base(name)
	if file, exists := files.byName[name]; exists {
		return file
	}
	log.Debugf("file %s not found in files map", name)
	return nil
}

func (m *mdConverter) processFileBlock(block *model.Block, importedSource source.Source, importPath string, files *fileContainer) {
	if f := block.GetFile(); f != nil {
		if block.Id == "" {
			block.Id = bson.NewObjectId().Hex()
		}
		var err error
		name := f.Name
		if file := findFile(files, name); file != nil {
			name, _, err = common.ProvideFileName(file.OriginalPath, importedSource, importPath, m.tempDirProvider)
			if err != nil {
				log.Errorf("failed to update file block, %v", err)
			}
		} else {
			// If it's a URL, preserve it; otherwise clear it
			if !strings.HasPrefix(name, "http://") && !strings.HasPrefix(name, "https://") {
				name = ""
			}
		}
		block.GetFile().Name = name
	}
}

func (m *mdConverter) processLinkBlock(shortPath string, file *FileInfo, files *fileContainer) {
	ext := filepath.Ext(shortPath)
	if !strings.EqualFold(ext, ".csv") {
		return
	}
	dependentFilesDir := strings.TrimSuffix(shortPath, ext)
	for targetName, targetFile := range files.byPath {
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
	if !anymark.IsUrl(url) {
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

// createDirectoryPages creates a page for each directory level (including root) in the import
// Each directory page contains block links to nested pages and subdirectories
func (m *mdConverter) createDirectoryPages(importPath string, files *fileContainer) {
	rootPath := m.findRootPath(files)
	dirStructure := m.buildDirectoryStructure(files, rootPath)

	// For zip files with single root directory, collapse it
	if shouldCollapseRoot(rootPath, files) {
		rootPath = ""
		dirStructure = m.buildDirectoryStructure(files, rootPath)
	}

	// Create a page for each directory
	for dirPath, children := range dirStructure {
		if shouldSkipDirectory(dirPath, rootPath) {
			continue
		}

		dirPage := m.createDirectoryPage(dirPath, rootPath, children, files, dirStructure)
		files.setFile(dirPath, dirPage)
	}
}

// findRootPath finds the common root directory of all files
func (m *mdConverter) findRootPath(files *fileContainer) string {
	var paths []string
	for _, file := range files.byPath {
		paths = append(paths, filepath.Dir(file.OriginalPath))
	}

	if len(paths) == 0 {
		return ""
	}

	root := paths[0]
	for _, path := range paths[1:] {
		root = findCommonPrefix(root, path)
	}

	if root == "." || root == "" {
		return ""
	}
	return root
}

// buildDirectoryStructure creates a map of directory paths to their children
func (m *mdConverter) buildDirectoryStructure(files *fileContainer, rootPath string) map[string][]string {
	dirStructure := make(map[string][]string)
	dirStructure[rootPath] = []string{}

	// If we're collapsing root (rootPath is ""), we need to strip the common prefix
	commonPrefix := ""
	if rootPath == "" && shouldCollapseRoot(m.findRootPath(files), files) {
		// Find the common directory that we're collapsing
		for _, file := range files.byPath {
			parts := strings.Split(file.OriginalPath, string(filepath.Separator))
			if len(parts) > 0 && parts[0] != "" && parts[0] != "." {
				commonPrefix = parts[0]
				break
			}
		}
	}

	for filePath, file := range files.byPath {
		if !isMarkdownOrCSV(file.OriginalPath) {
			continue
		}

		// Determine the directory path relative to root
		dir := m.getRelativeDir(file.OriginalPath, rootPath, commonPrefix)

		// Add file to its directory
		dirStructure[dir] = append(dirStructure[dir], filePath)

		// Create parent directory entries
		m.createParentDirs(dir, rootPath, dirStructure)
	}

	return dirStructure
}

// getRelativeDir returns the directory path relative to the root
func (m *mdConverter) getRelativeDir(filePath, rootPath, commonPrefix string) string {
	// If we have a common prefix to strip (for collapsed roots)
	if commonPrefix != "" {
		filePath = strings.TrimPrefix(filePath, commonPrefix+string(filepath.Separator))
	}

	dir := filepath.Dir(filePath)

	if rootPath == "" {
		if dir == "." {
			return ""
		}
		return dir
	}

	// For zip imports with common prefix
	if !filepath.IsAbs(rootPath) && strings.HasPrefix(filePath, rootPath+string(filepath.Separator)) {
		trimmed := strings.TrimPrefix(filePath, rootPath+string(filepath.Separator))
		dir = filepath.Dir(trimmed)
		if dir == "." {
			return ""
		}
		return dir
	}

	// For regular path imports
	if rootPath != "" && dir != rootPath {
		relPath, err := filepath.Rel(rootPath, dir)
		if err == nil && !strings.HasPrefix(relPath, "..") {
			if relPath == "." {
				return rootPath
			}
			return relPath
		}
	}

	return dir
}

// createParentDirs ensures all parent directories exist in the structure
func (m *mdConverter) createParentDirs(dir, rootPath string, dirStructure map[string][]string) {
	current := dir
	for current != rootPath && current != "" && current != "." && current != "/" {
		parent := m.getParentDir(current, rootPath)

		// Add current to parent's children if not already there
		if !contains(dirStructure[parent], current) {
			dirStructure[parent] = append(dirStructure[parent], current)
		}

		if parent == rootPath {
			break
		}
		current = parent
	}
}

// getParentDir returns the parent directory path
func (m *mdConverter) getParentDir(dir, rootPath string) string {
	if rootPath != "" && !strings.Contains(dir, string(filepath.Separator)) {
		// Direct child of root
		return rootPath
	}

	parent := filepath.Dir(dir)
	if parent == "." {
		if rootPath != "" {
			return rootPath
		}
		return ""
	}
	return parent
}

// shouldCollapseRoot checks if we should collapse a single root directory (common in zip files)
func shouldCollapseRoot(rootPath string, files *fileContainer) bool {
	if rootPath == "" || filepath.IsAbs(rootPath) {
		return false
	}

	// Check if all files share the same top-level directory
	var commonDir string
	for _, file := range files.byPath {
		parts := strings.Split(file.OriginalPath, string(filepath.Separator))
		if len(parts) == 0 {
			return false
		}

		if commonDir == "" {
			commonDir = parts[0]
		} else if parts[0] != commonDir {
			return false
		}
	}

	return commonDir != "" && commonDir != "."
}

// shouldSkipDirectory checks if a directory should be skipped
func shouldSkipDirectory(dirPath, rootPath string) bool {
	if dirPath == rootPath {
		return false // Never skip root
	}

	dirName := filepath.Base(dirPath)
	return strings.HasPrefix(dirName, ".") // Skip hidden directories
}

// createDirectoryPage creates a FileInfo for a directory page
func (m *mdConverter) createDirectoryPage(dirPath, rootPath string, children []string, files *fileContainer, dirStructure map[string][]string) *FileInfo {
	displayName := m.getDirectoryDisplayName(dirPath, rootPath)
	blocks := m.createChildLinks(children, files, dirStructure)

	dirFile := &FileInfo{
		Title:           displayName,
		ParsedBlocks:    blocks,
		HasInboundLinks: true,
		YAMLDetails:     domain.NewDetails(),
		IsRootDirPage:   dirPath == rootPath,
	}

	// Set basic properties
	dirFile.YAMLDetails.SetString(domain.RelationKey(bundle.RelationKeyName.String()), displayName)
	dirFile.YAMLDetails.SetString(domain.RelationKey(bundle.RelationKeyIconEmoji.String()), "📂")

	return dirFile
}

// getDirectoryDisplayName returns the display name for a directory
func (m *mdConverter) getDirectoryDisplayName(dirPath, rootPath string) string {
	if dirPath == rootPath {
		name := filepath.Base(rootPath)
		if name == "." || name == "/" || name == "" {
			return rootCollectionName
		}
		return name
	}
	return filepath.Base(dirPath)
}

// createChildLinks creates block links for all children in a directory
func (m *mdConverter) createChildLinks(children []string, files *fileContainer, dirStructure map[string][]string) []*model.Block {
	// Sort children: directories first, then files
	sortedChildren := m.sortChildren(children, dirStructure)

	var blocks []*model.Block
	for _, childPath := range sortedChildren {
		if _, isDir := dirStructure[childPath]; isDir {
			// Skip hidden directories
			if strings.HasPrefix(filepath.Base(childPath), ".") {
				continue
			}
			blocks = append(blocks, m.createDirectoryLink(childPath))
		} else if file, exists := files.byPath[childPath]; exists {
			blocks = append(blocks, m.createFileLink(childPath, file))
		}
	}

	return blocks
}

// sortChildren sorts children with directories first, then files, both alphabetically
func (m *mdConverter) sortChildren(children []string, dirStructure map[string][]string) []string {
	sorted := make([]string, len(children))
	copy(sorted, children)

	sort.Slice(sorted, func(i, j int) bool {
		_, iIsDir := dirStructure[sorted[i]]
		_, jIsDir := dirStructure[sorted[j]]

		if iIsDir != jIsDir {
			return iIsDir // Directories first
		}
		return sorted[i] < sorted[j] // Alphabetical
	})

	return sorted
}

// createDirectoryLink creates a link block for a subdirectory
func (m *mdConverter) createDirectoryLink(dirPath string) *model.Block {
	return &model.Block{
		Id: bson.NewObjectId().Hex(),
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				CardStyle:     model.BlockContentLink_Inline,
				IconSize:      model.BlockContentLink_SizeSmall,
				TargetBlockId: dirPath,
				Style:         model.BlockContentLink_Page,
			},
		},
	}
}

// createFileLink creates a link block for a file
func (m *mdConverter) createFileLink(filePath string, file *FileInfo) *model.Block {
	fileName := file.Title
	if fileName == "" {
		fileName = filepath.Base(filePath)
		if strings.HasSuffix(fileName, ".md") {
			fileName = fileName[:len(fileName)-3]
		}
	}

	return &model.Block{
		Id: bson.NewObjectId().Hex(),
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: filePath,
				Style:         model.BlockContentLink_Page,
				CardStyle:     model.BlockContentLink_Card,
				Description:   model.BlockContentLink_Content,
				IconSize:      model.BlockContentLink_SizeMedium,
			},
		},
	}
}

// Helper functions
func isMarkdownOrCSV(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".md" || ext == ".csv"
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
