package clipboard

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-middleware/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/core/converter/html"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var (
	ErrAllSlotsEmpty        = errors.New("all slots are empty")
	ErrTitlePasteRestricted = errors.New("paste to title restricted")
	ErrOutOfRange           = errors.New("out of range")
	log                     = logging.Logger("anytype-clipboard")
)

type Clipboard interface {
	Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)
	Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
}

func NewClipboard(sb smartblock.SmartBlock, file file.File) Clipboard {
	return &clipboard{SmartBlock: sb, file: file}
}

type clipboard struct {
	smartblock.SmartBlock
	file file.File
}

func (cb *clipboard) Paste(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	caretPosition = -1
	if len(req.FileSlot) > 0 {
		blockIds, err = cb.pasteFiles(ctx, req)
		return
	} else if len(req.AnySlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteAny(ctx, req, groupId)
	} else if len(req.HtmlSlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteHtml(ctx, req, groupId)

		if err != nil {
			blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteText(ctx, req, groupId)
		}

	} else if len(req.TextSlot) > 0 {
		blockIds, uploadArr, caretPosition, isSameBlockCaret, err = cb.pasteText(ctx, req, groupId)

	} else {
		return nil, nil, caretPosition, isSameBlockCaret, ErrAllSlotsEmpty
	}

	return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
}

func (cb *clipboard) Copy(req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	anySlot = req.Blocks
	textSlot = ""
	htmlSlot = ""

	if len(req.Blocks) == 0 {
		return textSlot, htmlSlot, anySlot, nil
	}

	s := cb.blocksToState(req.Blocks)

	var texts []string
	for _, b := range req.Blocks {
		if text := b.GetText(); text != nil {
			texts = append(texts, text.Text)
		}
	}

	if len(texts) > 0 {
		textSlot = strings.Join(texts, "\n")
	}

	var firstTextBlock, lastTextBlock *model.Block
	for _, b := range req.Blocks {
		if b.GetText() != nil {
			if firstTextBlock == nil {
				firstTextBlock = b
			} else {
				lastTextBlock = b
			}
		}
	}

	// scenario: rangeCopy
	if firstTextBlock != nil &&
		req.SelectedTextRange != nil &&
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) &&
		lastTextBlock == nil {
		cutBlock, _, err := simple.New(firstTextBlock).(text.Block).RangeCut(req.SelectedTextRange.From, req.SelectedTextRange.To)
		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("error while cut: %s", err)
		}

		if cutBlock.GetText() != nil && cutBlock.GetText().Marks != nil {
			for i, m := range cutBlock.GetText().Marks.Marks {
				cutBlock.GetText().Marks.Marks[i].Range.From = m.Range.From - req.SelectedTextRange.From
				cutBlock.GetText().Marks.Marks[i].Range.To = m.Range.To - req.SelectedTextRange.From
			}
		}

		cutBlock.GetText().Style = model.BlockContentText_Paragraph
		textSlot = cutBlock.GetText().Text
		s.Set(simple.New(cutBlock))
		htmlSlot = html.NewHTMLConverter(cb.Anytype(), s).Convert()
		textSlot = cutBlock.GetText().Text
		anySlot = cb.stateToBlocks(s)
		return textSlot, htmlSlot, anySlot, nil
	}

	// scenario: ordinary copy
	htmlSlot = html.NewHTMLConverter(cb.Anytype(), s).Convert()
	anySlot = cb.stateToBlocks(s)
	return textSlot, htmlSlot, anySlot, nil
}

func (cb *clipboard) Cut(ctx *state.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	s := cb.NewStateCtx(ctx)

	textSlot = ""

	if len(req.Blocks) == 0 || req.Blocks[0].Id == "" {
		return textSlot, htmlSlot, anySlot, errors.New("nothing to cut")
	}

	var firstTextBlock, lastTextBlock *model.Block
	for _, b := range req.Blocks {
		if b.GetText() != nil {
			if firstTextBlock == nil {
				firstTextBlock = b
			} else {
				lastTextBlock = b
			}
		}
	}

	if req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 && firstTextBlock != nil && lastTextBlock == nil {
		req.SelectedTextRange.To = int32(utf8.RuneCountInString(firstTextBlock.GetText().Text))
	}

	// scenario: rangeCut
	if firstTextBlock != nil &&
		lastTextBlock == nil &&
		req.SelectedTextRange != nil &&
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) {
		first := s.Get(firstTextBlock.Id).(text.Block)
		cutBlock, initialBlock, err := first.RangeCut(req.SelectedTextRange.From, req.SelectedTextRange.To)

		if err != nil {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("error while cut: %s", err)
		}

		first.SetText(initialBlock.GetText().Text, initialBlock.GetText().Marks)

		if cutBlock.GetText() != nil && cutBlock.GetText().Marks != nil {
			for i, m := range cutBlock.GetText().Marks.Marks {
				cutBlock.GetText().Marks.Marks[i].Range.From = m.Range.From - req.SelectedTextRange.From
				cutBlock.GetText().Marks.Marks[i].Range.To = m.Range.To - req.SelectedTextRange.From
			}
		}

		textSlot = cutBlock.GetText().Text
		anySlot = []*model.Block{cutBlock}
		cbs := cb.blocksToState(req.Blocks)
		cbs.Set(simple.New(cutBlock))
		htmlSlot = html.NewHTMLConverter(cb.Anytype(), cbs).Convert()

		return textSlot, htmlSlot, anySlot, cb.Apply(s)
	}

	// scenario: cutBlocks
	var ids []string
	for _, b := range req.Blocks {
		if text := b.GetText(); text != nil {
			textSlot += text.Text + "\n"
		}

		ids = append(ids, b.Id)
	}

	htmlSlot = html.NewHTMLConverter(cb.Anytype(), cb.blocksToState(req.Blocks)).Convert()
	anySlot = req.Blocks

	for _, b := range req.Blocks {
		if b.GetLayout() != nil {
			continue
		}
		ok := s.Unlink(b.Id)
		if !ok {
			return textSlot, htmlSlot, anySlot, fmt.Errorf("can't remove block with id: %s", b.Id)
		}
	}

	return textSlot, htmlSlot, anySlot, cb.Apply(s)
}

