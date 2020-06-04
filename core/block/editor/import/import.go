package _import

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
)

var (
	linkRegexp                   = regexp.MustCompile(`\[([\s\S]*?)\]\((.*?)\)`)
	filenameCleanRegexp          = regexp.MustCompile(`[^\w_\s]+`)
	filenameDuplicateSpaceRegexp = regexp.MustCompile(`\s+`)
	emojiAproxRegexp             = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)

	log          = logging.Logger("anytype-import")
	articleIcons = []string{"📓", "📕", "📗", "📘", "📙", "📖", "📔", "📒", "📝", "📄", "📑"}
	dbIcons      = []string{"🗃", "🗂"}
)

type Import interface {
	ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinks []*model.Block, err error)
}

func NewImport(sb smartblock.SmartBlock, ctrl Services) Import {
	return &importImpl{SmartBlock: sb, ctrl: ctrl}
}

type importImpl struct {
	smartblock.SmartBlock
	ctrl Services
}

type fileInfo struct {
	os.FileInfo
	io.ReadCloser
	hasInboundLinks bool
	pageID          string
	isRootFile      bool
	title           string
	parsedBlocks    []*model.Block
}

type Services interface {
	CreateSmartBlock(req pb.RpcBlockCreatePageRequest) (pageId string, err error)
	SetDetails(req pb.RpcBlockSetDetailsRequest) (err error)
	SimplePaste(contextId string, anySlot []*model.Block) (err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest) error
	BookmarkFetch(ctx *state.Context, req pb.RpcBlockBookmarkFetchRequest) error
}

