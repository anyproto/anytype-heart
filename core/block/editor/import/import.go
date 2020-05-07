package _import

import (
	"fmt"
	"io/ioutil"
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
	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, err error)
	UploadBlockFile(ctx *state.Context, req pb.RpcBlockUploadRequest) error
}

func (imp *importImpl) ImportMarkdown(ctx *state.Context, req pb.RpcBlockImportMarkdownRequest) (rootLinks []*model.Block, err error) {
	s := imp.NewStateCtx(ctx)

	nameToBlocks, isPageLinked, filesCount, err := imp.DirWithMarkdownToBlocks(req.ImportPath)
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

		nameToId[name], err = imp.ctrl.CreateSmartBlock(pb.RpcBlockCreatePageRequest{
			Details: &types.Struct{
				Fields: map[string]*types.Value{
					"name":      pbtypes.String(fileName),
					"iconEmoji": pbtypes.String(slice.GetRangomString(articleIcons)),
				},
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
				link.TargetBlockId = nameToId[strings.Replace(link.TargetBlockId, "%20", " ", -1)]
			}
		}
	}

	log.Debug("2. ImportMarkdown: start to paste blocks")
	for name := range nameToBlocks {
		if len(nameToBlocks[name]) > 0 {
			log.Debug("   >>> start to paste to page:", name)
			_, _, _, err = imp.ctrl.Paste(ctx, pb.RpcBlockPasteRequest{
				ContextId: nameToId[name],
				AnySlot:   nameToBlocks[name],
			})
		}

		if err != nil {
			return rootLinks, err
		}
	}

	log.Debug("3. ImportMarkdown: all blocks pasted. Start to convert rootLinks")
	for name := range nameToBlocks {
		log.Debug("   >>> current page:", name, "    |   linked: ", isPageLinked[name])
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

		for _, b := range nameToBlocks[name] {
			if f := b.GetFile(); f != nil {

				filesCount = filesCount - 1
				log.Debug("          page:", name, " | start to upload file :", f.Name)
				err = imp.ctrl.UploadBlockFile(ctx, pb.RpcBlockUploadRequest{
					ContextId: nameToId[name],
					BlockId:   b.Id,
					FilePath:  f.Name,
					Url:       "",
				})

				if err != nil {
					return rootLinks, fmt.Errorf("can not import this file: %s. error: %s", f.Name, err)
				}
			}
		}
	}

	log.Debug("4. ImportMarkdown: everything done")

	return rootLinks, imp.Apply(s)
}

func (imp *importImpl) DirWithMarkdownToBlocks(directoryPath string) (nameToBlock map[string][]*model.Block, isPageLinked map[string]bool, filesCount int, err error) {
	anymarkConv := anymark.New()

	nameToBlocks := make(map[string][]*model.Block)
	allFileShortPaths := []string{}
	files := make(map[string][]byte)

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

				files[shortPath] = dat
			}

			return nil
		},
	)

	log.Debug("1. DirWithMarkdownToBlocks: Get allFileShortPaths:", allFileShortPaths)
	if err != nil {
		return nameToBlocks, isPageLinked, filesCount, err
	}

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

				linkConverted := strings.Replace(txt.Marks.Marks[0].Param, "%20", " ", -1)

				if nameToBlocks[linkConverted] != nil {
					nameToBlocks[name][i], isPageLinked = imp.convertTextToPageLink(block, isPageLinked)
				}

				if isFileExist[linkConverted] {
					nameToBlocks[name][i] = imp.convertTextToFile(block, directoryPath)
				}
			}

			if block.GetFile() != nil {
				block.GetFile().Name = strings.Replace(directoryPath+"/"+block.GetFile().Name, "%20", " ", -1)
				block.GetFile().Type = model.BlockContentFile_Image
			}
		}
	}

	log.Debug("3. DirWithMarkdownToBlocks: convertTextToPageLink, convertTextToFile completed")

	return nameToBlocks, isPageLinked, len(isFileExist), err
}

func (imp *importImpl) convertTextToPageLink(block *model.Block, isPageLinked map[string]bool) (*model.Block, map[string]bool) {
	targetId := strings.Replace(block.GetText().Marks.Marks[0].Param, "%20", " ", -1)
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
	fName := strings.Replace(importPath+"/"+block.GetText().Marks.Marks[0].Param, "%20", " ", -1)

	imageFormats := []string{"jpg", "jpeg", "png", "gif", "webp"} // "svg" excluded
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
