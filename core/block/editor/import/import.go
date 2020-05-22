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
	linkRegexp   = regexp.MustCompile(`\[([\s\S]*?)\]\((.*?)\)`)
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
	SimplePaste(contextId string, anySlot []*model.Block) (err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest) error
}

func (imp *importImpl) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinks []*model.Block, err error) {
	s := imp.NewStateCtx(ctx)
	defer log.Debug("5. ImportMarkdown: all smartBlocks done")

	files := make(map[string][]byte)
	isPageLinked := make(map[string]bool)
	nameToBlocks := make(map[string][]*model.Block)
	nameToBlocksAfterCsv := make(map[string][][]*model.Block)

	nameToBlocks, isPageLinked, files, err = imp.DirWithMarkdownToBlocks(req.ImportPath)

	filesCount := len(files)
	log.Debug("FILES COUNT:", filesCount)
	nameToId := make(map[string]string)

	for name := range nameToBlocks {
		fileName := strings.Replace(filepath.Base(name), ".md", "", -1)

		if len(nameToBlocks[name]) > 0 && nameToBlocks[name][0].GetText() != nil &&
			nameToBlocks[name][0].GetText().Text == fileName {
			nameToBlocks[name] = nameToBlocks[name][1:]
		}

		//untitled
		if len(name) >= 8 &&
			strings.ToLower(name)[:8] == "untitled" &&
			len(nameToBlocks[name]) > 0 &&
			nameToBlocks[name][0].GetText() != nil &&
			len(nameToBlocks[name][0].GetText().Text) > 0 {

			if strings.Contains(name, ".") {
				fileName = strings.Split(name, ".")[0]
			} else {
				fileName = nameToBlocks[name][0].GetText().Text
			}
		}

		fArr := strings.Split(fileName, " ")
		fileName = strings.Join(fArr[:len(fArr)-1], "")

		// FIELD-BLOCK
		fields := map[string]*types.Value{
			"name":      pbtypes.String(fileName),
			"iconEmoji": pbtypes.String(slice.GetRandomString(articleIcons, fileName)),
		}

		if t := nameToBlocks[name][0].GetText(); t != nil && t.Text == fileName {
			nameToBlocks[name] = nameToBlocks[name][1:]
		}

		if len(nameToBlocks[name]) > 0 {
			nameToBlocks[name], fields = imp.processFieldBlockIfItIs(nameToBlocks[name], fields)
		}

		nameToId[name], err = imp.ctrl.CreateSmartBlock(pb.RpcBlockCreatePageRequest{
			Details: &types.Struct{
				Fields: fields,
			},
		})

		if err != nil {
			return rootLinks, err
		}
	}

	log.Debug("1. ImportMarkdown: all smartBlocks created")

	for name := range nameToBlocks {
		for i := range nameToBlocks[name] {
			if link := nameToBlocks[name][i].GetLink(); link != nil && len(nameToId[name]) > 0 {
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
				nameToBlocksAfterCsv[name][i], isPageLinked = imp.convertCsvToLinks(files[csvName], b, isPageLinked, nameToId, files)
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

func (imp *importImpl) DirWithMarkdownToBlocks(directoryPath string) (nameToBlock map[string][]*model.Block, isPageLinked map[string]bool, files map[string][]byte, err error) {
	log.Debug("1. DirWithMarkdownToBlocks: directory %s", directoryPath)

	anymarkConv := anymark.New()

	files = make(map[string][]byte)
	isPageLinked = make(map[string]bool)
	nameToBlocks := make(map[string][]*model.Block)
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
				return nameToBlock, isPageLinked, files, fmt.Errorf("ERR while read file from zip while import: %s", err)
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

				fileName = directoryPath + "/" + fileName

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

func (imp *importImpl) convertCsvToLinks(csvData []byte, block *model.Block, isPageLinked map[string]bool, nameToId map[string]string, files map[string][]byte) (blocks []*model.Block, newIsPageLinked map[string]bool) {
	var records [][]string

	for _, str := range strings.Split(string(csvData), "\n") {
		records = append(records, strings.Split(str, ","))
	}

	headers := records[0]
	records = records[1:]
	nameArr := strings.Split(block.GetFile().Name, "/")

	headerLastElArr := strings.Split(nameArr[len(nameArr)-1], " ")
	headerLastElArr = headerLastElArr[:len(headerLastElArr)-1]

	blocks = append(blocks, &model.Block{
		Id: uuid.New().String(),
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  strings.Join(headerLastElArr, " "),
			Style: model.BlockContentText_Header3,
		}},
	})

	for _, record := range records {
		fileName := record[0]
		fileName = strings.ReplaceAll(fileName, `"`, "")
		shortPath := ""
		for name, _ := range files {
			nameSects := strings.Split(name, "/")
			if strings.Contains(nameSects[len(nameSects)-1], fileName) && filepath.Ext(nameSects[len(nameSects)-1]) == ".md" {
				shortPath = name
			}
		}

		targetId := nameToId[shortPath]
		if len(targetId) == 0 {
			log.Warn("WARNING! target (%s) with shortpath (%s) not found (%s)\n", fileName, shortPath, nameToId[shortPath])
		} else {
			isPageLinked[shortPath] = true
		}

		// TODO: if no targetId
		fields := make(map[string]*types.Value)

		for h, header := range headers {
			fields[header] = &types.Value{
				Kind: &types.Value_StringValue{StringValue: record[h]},
			}
		}

		fields["name"] = &types.Value{
			Kind: &types.Value_StringValue{StringValue: record[0]},
		}

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

	return blocks, isPageLinked
}

func (imp *importImpl) processFieldBlockIfItIs(blocks []*model.Block, fields map[string]*types.Value) (blocksOut []*model.Block, fieldsOut map[string]*types.Value) {
	if len(blocks) < 2 || blocks[1].GetText() == nil {
		return blocks, fields
	}
	blocksOut = blocks

	txt := blocks[1].GetText().Text
	potentialPairs := strings.Split(txt, "\n")

	for _, pair := range potentialPairs {
		keyVal := strings.Split(pair, ":")
		if len(keyVal) != 2 {
			return blocksOut, fields
		}

		fields[keyVal[0]] = pbtypes.String(keyVal[1])
	}

	// TODO: do not remove while we can not render fields
	// blocksOut = append(blocks[:1], blocks[2:]...)

	return blocksOut, fields
}