func (imp *importImpl) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinks []*model.Block, err error) {
	s := imp.NewStateCtx(ctx)
	defer log.Debug("5. ImportMarkdown: all smartBlocks done")

	files, close, err := imp.DirWithMarkdownToBlocks(req.ImportPath)
	defer func() {
		if close != nil {
			_ = close()
		}
	}()

	filesCount := len(files)
	log.Debug("FILES COUNT:", filesCount)

	var pagesCreated int

	for name, file := range files {
		// index links in the root csv file
		if !file.isRootFile || !strings.EqualFold(filepath.Ext(name), ".csv") {
			continue
		}

		ext := filepath.Ext(name)
		csvDir := strings.TrimSuffix(name, ext)

		for targetName, targetFile := range files {
			fileExt := filepath.Ext(targetName)
			if filepath.Dir(targetName) == csvDir && strings.EqualFold(fileExt, ".md") {
				targetFile.hasInboundLinks = true
			}
		}
	}

	for name, file := range files {
		if !strings.EqualFold(filepath.Ext(name), ".md") {
			continue
		}

		if !file.isRootFile && !file.hasInboundLinks {
			log.Errorf("skip non-root md files without inbound links %s", name)
			continue
		}

		pageID, err := imp.ctrl.CreateSmartBlock(pb.RpcBlockCreatePageRequest{})
		if err != nil {
			log.Errorf("failed to create smartblock: %s", err.Error())
			continue
		}
		file.pageID = pageID
		pagesCreated++
	}

	log.Debug("pages created:", pagesCreated)

	for name, file := range files {
		var title string
		var emoji string

		if file.pageID == "" {
			// file is not a page
			continue
		}

		if len(file.parsedBlocks) > 0 {
			if text := file.parsedBlocks[0].GetText(); text != nil && text.Style == model.BlockContentText_Header1 {
				title = text.Text
				titleParts := strings.SplitN(title, " ", 2)

				// only select the first rune to see if it looks like emoji
				if len(titleParts) == 2 && emojiAproxRegexp.MatchString(string([]rune(titleParts[0])[0:1])) {
					// first symbol is emoji - just use it all before the space
					emoji = titleParts[0]
					title = titleParts[1]
				}
				// remove title block
				file.parsedBlocks = file.parsedBlocks[1:]
			}
		}

		if emoji == "" {
			emoji = slice.GetRandomString(articleIcons, name)
		}

		if title == "" {
			title := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
			titleParts := strings.Split(title, " ")
			title = strings.Join(titleParts[:len(titleParts)-1], " ")
		}

		// FIELD-BLOCK
		fields := map[string]*types.Value{
			"name":      pbtypes.String(title),
			"iconEmoji": pbtypes.String(emoji),
		}

		file.title = title

		var details = []*pb.RpcBlockSetDetailsDetail{}

		for name, val := range fields {
			details = append(details, &pb.RpcBlockSetDetailsDetail{
				Key:   name,
				Value: val,
			})
		}

		err = imp.ctrl.SetDetails(pb.RpcBlockSetDetailsRequest{
			ContextId: file.pageID,
			Details:   details,
		})

		if err != nil {
			return rootLinks, err
		}
	}

	log.Debug("1. ImportMarkdown: all smartBlocks created")
	log.Debug("1. ImportMarkdown: all smartBlocks created")
	for _, file := range files {
		if file.pageID == "" {
			// file is not a page
			continue
		}

		file.parsedBlocks = imp.processFieldBlockIfItIs(file.parsedBlocks, files)

		for _, block := range file.parsedBlocks {
			if link := block.GetLink(); link != nil {
				target, err := url.PathUnescape(link.TargetBlockId)
				if err != nil {
					log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, link.TargetBlockId)
					target = link.TargetBlockId
				}

				if files[target] != nil {
					link.TargetBlockId = files[target].pageID
					files[target].hasInboundLinks = true
				}

			} else if text := block.GetText(); text != nil && text.Marks != nil && len(text.Marks.Marks) > 0 {
				for _, mark := range text.Marks.Marks {
					if mark.Type != model.BlockContentTextMark_Mention {
						continue
					}

					if targetFile, exists := files[mark.Param]; exists {
						mark.Param = targetFile.pageID
					}
				}
			}
		}
	}

	for name, file := range files {
		// wrap root-level csv files with page
		if file.isRootFile && strings.EqualFold(filepath.Ext(name), ".csv") {
			pageID, err := imp.ctrl.CreateSmartBlock(pb.RpcBlockCreatePageRequest{})
			if err != nil {
				log.Errorf("failed to create smartblock: %s", err.Error())
				continue
			}

			fields := map[string]*types.Value{
				"name":      pbtypes.String(imp.shortPathToName(name)),
				"iconEmoji": pbtypes.String(slice.GetRandomString(dbIcons, name)),
			}

			var details = []*pb.RpcBlockSetDetailsDetail{}

			for name, val := range fields {
				details = append(details, &pb.RpcBlockSetDetailsDetail{
					Key:   name,
					Value: val,
				})
			}

			err = imp.ctrl.SetDetails(pb.RpcBlockSetDetailsRequest{
				ContextId: pageID,
				Details:   details,
			})

			file.pageID = pageID
			file.parsedBlocks = imp.convertCsvToLinks(name, files)
		}

		if file.pageID == "" {
			// file is not a page
			continue
		}

		var blocks = make([]*model.Block, 0, len(file.parsedBlocks))
		for i, b := range file.parsedBlocks {
			if f := b.GetFile(); f != nil && strings.EqualFold(filepath.Ext(f.Name), ".csv") {
				if csvFile, exists := files[f.Name]; exists {
					csvFile.hasInboundLinks = true
				} else {
					continue
				}

				csvInlineBlocks := imp.convertCsvToLinks(f.Name, files)
				blocks = append(blocks, csvInlineBlocks...)
			} else {
				blocks = append(blocks, file.parsedBlocks[i])
			}
		}

		file.parsedBlocks = blocks
	}

	log.Debug("2. ImportMarkdown: start to paste blocks")
	for name, file := range files {
		if file.pageID == "" {
			// file is not a page
			continue
		}

		log.Debug("   >>> start to paste to page:", name, file.pageID)
		if file.parsedBlocks == nil {
			log.Errorf("parsedBlocks is nil")
		}
		err = imp.ctrl.SimplePaste(file.pageID, file.parsedBlocks)
		if err != nil {
			return rootLinks, err
		}
	}

	log.Debug("3. ImportMarkdown: all blocks pasted. Start to upload files & fetch bookmarks")
	for name, file := range files {
		log.Debug("   >>> current page:", name, "    |   linked: ", file.hasInboundLinks)
		if file.pageID == "" {
			continue
		}

		for _, b := range file.parsedBlocks {
			if bm := b.GetBookmark(); bm != nil {
				err = imp.ctrl.BookmarkFetch(ctx, pb.RpcBlockBookmarkFetchRequest{
					ContextId: file.pageID,
					BlockId:   b.Id,
					Url:       bm.Url,
				})
				if err != nil {
					log.Errorf("failed to fetch bookmark %s: %s", bm.Url, err.Error())
				}
			} else if f := b.GetFile(); f != nil {
				filesCount = filesCount - 1
				log.Debug("          page:", name, " | start to upload file :", f.Name)

				if strings.HasPrefix(f.Name, "http://") || strings.HasPrefix(f.Name, "https://") {
					err = imp.ctrl.UploadBlockFile(ctx, pb.RpcBlockUploadRequest{
						ContextId: file.pageID,
						BlockId:   b.Id,
						Url:       f.Name,
					})
					if err != nil {
						return rootLinks, fmt.Errorf("can not import this file from URL: %s. error: %s", f.Name, err)
					}
					continue
				}

				baseName := filepath.Base(f.Name)
				tmpFile, err := os.Create(filepath.Join(os.TempDir(), baseName))

				shortPath := f.Name

				w := bufio.NewWriter(tmpFile)
				targetFile, found := files[shortPath]
				if !found {
					log.Errorf("file %s not found", shortPath)
					continue
				}

				_, err = w.ReadFrom(targetFile.ReadCloser)
				if err != nil {
					log.Errorf("failed to read file %s: %s", shortPath, err.Error())
					continue
				}

				if err := w.Flush(); err != nil {
					log.Errorf("failed to flush file %s: %s", shortPath, err.Error())
					continue
				}

				targetFile.Close()
				tmpFile.Close()

				err = imp.ctrl.UploadBlockFile(ctx, pb.RpcBlockUploadRequest{
					ContextId: file.pageID,
					BlockId:   b.Id,
					FilePath:  tmpFile.Name(),
					Url:       "",
				})

				if err != nil {
					return rootLinks, fmt.Errorf("can not import this file: %s. error: %s", f.Name, err)
				}

				os.Remove(tmpFile.Name())
			}
		}
	}

	for _, file := range files {
		if file.pageID == "" {
			// not a page
			continue
		}

		if file.hasInboundLinks {
			continue
		}

		rootLinks = append(rootLinks, &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: file.pageID,
					Style:         model.BlockContentLink_Page,
					Fields:        nil,
				},
			},
		})
	}

	log.Debug("4. ImportMarkdown: everything done")

	return rootLinks, imp.Apply(s)
}

