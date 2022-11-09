package markdown

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	ce "github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
	"github.com/pkg/errors"
)

type Service interface {
	TempDir() string
}

type MarkdownToBlocks struct{
	Service
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

func NewMarkdownToBlocks(s Service) *MarkdownToBlocks {
	return &MarkdownToBlocks{Service: s}
}

func (m *MarkdownToBlocks) MarkdownToBlocks(importPath, mode string) (map[string]*FileInfo, ce.ConvertError) {
	allErrors := ce.NewError()
	files := m.processFiles(importPath, mode, allErrors)

	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	return files, allErrors
}

func (m *MarkdownToBlocks) processFiles(importPath, mode string, allErrors ce.ConvertError) (map[string]*FileInfo) {
	ext := filepath.Ext(importPath)
	if strings.EqualFold(ext, ".zip") {
		return m.processZipFile(importPath, mode, allErrors)
	} else {
		return m.processDirectory(importPath, mode, allErrors)
	}
}

func (m *MarkdownToBlocks) processZipFile(importPath, mode string, allErrors ce.ConvertError) (map[string]*FileInfo) {
	r, err := zip.OpenReader(importPath)
	anymarkConv := anymark.New()
	if err != nil {
		allErrors.Add(importPath, err)
		return nil
	}
	defer r.Close()
	files := make(map[string]*FileInfo, 0)
	zipName := strings.TrimSuffix(importPath, filepath.Ext(importPath))
	for _, f := range r.File {
		shortPath := filepath.Clean(f.Name)
		// remove zip root folder if exists
		shortPath = strings.TrimPrefix(shortPath, zipName+"/")

		if err != nil {
			allErrors.Add(shortPath, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
				return nil
			}
			log.Errorf("failed to read file %s: %s", shortPath, err.Error())
			continue
		}
		rc, err := f.Open()
		if err != nil {
			allErrors.Add(shortPath, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
				return nil
			}
			log.Errorf("failed to read file %s: %s", shortPath, err.Error())
			continue
		}
		files[shortPath] = &FileInfo{}
		files[shortPath].Source = ce.GetSourceDetail(shortPath, importPath)
		if err := m.createBlocksFromFile(shortPath, anymarkConv, rc, files); err != nil {
			allErrors.Add(shortPath, err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING.String() {
				return nil
			}
			log.Errorf("failed to create blocks from file %s: %s", shortPath, err.Error())
		}
	}
	for _, file := range files {
		for _, b := range file.ParsedBlocks {
			m.processFileBlock(b, files)
		}
	}

	return files
}

func (m *MarkdownToBlocks) processDirectory(importPath, mode string, allErrors ce.ConvertError) (map[string]*FileInfo)  {
	files := make(map[string]*FileInfo)
	anymarkConv := anymark.New()
	err := filepath.Walk(importPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.Wrap(err, "markdown import: processDirectory")
			}

			if !info.IsDir() {
				shortPath, err := filepath.Rel(importPath+string(filepath.Separator), path)
				if err != nil {
					return fmt.Errorf("failed to get relative path %s", err)
				}
				f, err := os.Open(path)
				if err != nil {
					return fmt.Errorf("failed to open file: %s", err)
				}
				files[shortPath] = &FileInfo{}
				m.createBlocksFromFile(shortPath, anymarkConv, f, files)
				files[shortPath].Source = ce.GetSourceDetail(shortPath, importPath)
			}

			return nil
		},
	)
	for _, file := range files {
		for _, b := range file.ParsedBlocks {
			m.processFileBlock(b, files)
		}
	}
	if err != nil {
		allErrors.Add(importPath, err)
	}
	return files 
}

func (m *MarkdownToBlocks) processBlocks(shortPath string, file *FileInfo, files map[string]*FileInfo) {
	for _, block := range file.ParsedBlocks {
		m.processTextBlock(block, files)
	}
	m.processLinkBlock(shortPath, file, files)
}

func (m *MarkdownToBlocks) processTextBlock(block *model.Block, files map[string]*FileInfo) {
	txt := block.GetText()
	if txt != nil && txt.Marks != nil && len(txt.Marks.Marks) == 1 &&
		txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {

		link := txt.Marks.Marks[0].Param

		var wholeLineLink bool
		textRunes := []rune(txt.Text)
		var from, to = int(txt.Marks.Marks[0].Range.From), int(txt.Marks.Marks[0].Range.To)
		if from == 0 || (from < len(textRunes) && len(strings.TrimSpace(string(textRunes[0:from]))) == 0) {
			if to >= len(textRunes) || len(strings.TrimSpace(string(textRunes[to:]))) == 0 {
				wholeLineLink = true
			}
		}

		ext := filepath.Ext(link)

		// todo: bug with multiple markup links in arow when the first is external
		if file := files[link]; file != nil {
			if strings.EqualFold(ext, ".md") {
				// only convert if this is the only link in the row
				if wholeLineLink {
					m.convertTextToPageLink(block)
				} else {
					m.convertTextToPageMention(block)
				}
			} else {
				m.convertTextToFile(block)
			}

			if strings.EqualFold(ext, ".csv") {
				csvDir := strings.TrimSuffix(link, ext)
				for name, file := range files {
					// set HasInboundLinks for all CSV-origin md files
					fileExt := filepath.Ext(name)
					if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
						file.HasInboundLinks = true
					}
				}
			}
			file.HasInboundLinks = true
		} else if wholeLineLink {
			m.convertTextToBookmark(block)
		} else {
			log.Debugf("")
		}
	}
}

