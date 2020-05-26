package _import

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/util/slice"

	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

type Services interface {
	CreateSmartBlock(req pb.RpcBlockCreatePageRequest) (pageId string, err error)
	SetDetails(req pb.RpcBlockSetDetailsRequest) (err error)
	SimplePaste(contextId string, anySlot []*model.Block) (err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest) error
}

func (imp *importImpl) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinks []*model.Block, err error) {
	s := imp.NewStateCtx(ctx)
	defer log.Debug("5. ImportMarkdown: all smartBlocks done")

	nameToBlocksAfterCsv := make(map[string][][]*model.Block)
	idToTitle := make(map[string]string)

	nameToBlocks, isPageLinked, files, err := imp.DirWithMarkdownToBlocks(req.ImportPath)

	filesCount := len(files)
	log.Debug("FILES COUNT:", filesCount)
	nameToId := make(map[string]string)

	for name := range nameToBlocks {
		nameToId[name], err = imp.ctrl.CreateSmartBlock(pb.RpcBlockCreatePageRequest{})
	}

	for name := range nameToBlocks {
		var title string
		var emoji string
		if len(nameToBlocks[name]) > 0 {
			if text := nameToBlocks[name][0].GetText(); text != nil && text.Style == model.BlockContentText_Header1 {
				title = text.Text
				titleParts := strings.SplitN(title, " ", 2)

				// only select the first rune to see if it looks like emoji
				if len(titleParts) == 2 && emojiAproxRegexp.MatchString(string([]rune(titleParts[0])[0:1])) {
					// first symbol is emoji - just use it all before the space
					emoji = titleParts[0]
					title = titleParts[1]
				}
				// remove title block
				nameToBlocks[name] = nameToBlocks[name][1:]
			} else {
				title := strings.Replace(filepath.Base(name), ".md", "", -1)
				titleParts := strings.Split(title, " ")
				title = strings.Join(titleParts[:len(titleParts)-1], " ")
			}
		}

		if emoji == "" {
			emoji = slice.GetRandomString(articleIcons, name)
		}

		// FIELD-BLOCK
		fields := map[string]*types.Value{
			"name":      pbtypes.String(title),
			"iconEmoji": pbtypes.String(emoji),
		}

		smartblockID := nameToId[name]
		idToTitle[smartblockID] = title

		var details = []*pb.RpcBlockSetDetailsDetail{}

		for name, val := range fields {
			details = append(details, &pb.RpcBlockSetDetailsDetail{
				Key:   name,
				Value: val,
			})
		}

		err = imp.ctrl.SetDetails(pb.RpcBlockSetDetailsRequest{
			ContextId: smartblockID,
			Details:   details,
		})

		if err != nil {
			return rootLinks, err
		}
	}

	log.Debug("1. ImportMarkdown: all smartBlocks created")

	for name, blocks := range nameToBlocks {
		if len(blocks) > 0 {
			blocks, isPageLinked = imp.processFieldBlockIfItIs(blocks, isPageLinked, files, nameToId, idToTitle)
		}

		for _, block := range blocks {
			if link := block.GetLink(); link != nil && len(nameToId[name]) > 0 {
				target, err := url.PathUnescape(link.TargetBlockId)
				if err != nil {
					log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, link.TargetBlockId)
					target = link.TargetBlockId
				}

				link.TargetBlockId = nameToId[target]
			}
		}
	}

	for name := range nameToBlocks {
		for i, b := range nameToBlocks[name] {
			nameToBlocksAfterCsv[name] = append(nameToBlocksAfterCsv[name], []*model.Block{nameToBlocks[name][i]})

			if f := b.GetFile(); f != nil && filepath.Ext(f.Name) == ".csv" {
				csvName := strings.Replace(f.Name, req.ImportPath+"/", "", -1)
				nameToBlocksAfterCsv[name][i], isPageLinked = imp.convertCsvToLinks(csvName, files[csvName], b, isPageLinked, nameToId, files)
			}
		}
	}

	for name := range nameToBlocksAfterCsv {
		nameToBlocks[name] = []*model.Block{}
		for i := range nameToBlocksAfterCsv[name] {
			for j := range nameToBlocksAfterCsv[name][i] {
				nameToBlocks[name] = append(nameToBlocks[name], nameToBlocksAfterCsv[name][i][j])
			}
		}
	}

	log.Debug("2. ImportMarkdown: start to paste blocks")
	for name := range nameToBlocks {
		if len(nameToBlocks[name]) > 0 {
			log.Debug("   >>> start to paste to page:", name)
			err = imp.ctrl.SimplePaste(nameToId[name], nameToBlocks[name])
		}

		if err != nil {
			return rootLinks, err
		}
	}

	log.Debug("3. ImportMarkdown: all blocks pasted. Start to convert rootLinks")
	for name := range nameToBlocks {
		log.Debug("   >>> current page:", name, "    |   linked: ", isPageLinked[name])

		for _, b := range nameToBlocks[name] {
			if f := b.GetFile(); f != nil {

				filesCount = filesCount - 1
				log.Debug("          page:", name, " | start to upload file :", f.Name)

				if strings.HasPrefix(f.Name, "http://") || strings.HasPrefix(f.Name, "https://") {
					err = imp.ctrl.UploadBlockFile(ctx, pb.RpcBlockUploadRequest{
						ContextId: nameToId[name],
						BlockId:   b.Id,
						Url:       f.Name,
					})
					if err != nil {
						return rootLinks, fmt.Errorf("can not import this file from URL: %s. error: %s", f.Name, err)
					}
					continue
				}

				FN := strings.Split(f.Name, "/")
				tmpFile, err := os.Create(filepath.Join(os.TempDir(), FN[len(FN)-1]))
				fName := strings.ReplaceAll(f.Name, req.ImportPath+"/", "")
				w := bufio.NewWriter(tmpFile)

				for fn := range files {
					if strings.Contains(fn, fName) {
						fName = fn
					}
				}

				if _, err = w.Write(files[fName]); err != nil {
					log.Warn("Failed to write to temporary file:", err)
				}

				if err := w.Flush(); err != nil {
					log.Fatal(err)
				}

				err = imp.ctrl.UploadBlockFile(ctx, pb.RpcBlockUploadRequest{
					ContextId: nameToId[name],
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

	for name := range nameToBlocks {
		if !isPageLinked[name] {
			rootLinks = append(rootLinks, &model.Block{
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: nameToId[name],
						Style:         model.BlockContentLink_Page,
						Fields:        nil,
					},
				},
			})
		}
	}

	log.Debug("4. ImportMarkdown: everything done")

	return rootLinks, imp.Apply(s)
}

func (imp *importImpl) DirWithMarkdownToBlocks(directoryPath string) (nameToBlocks map[string][]*model.Block, isPageLinked map[string]bool, files map[string][]byte, err error) {
	log.Debug("1. DirWithMarkdownToBlocks: directory %s", directoryPath)

	anymarkConv := anymark.New()

	files = make(map[string][]byte)
	isPageLinked = make(map[string]bool)
	nameToBlocks = make(map[string][]*model.Block)

	allFileShortPaths := []string{}

	if filepath.Ext(directoryPath) == ".zip" {
		r, err := zip.OpenReader(directoryPath)
		defer r.Close()

		if err != nil {
			return nameToBlocks, isPageLinked, files, fmt.Errorf("can not read zip while import: %s", err)
		}

		for _, f := range r.File {
			elements := strings.Split(f.Name, "/")
			if len(elements) > 0 &&
				len(elements[0]) > 2 &&
				elements[0][:2] == "__" {
				elements = elements[1:]
			}

			if len(elements) > 0 &&
				len(elements[len(elements)-1]) > 2 &&
				elements[len(elements)-1][:2] == "._" {

				continue

			}

			shortPath := strings.Join(elements, "/")

			allFileShortPaths = append(allFileShortPaths, shortPath)
			rc, err := f.Open()

			files[shortPath], err = ioutil.ReadAll(rc)
			rc.Close()

			if err != nil {
				return nameToBlocks, isPageLinked, files, fmt.Errorf("ERR while read file from zip while import: %s", err)
			}

		}

	} else {
		err = filepath.Walk(directoryPath,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() {
					shortPath := strings.Replace(path, directoryPath+"/", "", -1)
					allFileShortPaths = append(allFileShortPaths, shortPath)
					dat, err := ioutil.ReadFile(path)
					if err != nil {
						return err
					}

					if len(shortPath) > 0 {
						files[shortPath] = dat
					}
				}

				return nil
			},
		)
		if err != nil {
			return nameToBlocks, isPageLinked, files, err
		}
	}

	log.Debug("1. DirWithMarkdownToBlocks: Get allFileShortPaths:", allFileShortPaths)
	isFileExist := make(map[string]bool)

	for shortPath := range files {
		log.Debug("   >>> Current file:", shortPath)
		if filepath.Ext(shortPath) == ".md" {
			nameToBlocks[shortPath], err = anymarkConv.MarkdownToBlocks(files[shortPath], allFileShortPaths)
		} else {
			isFileExist[shortPath] = true
		}
	}

	log.Debug("2. DirWithMarkdownToBlocks: MarkdownToBlocks completed")

	isPageLinked = make(map[string]bool)
	for name, j := range nameToBlocks {
		log.Debug("   >>> Page:", name, " Number: ", j)
		for i, block := range nameToBlocks[name] {
			log.Debug("      Block:", i)
			nameToBlocks[name][i].Id = uuid.New().String()

			txt := block.GetText()
			if txt != nil && txt.Marks != nil && len(txt.Marks.Marks) == 1 &&
				txt.Marks.Marks[0].Type == model.BlockContentTextMark_Link {

				linkConverted, err := url.PathUnescape(txt.Marks.Marks[0].Param)
				if err != nil {
					log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, txt.Marks.Marks[0].Param)
					linkConverted = txt.Marks.Marks[0].Param
				}

				if nameToBlocks[linkConverted] != nil {
					nameToBlocks[name][i], isPageLinked = imp.convertTextToPageLink(block, isPageLinked)
				}

				if isFileExist[linkConverted] {
					nameToBlocks[name][i] = imp.convertTextToFile(block, directoryPath)
				}
			}

			if block.GetFile() != nil {

				fileName, err := url.PathUnescape(block.GetFile().Name)
				if err != nil {
					log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, block.GetFile().Name)
					fileName = txt.Marks.Marks[0].Param
				}
				if !strings.HasPrefix(fileName, "http://") && !strings.HasPrefix(fileName, "https://") {
					// not a URL
					fileName = directoryPath + "/" + fileName
				}

				block.GetFile().Name = fileName
				block.GetFile().Type = model.BlockContentFile_Image
			}
		}
	}

	log.Debug("3. DirWithMarkdownToBlocks: convertTextToPageLink, convertTextToFile completed")

	return nameToBlocks, isPageLinked, files, err
}