func (imp *importImpl) DirWithMarkdownToBlocks(importPath string) (files map[string]*fileInfo, fileClose func() error, err error) {
	log.Debug("1. DirWithMarkdownToBlocks: directory %s", importPath)

	anymarkConv := anymark.New()

	files = make(map[string]*fileInfo)
	fileClose = func() error {
		return nil
	}
	allFileShortPaths := []string{}

	ext := filepath.Ext(importPath)
	if strings.EqualFold(ext, ".zip") {
		r, err := zip.OpenReader(importPath)
		if err != nil {
			return files, fileClose, fmt.Errorf("can not read zip while import: %s", err)
		}
		fileClose = r.Close

		zipName := strings.TrimSuffix(importPath, ext)
		for _, f := range r.File {
			if strings.HasPrefix(f.Name, "__MACOSX/") {
				continue
			}
			shortPath := filepath.Clean(f.Name)
			// remove zip root folder if exists
			shortPath = strings.TrimPrefix(shortPath, zipName+"/")

			allFileShortPaths = append(allFileShortPaths, shortPath)

			rc, err := f.Open()
			if err != nil {
				return files, fileClose, fmt.Errorf("failed to open file from zip while import: %s", err)
			}

			files[shortPath] = &fileInfo{
				FileInfo:   f.FileInfo(),
				ReadCloser: rc,
			}
		}

	} else {
		err = filepath.Walk(importPath,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() {
					shortPath, err := filepath.Rel(importPath+"/", path)
					if err != nil {
						return fmt.Errorf("failed to get relative path: %s", err.Error())
					}

					allFileShortPaths = append(allFileShortPaths, shortPath)
					f, err := os.Open(path)
					if err != nil {
						return err
					}

					files[shortPath] = &fileInfo{
						FileInfo:   info,
						ReadCloser: f,
					}
				}

				return nil
			},
		)
		if err != nil {
			return files, fileClose, err
		}
	}

	log.Debug("1. DirWithMarkdownToBlocks: Get allFileShortPaths:", allFileShortPaths)

	for shortPath, file := range files {
		log.Debug("   >>> Current file:", shortPath)
		if filepath.Base(shortPath) == shortPath {
			file.isRootFile = true
		}

		if filepath.Ext(shortPath) == ".md" {
			b, err := ioutil.ReadAll(file)
			if err != nil {
				log.Errorf("failed to read file %s: %s", shortPath, err.Error())
				continue
			}

			file.parsedBlocks, _, err = anymarkConv.MarkdownToBlocks(b, filepath.Dir(shortPath), allFileShortPaths)
			if err != nil {
				log.Errorf("failed to read blocks %s: %s", shortPath, err.Error())
			}
			// md file no longer needed
			file.Close()

			for i, block := range file.parsedBlocks {
				log.Debug("      Block:", i)
				//file.parsedBlocks[i].Id = uuid.New().String()

				txt := block.GetText()
				if txt != nil && txt.Marks != nil && len(txt.Marks.Marks) == 1 &&
					txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {

					link := txt.Marks.Marks[0].Param

					var wholeLineLink bool
					if (txt.Marks.Marks[0].Range.From == 0 || strings.TrimSpace(txt.Text[0:txt.Marks.Marks[0].Range.From]) == "") &&
						(int(txt.Marks.Marks[0].Range.To) >= (len(txt.Text)-1) || strings.TrimSpace(txt.Text[txt.Marks.Marks[0].Range.To:]) == "") {
						wholeLineLink = true
					}

					ext := filepath.Ext(link)

					// todo: bug with multiple markup links in arow when the first is external
					if file := files[link]; file != nil {
						if strings.EqualFold(ext, ".md") {
							// only convert if this is the only link in the row
							if wholeLineLink {
								imp.convertTextToPageLink(block)
							} else {
								imp.convertTextToPageMention(block)
							}
						} else {
							imp.convertTextToFile(block)
						}

						if strings.EqualFold(ext, ".csv") {
							csvDir := strings.TrimSuffix(link, ext)
							for name, file := range files {
								// set hasInboundLinks for all CSV-origin md files
								fileExt := filepath.Ext(name)
								if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
									file.hasInboundLinks = true
								}
							}
						}
						file.hasInboundLinks = true
					} else if wholeLineLink {
						imp.convertTextToBookmark(block)
					} else {
						log.Debugf("")
					}

					/*if block.GetFile() != nil {
						fileName, err := url.PathUnescape(block.GetFile().Name)
						if err != nil {
							log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, block.GetFile().Name)
							fileName = txt.Marks.Marks[0].Param
						}
						if !strings.HasPrefix(fileName, "http://") && !strings.HasPrefix(fileName, "https://") {
							// not a URL
							fileName = importPath + "/" + fileName
						}

						block.GetFile().Name = fileName
						block.GetFile().Type = model.BlockContentFile_Image
					}*/
				}
			}

			ext := filepath.Ext(shortPath)
			dependentFilesDir := strings.TrimSuffix(shortPath, ext)
			var hasUnlinkedDependentMDFiles bool
			for targetName, targetFile := range files {
				fileExt := filepath.Ext(targetName)
				if filepath.Dir(targetName) == dependentFilesDir && strings.EqualFold(fileExt, ".md") {
					if !targetFile.hasInboundLinks {
						if !hasUnlinkedDependentMDFiles {
							// add Unsorted header
							file.parsedBlocks = append(file.parsedBlocks, &model.Block{
								Id: uuid.New().String(),
								Content: &model.BlockContentOfText{Text: &model.BlockContentText{
									Text:  "Unsorted",
									Style: model.BlockContentText_Header3,
								}},
							})
							hasUnlinkedDependentMDFiles = true
						}

						file.parsedBlocks = append(file.parsedBlocks, &model.Block{
							Id: uuid.New().String(),
							Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
								TargetBlockId: targetName,
								Style:         model.BlockContentLink_Page,
							}},
						})

						targetFile.hasInboundLinks = true
					}
				}
			}
		}

	}
	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	return files, fileClose, err
}