func (m *MarkdownToBlocks) processFileBlock(block *model.Block, files map[string]*FileInfo) {
	if f := block.GetFile(); f != nil {
		if block.Id == "" {
			block.Id = bson.NewObjectId().Hex()
		}
		m.createFile(f, block.Id, files)
	}
}

func (m *MarkdownToBlocks) processLinkBlock(shortPath string, file *FileInfo, files map[string]*FileInfo) {
	ext := filepath.Ext(shortPath)
	dependentFilesDir := strings.TrimSuffix(shortPath, ext)
	var hasUnlinkedDependentMDFiles bool
	for targetName, targetFile := range files {
		fileExt := filepath.Ext(targetName)
		if filepath.Dir(targetName) == dependentFilesDir && strings.EqualFold(fileExt, ".md") {
			if !targetFile.HasInboundLinks {
				if !hasUnlinkedDependentMDFiles {
					// add Unsorted header
					file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
						Id: bson.NewObjectId().Hex(),
						Content: &model.BlockContentOfText{Text: &model.BlockContentText{
							Text:  "Unsorted",
							Style: model.BlockContentText_Header3,
						}},
					})
					hasUnlinkedDependentMDFiles = true
				}

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



func (m *MarkdownToBlocks) convertTextToPageLink(block *model.Block) {
	block.Content = &model.BlockContentOfLink{
		Link: &model.BlockContentLink{
			TargetBlockId: block.GetText().Marks.Marks[0].Param,
			Style:         model.BlockContentLink_Page,
		},
	}
}

func (m *MarkdownToBlocks) convertTextToBookmark(block *model.Block) {
	if _, err := url.Parse(block.GetText().Marks.Marks[0].Param); err != nil {
		return
	}

	block.Content = &model.BlockContentOfBookmark{
		Bookmark: &model.BlockContentBookmark{
			Url: block.GetText().Marks.Marks[0].Param,
		},
	}
}

func (m *MarkdownToBlocks) convertTextToPageMention(block *model.Block) {
	for _, mark := range block.GetText().Marks.Marks {
		if mark.Type != model.BlockContentTextMark_Link {
			continue
		}
		mark.Type = model.BlockContentTextMark_Mention
	}
}

func (m *MarkdownToBlocks) convertTextToFile(block *model.Block) {
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

func (m *MarkdownToBlocks) createBlocksFromFile(shortPath string, anymarkConv anymark.Markdown, f io.ReadCloser, files map[string]*FileInfo) error {
	if filepath.Base(shortPath) == shortPath {
		files[shortPath].IsRootFile = true
	}
	if filepath.Ext(shortPath) == ".md" {
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		files[shortPath].ParsedBlocks, _, err = anymarkConv.MarkdownToBlocks(b, filepath.Dir(shortPath), nil)
		if err != nil {
			log.Errorf("failed to read blocks %s: %s", shortPath, err.Error())
		}
		// md file no longer needed
		m.processBlocks(shortPath, files[shortPath], files)
		f.Close()
	} else {
		// need to store file reader, so we can use it to create local file and upload it
		files[shortPath].ReadCloser = f
	}
	return nil
}

func (m *MarkdownToBlocks) createFile(f *model.BlockContentFile, id string, files map[string]*FileInfo) {
	baseName := filepath.Base(f.Name) + id
	tempDir := m.TempDir()
	newFile := filepath.Join(tempDir, baseName)
	tmpFile, err := os.Create(newFile)
	if err != nil {
		log.Errorf("failed to create file file %s: %s", baseName, err.Error())
		return
	}
	w := bufio.NewWriter(tmpFile)
	shortPath := f.Name
	targetFile, found := files[shortPath]
	if !found {
		log.Errorf("file %s not found", newFile)
		return
	}

	_, err = w.ReadFrom(targetFile.ReadCloser)
	if err != nil {
		log.Errorf("failed to read file %s: %s", shortPath, err.Error())
		return
	}

	if err := w.Flush(); err != nil {
		log.Errorf("failed to flush file %s: %s", shortPath, err.Error())
		return
	}

	targetFile.Close()
	tmpFile.Close()
	f.Name = newFile
}