func (imp *importImpl) convertTextToPageLink(block *model.Block, isPageLinked map[string]bool) (*model.Block, map[string]bool) {
	targetId, err := url.PathUnescape(block.GetText().Marks.Marks[0].Param)
	if err != nil {
		log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, block.GetText().Marks.Marks[0].Param)
		targetId = block.GetText().Marks.Marks[0].Param
	}

	blockOut := &model.Block{
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: targetId,
				Style:         model.BlockContentLink_Page,
			},
		},
	}

	isPageLinked[targetId] = true
	return blockOut, isPageLinked
}

func (imp *importImpl) convertTextToFile(block *model.Block, importPath string) *model.Block {
	fName, err := url.PathUnescape(block.GetText().Marks.Marks[0].Param)
	if err != nil {
		log.Warnf("err while url.PathUnescape: %s \n \t\t\t url: %s", err, block.GetText().Marks.Marks[0].Param)
		fName = block.GetText().Marks.Marks[0].Param
	}

	fName = importPath + "/" + fName

	// "svg" excluded
	imageFormats := []string{"jpg", "jpeg", "png", "gif", "webp"}
	videoFormats := []string{"mp4", "m4v"}

	fileType := model.BlockContentFile_File
	for _, ext := range imageFormats {
		if filepath.Ext(fName)[1:] == ext {
			fileType = model.BlockContentFile_Image
		}
	}

	for _, ext := range videoFormats {
		if filepath.Ext(fName)[1:] == ext {
			fileType = model.BlockContentFile_Video
		}
	}

	blockOut := &model.Block{
		Id: block.Id,
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:  fName,
				State: model.BlockContentFile_Empty,
				Type:  fileType,
			},
		},
	}

	return blockOut
}