func (imp *importImpl) convertTextToPageLink(block *model.Block) {
	block.Content = &model.BlockContentOfLink{
		Link: &model.BlockContentLink{
			TargetBlockId: block.GetText().Marks.Marks[0].Param,
			Style:         model.BlockContentLink_Page,
		},
	}
}

func (imp *importImpl) convertTextToBookmark(block *model.Block) {
	if _, err := url.Parse(block.GetText().Marks.Marks[0].Param); err != nil {
		return
	}

	block.Content = &model.BlockContentOfBookmark{
		Bookmark: &model.BlockContentBookmark{
			Url: block.GetText().Marks.Marks[0].Param,
		},
	}
}

func (imp *importImpl) convertTextToPageMention(block *model.Block) {
	for _, mark := range block.GetText().Marks.Marks {
		if mark.Type != model.BlockContentTextMark_Link {
			continue
		}

		mark.Param = mark.Param
		mark.Type = model.BlockContentTextMark_Mention
	}
}

func (imp *importImpl) convertTextToFile(block *model.Block) {
	// "svg" excluded
	imageFormats := []string{"jpg", "jpeg", "png", "gif", "webp"}
	videoFormats := []string{"mp4", "m4v"}

	fileType := model.BlockContentFile_File
	for _, ext := range imageFormats {
		if strings.EqualFold(filepath.Ext(block.GetText().Marks.Marks[0].Param)[1:], ext) {
			fileType = model.BlockContentFile_Image
			break
		}
	}

	for _, ext := range videoFormats {
		if strings.EqualFold(filepath.Ext(block.GetText().Marks.Marks[0].Param)[1:], ext) {
			fileType = model.BlockContentFile_Video
			break
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

func (imp *importImpl) convertCsvToLinks(csvFileName string, files map[string]*fileInfo) (blocks []*model.Block) {
	ext := filepath.Ext(csvFileName)
	csvDir := strings.TrimSuffix(csvFileName, ext)

	blocks = append(blocks, &model.Block{
		Id: uuid.New().String(),
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  imp.shortPathToName(csvFileName),
			Style: model.BlockContentText_Header3,
		}},
	})

	for name, file := range files {
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.hasInboundLinks = true
			fields := make(map[string]*types.Value)
			fields["name"] = &types.Value{
				Kind: &types.Value_StringValue{StringValue: file.title},
			}

			blocks = append(blocks, &model.Block{
				Id: uuid.New().String(),
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: file.pageID,
						Style:         model.BlockContentLink_Page,
						Fields: &types.Struct{
							Fields: fields,
						},
					},
				},
			})
		}
	}

	return blocks
}

