package markdown

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/uri"
)

type mdConverter struct {
	tempDirProvider core.TempDirProvider
}

type FileInfo struct {
	os.FileInfo
	HasInboundLinks       bool
	PageID                string
	IsRootFile            bool
	Title                 string
	ParsedBlocks          []*model.Block
	CollectionsObjectsIds []string
}

func newMDConverter(tempDirProvider core.TempDirProvider) *mdConverter {
	return &mdConverter{tempDirProvider: tempDirProvider}
}

func (m *mdConverter) markdownToBlocks(importPath string, importSource source.Source, allErrors *common.ConvertError) map[string]*FileInfo {
	files := m.processFiles(importPath, allErrors, importSource)

	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	return files
}

func (m *mdConverter) processFiles(importPath string, allErrors *common.ConvertError, importSource source.Source) map[string]*FileInfo {
	err := importSource.Initialize(importPath)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(0, model.Import_Markdown) {
			return nil
		}
	}
	if importSource.CountFilesWithGivenExtensions([]string{".md"}) == 0 {
		allErrors.Add(common.ErrNoObjectsToImport)
		return nil
	}
	fileInfo := m.getFileInfo(importSource, allErrors)
	for name, file := range fileInfo {
		m.processBlocks(name, file, fileInfo)
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

func (m *mdConverter) processBlocks(shortPath string, file *FileInfo, files map[string]*FileInfo) {
	for _, block := range file.ParsedBlocks {
		m.processTextBlock(block, files)
	}
	m.processLinkBlock(shortPath, file, files)
}

func (m *mdConverter) processTextBlock(block *model.Block, files map[string]*FileInfo) {
	txt := block.GetText()
	if txt != nil && txt.Marks != nil {
		if len(txt.Marks.Marks) == 1 && txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {
			m.handleSingleMark(block, files)
		} else {
			m.handleMultipleMarks(block, files)
		}
	}
}

func (m *mdConverter) handleSingleMark(block *model.Block, files map[string]*FileInfo) {
	txt := block.GetText()
	link := txt.Marks.Marks[0].Param
	wholeLineLink := m.isWholeLineLink(txt.Text, txt.Marks.Marks[0])
	ext := filepath.Ext(link)
	if file := files[link]; file != nil {
		if strings.EqualFold(ext, ".csv") {
			m.processCSVFileLink(block, files, link, wholeLineLink)
			return
		}
		if strings.EqualFold(ext, ".md") {
			// only convert if this is the only link in the row
			m.convertToAnytypeLinkBlock(block, wholeLineLink)
		} else {
			block.Content = anymark.ConvertTextToFile(txt.Marks.Marks[0].Param)
		}
		file.HasInboundLinks = true
	} else if wholeLineLink {
		block.Content = m.convertTextToBookmark(txt.Marks.Marks[0].Param)
	}
}

func (m *mdConverter) handleMultipleMarks(block *model.Block, files map[string]*FileInfo) {
	txt := block.GetText()
	for _, mark := range txt.Marks.Marks {
		if mark.Type == model.BlockContentTextMark_Link {
			if stop := m.handleSingleLinkMark(block, files, mark, txt); stop {
				return
			}
		}
	}
}

func (m *mdConverter) handleSingleLinkMark(block *model.Block, files map[string]*FileInfo, mark *model.BlockContentTextMark, txt *model.BlockContentText) bool {
	link := mark.Param
	ext := filepath.Ext(link)
	if file := files[link]; file != nil {
		file.HasInboundLinks = true
		if strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".csv") {
			mark.Type = model.BlockContentTextMark_Mention
			return false
		}
		if m.isWholeLineLink(txt.Text, mark) {
			block.Content = anymark.ConvertTextToFile(mark.Param)
			return true
		}
	} else if m.isWholeLineLink(txt.Text, mark) {
		block.Content = m.convertTextToBookmark(mark.Param)
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

func (m *mdConverter) convertTextToBookmark(url string) *model.BlockContentOfBookmark {
	if err := uri.ValidateURI(url); err != nil {
		return nil
	}

	return &model.BlockContentOfBookmark{
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
		files[filePath].ParsedBlocks, _, err = anymark.MarkdownToBlocks(b, filepath.Dir(filePath), nil)
		if err != nil {
			log.Errorf("failed to read blocks: %s", err)
		}
	}
	return nil
}
