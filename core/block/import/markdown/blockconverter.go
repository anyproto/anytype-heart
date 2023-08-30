package markdown

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/globalsign/mgo/bson"

	ce "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/util/uri"
)

type mdConverter struct {
	tempDirProvider core.TempDirProvider
}

type FileInfo struct {
	os.FileInfo
	io.ReadCloser
	HasInboundLinks bool
	PageID          string
	IsRootFile      bool
	Title           string
	ParsedBlocks    []*model.Block
	Source          string
}

func newMDConverter(tempDirProvider core.TempDirProvider) *mdConverter {
	return &mdConverter{tempDirProvider: tempDirProvider}
}

func (m *mdConverter) markdownToBlocks(importPath, mode string) (map[string]*FileInfo, *ce.ConvertError) {
	allErrors := ce.NewError()
	files := m.processFiles(importPath, mode, allErrors)

	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	return files, allErrors
}

func (m *mdConverter) processFiles(importPath string, mode string, allErrors *ce.ConvertError) map[string]*FileInfo {
	fileInfo := make(map[string]*FileInfo, 0)
	s := source.GetSource(importPath)
	if s == nil {
		return nil
	}
	supportedExtensions := []string{".md", ".csv"}
	imageFormats := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	videoFormats := []string{".mp4", ".m4v"}
	audioFormats := []string{".mp3", ".ogg", ".wav", ".m4a", ".flac"}

	supportedExtensions = append(supportedExtensions, videoFormats...)
	supportedExtensions = append(supportedExtensions, imageFormats...)
	supportedExtensions = append(supportedExtensions, audioFormats...)
	readers, err := s.GetFileReaders(importPath, supportedExtensions, nil)
	if err != nil {
		allErrors.Add(err)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
			return nil
		}
	}
	if len(readers) == 0 {
		allErrors.Add(ce.ErrNoObjectsToImport)
		return nil
	}
	for path, rc := range readers {
		if err = m.fillFilesInfo(importPath, fileInfo, path, rc); err != nil {
			allErrors.Add(err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
				return nil
			}
		}
	}

	for name, file := range fileInfo {
		m.processBlocks(name, file, fileInfo)
		for _, b := range file.ParsedBlocks {
			m.processFileBlock(b, fileInfo)
		}
	}
	return fileInfo
}

func (m *mdConverter) fillFilesInfo(importPath string, fileInfo map[string]*FileInfo, path string, rc io.ReadCloser) error {
	fileInfo[path] = &FileInfo{}
	if err := m.createBlocksFromFile(path, rc, fileInfo); err != nil {
		log.Errorf("failed to create blocks from file: %s", err)
		return err
	}
	fileInfo[path].Source = ce.GetSourceDetail(path, importPath)
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
	if txt != nil && txt.Marks != nil && len(txt.Marks.Marks) == 1 &&
		txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {
		link := txt.Marks.Marks[0].Param
		wholeLineLink := m.isWholeLineLink(txt)
		ext := filepath.Ext(link)

		// todo: bug with multiple markup links in arow when the first is external
		if file := files[link]; file != nil {
			if strings.EqualFold(ext, ".csv") {
				m.processCSVFileLink(block, files, link, wholeLineLink)
				return
			}
			if strings.EqualFold(ext, ".md") {
				// only convert if this is the only link in the row
				m.convertToAnytypeLinkBlock(block, wholeLineLink)
			} else {
				m.convertTextToFile(block)
			}
			file.HasInboundLinks = true
		} else if wholeLineLink {
			m.convertTextToBookmark(block)
		}
	}
}

func (m *mdConverter) isWholeLineLink(txt *model.BlockContentText) bool {
	var wholeLineLink bool
	textRunes := []rune(txt.Text)
	var from, to = int(txt.Marks.Marks[0].Range.From), int(txt.Marks.Marks[0].Range.To)
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

func (m *mdConverter) processFileBlock(block *model.Block, files map[string]*FileInfo) {
	if f := block.GetFile(); f != nil {
		if block.Id == "" {
			block.Id = bson.NewObjectId().Hex()
		}
		m.createFile(f, block.Id, files)
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

func (m *mdConverter) convertTextToBookmark(block *model.Block) {
	if err := uri.ValidateURI(block.GetText().Marks.Marks[0].Param); err != nil {
		return
	}

	block.Content = &model.BlockContentOfBookmark{
		Bookmark: &model.BlockContentBookmark{
			Url: block.GetText().Marks.Marks[0].Param,
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

func (m *mdConverter) convertTextToFile(block *model.Block) {
	// "svg" excluded
	if block.GetText().Marks.Marks[0].Param == "" {
		return
	}

	imageFormats := []string{"jpg", "jpeg", "png", "gif", "webp"}
	videoFormats := []string{"mp4", "m4v"}
	audioFormats := []string{"mp3", "ogg", "wav", "m4a", "flac"}
	pdfFormat := "pdf"

	fileType := model.BlockContentFile_File
	fileExt := filepath.Ext(block.GetText().Marks.Marks[0].Param)
	if fileExt != "" {
		fileExt = fileExt[1:]
		for _, ext := range imageFormats {
			if strings.EqualFold(fileExt, ext) {
				fileType = model.BlockContentFile_Image
				break
			}
		}

		for _, ext := range videoFormats {
			if strings.EqualFold(fileExt, ext) {
				fileType = model.BlockContentFile_Video
				break
			}
		}

		for _, ext := range audioFormats {
			if strings.EqualFold(fileExt, ext) {
				fileType = model.BlockContentFile_Audio
				break
			}
		}

		if strings.EqualFold(fileExt, pdfFormat) {
			fileType = model.BlockContentFile_PDF
		}
	}

	block.Content = &model.BlockContentOfFile{
		File: &model.BlockContentFile{
			Name:  block.GetText().Marks.Marks[0].Param,
			State: model.BlockContentFile_Empty,
			Type:  fileType,
		},
	}
}

func (m *mdConverter) createBlocksFromFile(shortPath string, f io.ReadCloser, files map[string]*FileInfo) error {
	if filepath.Base(shortPath) == shortPath {
		files[shortPath].IsRootFile = true
	}
	if filepath.Ext(shortPath) == ".md" {
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		files[shortPath].ParsedBlocks, _, err = anymark.MarkdownToBlocks(b, filepath.Dir(shortPath), nil)
		if err != nil {
			log.Errorf("failed to read blocks: %s", err.Error())
		}
		// md file no longer needed
		m.processBlocks(shortPath, files[shortPath], files)
	} else {
		// need to store file reader, so we can use it to create local file and upload it
		files[shortPath].ReadCloser = f
	}
	return nil
}

func (m *mdConverter) createFile(f *model.BlockContentFile, id string, files map[string]*FileInfo) {
	baseName := filepath.Base(f.Name) + id
	tempDir := m.tempDirProvider.TempDir()
	newFile := filepath.Join(tempDir, baseName)
	tmpFile, err := os.Create(newFile)
	if err != nil {
		log.Errorf("failed to create file: %s", oserror.TransformError(err).Error())
		return
	}
	defer tmpFile.Close()
	w := bufio.NewWriter(tmpFile)
	shortPath := f.Name
	targetFile, found := files[shortPath]
	if !found {
		return
	}
	defer targetFile.Close()
	_, err = w.ReadFrom(targetFile.ReadCloser)
	if err != nil {
		log.Errorf("failed to read file: %s", err.Error())
		return
	}

	if err := w.Flush(); err != nil {
		log.Errorf("failed to flush file: %s", err.Error())
		return
	}

	f.Name = newFile
}