func (cb *clipboard) getImages(blocks map[string]*model.Block) (images map[string][]byte, err error) {
	for _, b := range blocks {
		if file := b.GetFile(); file != nil {
			if file.Type == model.BlockContentFile_Image {
				fh, err := cb.Anytype().FileByHash(context.TODO(), file.Hash)
				if err != nil {
					return images, err
				}

				reader, err := fh.Reader()
				if err != nil {
					return images, err
				}

				reader.Read(images[file.Hash])
			}
		}
	}

	return images, nil
}

func (cb *clipboard) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	s := cb.blocksToState(req.Blocks)
	htmlData := html.NewHTMLConverter(cb.Anytype(), s).Export()

	dir := os.TempDir()
	fileName := "export-" + cb.Id() + ".html"
	filePath := filepath.Join(dir, fileName)
	err = ioutil.WriteFile(filePath, []byte(htmlData), 0644)

	if err != nil {
		return "", err
	}
	log.Debug("Export output. filepath:", filepath.Join(dir, fileName))

	return filePath, nil
}

func (cb *clipboard) pasteHtml(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	mdToBlocksConverter := anymark.New()
	err, blocks, _ := mdToBlocksConverter.HTMLToBlocks([]byte(req.HtmlSlot))

	if err != nil {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
	}

	req.AnySlot = blocks
	return cb.pasteAny(ctx, req, groupId)
}

func (cb *clipboard) pasteText(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	if utf8.RuneCountInString(req.TextSlot) == 0 {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, nil
	}

	textArr := strings.Split(req.TextSlot, "\n")

	if len(req.FocusedBlockId) > 0 {
		block := cb.Pick(req.FocusedBlockId)
		if block != nil {
			if b := block.Model().GetText(); b != nil && b.Style == model.BlockContentText_Code {
				textArr = []string{req.TextSlot}
			}
		}
	}

	req.AnySlot = []*model.Block{}
	for i := 0; i < len(textArr); i++ {
		req.AnySlot = append(req.AnySlot, &model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: textArr[i]},
			},
		})
	}

	return cb.pasteAny(ctx, req, groupId)

}

func (cb *clipboard) replaceIds(anySlot []*model.Block) (anySlotreplacedIds []*model.Block) {
	var oldToNew = make(map[string]string)
	for _, b := range anySlot {
		if b.Id == "" {
			b.Id = bson.NewObjectId().Hex()
		}
		oldToNew[b.Id] = bson.NewObjectId().Hex()
	}
	for _, b := range anySlot {
		b.Id = oldToNew[b.Id]
		for i := range b.ChildrenIds {
			b.ChildrenIds[i] = oldToNew[b.ChildrenIds[i]]
		}
	}
	return anySlot
}

func (cb *clipboard) pasteAny(ctx *state.Context, req pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	s := cb.NewStateCtx(ctx).SetGroupId(groupId)
	ctrl := &pasteCtrl{s: s, ps: cb.blocksToState(cb.replaceIds(req.AnySlot))}
	if err = ctrl.Exec(req); err != nil {
		return
	}
	caretPosition = ctrl.caretPos
	uploadArr = ctrl.uploadArr
	return blockIds, uploadArr, caretPosition, isSameBlockCaret, cb.Apply(s)
}

func (cb *clipboard) blocksToState(blocks []*model.Block) (cbs *state.State) {
	cbs = state.NewDoc("cbRoot", nil).(*state.State)
	cbs.SetDetails(cb.Details())
	cbs.Add(simple.New(&model.Block{Id: "cbRoot"}))

	var inChildrens, rootIds []string
	for _, b := range blocks {
		inChildrens = append(inChildrens, b.ChildrenIds...)
	}
	for _, b := range blocks {
		if slice.FindPos(inChildrens, b.Id) == -1 {
			rootIds = append(rootIds, b.Id)
		}
		if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Title{
			t.Style = model.BlockContentText_Header3
		}
		cbs.Add(simple.New(b))
	}
	cbs.Pick(cbs.RootId()).Model().ChildrenIds = rootIds
	cbs.BlocksInit()
	state.CleanupLayouts(cbs)
	cbs.Normalize(false)
	return
}

func (cb *clipboard) stateToBlocks(s *state.State) []*model.Block {
	blocks := s.Blocks()
	result := blocks[:0]
	for _, b := range blocks {
		if b.Id != "cbRoot" {
			result = append(result, b)
		}
	}
	return result
}

func (cb *clipboard) pasteFiles(ctx *state.Context, req pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	s := cb.NewStateCtx(ctx)
	for _, fs := range req.FileSlot {
		b := simple.New(&model.Block{
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name: fs.Name,
				},
			},
		})
		s.Add(b)
		if err = cb.file.UploadState(s, b.Model().Id, file.FileSource{
			Bytes: fs.Data,
			Path:  fs.LocalPath,
			Name:  fs.Name,
		}, true); err != nil {
			return
		}
		blockIds = append(blockIds, b.Model().Id)
	}
	if err = s.InsertTo(req.FocusedBlockId, model.Block_Bottom, blockIds...); err != nil {
		return
	}
	return blockIds, cb.Apply(s)
}