func (imp *importImpl) convertCsvToLinks(csvName string, csvData []byte, block *model.Block, isPageLinked map[string]bool, nameToId map[string]string, files map[string][]byte) (blocks []*model.Block, newIsPageLinked map[string]bool) {
	nameArr := strings.Split(csvName, "/")
	nArr := strings.Split(nameArr[len(nameArr)-1], ".")
	name := strings.Join(nArr[:len(nArr)-1], "") // csvname 28jf298f20fj029qd

	blocks = append(blocks, &model.Block{
		Id: uuid.New().String(),
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  imp.shortPathToName(csvName),
			Style: model.BlockContentText_Header3,
		}},
	})

	var shortPathArr []string

	for nf, _ := range files {
		if strings.Contains(nf, name) {
			log.Debugf("FILE for name (%s) FOUND: %s\n", name, nf)
			shortPathArr = append(shortPathArr, nf)
		}
	}

	for i, shortPath := range shortPathArr {
		log.Debugf(" %d.  path %s \n", i, shortPath)
		targetId := nameToId[shortPath]

		if len(targetId) == 0 {
			log.Warnf("WARNING! target (%s) with shortpath (%s) not found", shortPath)
		} else {
			isPageLinked[shortPath] = true

			fields := make(map[string]*types.Value)
			fields["name"] = &types.Value{
				Kind: &types.Value_StringValue{StringValue: imp.shortPathToName(shortPath)},
			}
			if len(shortPath) != 0 {

				blocks = append(blocks, &model.Block{
					Id: uuid.New().String(),
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: targetId,
							Style:         model.BlockContentLink_Page,
							Fields: &types.Struct{
								Fields: fields,
							},
						},
					},
				})
			}
		}
	}

	return blocks, isPageLinked
}