func (imp *importImpl) processFieldBlockIfItIs(blocks []*model.Block, files map[string]*fileInfo) (blocksOut []*model.Block) {
	if len(blocks) < 1 || blocks[0].GetText() == nil {
		return blocks
	}
	blocksOut = blocks

	txt := blocks[0].GetText().Text
	if txt == "" ||
		(blocks[0].GetText().Marks != nil && len(blocks[0].GetText().Marks.Marks) > 0) {
		// fields can't have a markup
		return blocks
	}

	potentialPairs := strings.Split(txt, "\n")

	var text string
	var marks []*model.BlockContentTextMark
	for _, pair := range potentialPairs {
		if text != "" {
			text += "\n"
		}
		if len(pair) <= 3 || pair[len(pair)-3:] != ".md" {
			text += pair
			continue
		}

		keyVal := strings.SplitN(pair, ":", 2)
		if len(keyVal) < 2 {
			text += pair
			continue
		}

		potentialFileNames := strings.Split(keyVal[1], ",")
		text += keyVal[0] + ": "

		for potentialFileNameIndex, potentialFileName := range potentialFileNames {
			potentialFileName, _ = url.PathUnescape(potentialFileName)
			potentialFileName = strings.ReplaceAll(potentialFileName, `"`, "")
			if potentialFileNameIndex != 0 {
				text += ", "
			}
			shortPath := ""
			id := imp.getIdFromPath(potentialFileName)
			for name, _ := range files {
				if imp.getIdFromPath(name) == id {
					shortPath = name
					break
				}
			}

			file := files[shortPath]
			/*for name, anytypePageId := range nameToId {
				if imp.getIdFromPath(name) == id {
					targetId = anytypePageId
				}
			}*/

			if len(file.pageID) == 0 {
				text += potentialFileName
				log.Debug("     TARGET NOT FOUND:", shortPath, potentialFileName)
			} else {
				log.Debug("     TARGET FOUND:", file.pageID, shortPath)
				file.hasInboundLinks = true
				if file.title == "" {
					// shouldn't be a case
					file.title = shortPath
				}

				text += file.title
				marks = append(marks, &model.BlockContentTextMark{
					Range: &model.Range{int32(len(text) - len(file.title)), int32(len(text))},
					Type:  model.BlockContentTextMark_Mention,
					Param: file.pageID,
				})
			}
		}
	}

	if len(marks) > 0 {
		blockText := blocks[0].GetText()
		blockText.Text = text
		blockText.Marks = &model.BlockContentTextMarks{marks}
	}

	return blocksOut
}

func (imp *importImpl) getIdFromPath(path string) (id string) {
	a := strings.Split(path, " ")
	b := a[len(a)-1]
	if len(b) < 3 {
		return ""
	}
	return b[:len(b)-3]
}

/*func (imp *importImpl) getShortPath(importPath string, ) (id string) {
	a := strings.Split(path, " ")
	b := a[len(a)-1]
	if len(b) < 3 {
		return ""
	}
	return b[:len(b)-3]
}*/

func (imp *importImpl) shortPathToName(path string) (name string) {
	sArr := strings.Split(filepath.Base(path), " ")
	if len(sArr) == 0 {
		return path
	}

	name = strings.Join(sArr[:len(sArr)-1], " ")
	return name
}