func (imp *importImpl) processFieldBlockIfItIs(blocks []*model.Block, isPageLinked map[string]bool, files map[string][]byte, nameToId map[string]string, idToTitle map[string]string) (blocksOut []*model.Block, isPageLinkedOut map[string]bool) {
	if len(blocks) < 1 || blocks[0].GetText() == nil {
		return blocks, isPageLinked
	}
	blocksOut = blocks

	txt := blocks[0].GetText().Text
	if txt == "" {
		return blocks, isPageLinked
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

		keyVal := strings.Split(pair, ":")
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

			var targetId = nameToId[shortPath]
			/*for name, anytypePageId := range nameToId {
				if imp.getIdFromPath(name) == id {
					targetId = anytypePageId
				}
			}*/

			if len(targetId) == 0 {
				text += potentialFileName
				log.Debug("     TARGET NOT FOUND:", shortPath, potentialFileName)
			} else {
				log.Debug("     TARGET FOUND:", targetId, shortPath)
				isPageLinked[shortPath] = true
				title := idToTitle[targetId]
				if title == "" {
					// should be a case
					title = shortPath
				}

				text += title
				marks = append(marks, &model.BlockContentTextMark{
					Range: &model.Range{int32(len(text) - len(title)), int32(len(text))},
					Type:  model.BlockContentTextMark_Mention,
					Param: targetId,
				})
			}
		}
	}

	blockText := blocks[0].GetText()
	blockText.Text = text
	blockText.Marks = &model.BlockContentTextMarks{marks}

	return blocksOut, isPageLinked
}

func (imp *importImpl) getIdFromPath(path string) (id string) {
	a := strings.Split(path, " ")
	b := a[len(a)-1]
	if len(b) < 3 {
		return ""
	}
	return b[:len(b)-3]
}

func (imp *importImpl) shortPathToName(path string) (name string) {
	sArr := strings.Split(filepath.Base(path), " ")
	if len(sArr) == 0 {
		return path
	}

	name = strings.Join(sArr[:len(sArr)-1], " ")
	return name
}
