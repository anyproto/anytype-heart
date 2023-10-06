package clipboard

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark/whitespace"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/converter/html"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/objectorigin"
	"github.com/anyproto/anytype-heart/util/slice"
	"github.com/anyproto/anytype-heart/util/strutil"
	textutil "github.com/anyproto/anytype-heart/util/text"
)

var (
	ErrAllSlotsEmpty = errors.New("all slots are empty")
	log              = logging.Logger("anytype-clipboard")
)

type Clipboard interface {
	Cut(ctx session.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Paste(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error)
	Copy(ctx session.Context, req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error)
	Export(req pb.RpcBlockExportRequest) (path string, err error)
}

func NewClipboard(
	sb smartblock.SmartBlock,
	file file.File,
	tempDirProvider core.TempDirProvider,
	systemObjectService system_object.Service,
	fileService files.Service,
) Clipboard {
	return &clipboard{
		SmartBlock:          sb,
		file:                file,
		tempDirProvider:     tempDirProvider,
		systemObjectService: systemObjectService,
		fileService:         fileService,
	}
}

type clipboard struct {
	smartblock.SmartBlock
	file                file.File
	tempDirProvider     core.TempDirProvider
	systemObjectService system_object.Service
	fileService         files.Service
}

func (cb *clipboard) Paste(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
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

func (cb *clipboard) Copy(ctx session.Context, req pb.RpcBlockCopyRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	anySlot = req.Blocks
	textSlot = ""
	htmlSlot = ""

	if len(req.Blocks) == 0 {
		return textSlot, htmlSlot, anySlot, fmt.Errorf("copy: no blocks")
	}

	s := cb.blocksToState(req.Blocks)

	textSlot = renderText(s)

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
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == int32(textutil.UTF16RuneCountString(firstTextBlock.GetText().Text))) &&
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
		htmlSlot = html.NewHTMLConverter(cb.SpaceID(), cb.fileService, s).Convert()
		textSlot = cutBlock.GetText().Text
		anySlot = cb.stateToBlocks(s)
		return textSlot, htmlSlot, anySlot, nil
	}

	// scenario: ordinary copy
	htmlSlot = html.NewHTMLConverter(cb.SpaceID(), cb.fileService, s).Convert()
	anySlot = cb.stateToBlocks(s)
	return textSlot, htmlSlot, anySlot, nil
}

func (cb *clipboard) Cut(ctx session.Context, req pb.RpcBlockCutRequest) (textSlot string, htmlSlot string, anySlot []*model.Block, err error) {
	s := cb.NewStateCtx(ctx)
	textSlot = ""

	stateBlocks, err := assertBlocks(s.Blocks(), req.Blocks)
	if err != nil {
		return textSlot, htmlSlot, anySlot, err
	}

	var firstTextBlock, lastTextBlock *model.Block
	for _, b := range req.Blocks {
		if b.GetText() != nil {
			if firstTextBlock == nil {
				firstTextBlock = b
			} else {
				lastTextBlock = b
			}
		} else {
			// if text block + object block - go to cutBlocks scenario imediately
			firstTextBlock = nil
			lastTextBlock = nil
			break
		}
	}

	if req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0 && firstTextBlock != nil && lastTextBlock == nil {
		req.SelectedTextRange.To = int32(textutil.UTF16RuneCountString(firstTextBlock.GetText().Text))
	}

	// scenario: rangeCut
	if firstTextBlock != nil &&
		lastTextBlock == nil &&
		req.SelectedTextRange != nil &&
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == 0) &&
		!(req.SelectedTextRange.From == 0 && req.SelectedTextRange.To == int32(textutil.UTF16RuneCountString(firstTextBlock.GetText().Text))) {
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

		cutBlock.GetText().Style = model.BlockContentText_Paragraph
		textSlot = cutBlock.GetText().Text
		anySlot = []*model.Block{cutBlock}
		cbs := cb.blocksToState(req.Blocks)
		cbs.Set(simple.New(cutBlock))
		htmlSlot = html.NewHTMLConverter(cb.SpaceID(), cb.fileService, cbs).Convert()

		return textSlot, htmlSlot, anySlot, cb.Apply(s)
	}

	// scenario: cutBlocks
	state := cb.blocksToState(req.Blocks)
	var ids []string
	for _, b := range req.Blocks {
		ids = append(ids, b.Id)
	}
	textSlot = renderText(state)

	htmlSlot = html.NewHTMLConverter(cb.SpaceID(), cb.fileService, state).Convert()
	anySlot = req.Blocks

	unlinkAndClearBlocks(s, stateBlocks, req.Blocks)
	return textSlot, htmlSlot, anySlot, cb.Apply(s)
}

func unlinkAndClearBlocks(
	s *state.State,
	stateBlocks map[string]*model.Block,
	requestBlocks []*model.Block,
) {
	for _, block := range requestBlocks {
		if block.GetLayout() != nil {
			continue
		}
		stateBlock := stateBlocks[block.Id]
		if stateBlock.Restrictions == nil || !stateBlock.Restrictions.Remove {
			s.Unlink(block.Id)
		} else {
			if textBlock, ok := s.Get(block.Id).(text.Block); ok {
				textBlock.SetText("", nil)
			}
		}
	}
}

func assertBlocks(stateBlocks []*model.Block, requestBlocks []*model.Block) (map[string]*model.Block, error) {
	if len(requestBlocks) == 0 || requestBlocks[0].Id == "" {
		return nil, errors.New("nothing to cut")
	}

	idToBlockMap := make(map[string]*model.Block)
	for _, stateBlock := range stateBlocks {
		idToBlockMap[stateBlock.Id] = stateBlock
	}

	for _, requestBlock := range requestBlocks {
		if requestBlock.Id == "" {
			return nil, errors.New("empty requestBlock id")
		}
		if stateBlock, ok := idToBlockMap[requestBlock.Id]; !ok {
			return nil, fmt.Errorf("requestBlock with id %s not found", stateBlock.Id)
		}
	}
	return idToBlockMap, nil
}

func (cb *clipboard) Export(req pb.RpcBlockExportRequest) (path string, err error) {
	s := cb.blocksToState(req.Blocks)
	htmlData := html.NewHTMLConverter(cb.SpaceID(), cb.fileService, s).Export()

	dir := cb.tempDirProvider.TempDir()
	fileName := "export-" + cb.Id() + ".html"
	filePath := filepath.Join(dir, fileName)
	err = ioutil.WriteFile(filePath, []byte(htmlData), 0644)

	if err != nil {
		return "", err
	}

	return filePath, nil
}

func (cb *clipboard) pasteHtml(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	blocks, _, err := anymark.HTMLToBlocks([]byte(req.HtmlSlot))

	if err != nil {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, err
	}

	// See GO-250 for more details
	// In short: if we paste plaintext blocks into a styled block, we make first ones to inherit style from this block
	if focused := cb.Pick(req.FocusedBlockId); focused != nil {
		if focusedTxt := focused.Model().GetText(); focusedTxt != nil && focusedTxt.Style != model.BlockContentText_Paragraph {
			for _, b := range blocks {
				if txt := b.GetText(); txt != nil && txt.Style == model.BlockContentText_Paragraph {
					txt.Style = focusedTxt.Style
				} else {
					break
				}
			}
		}
	}

	req.AnySlot = blocks
	return cb.pasteAny(ctx, req, groupId)
}

func (cb *clipboard) pasteText(ctx session.Context, req *pb.RpcBlockPasteRequest, groupId string) (blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error) {
	if len(req.TextSlot) == 0 {
		return blockIds, uploadArr, caretPosition, isSameBlockCaret, nil
	}

	textArr := strings.Split(req.TextSlot, "\n")

	if !req.IsPartOfBlock && len(textArr) == 1 && len(req.SelectedBlockIds) <= 1 {
		req.IsPartOfBlock = true
	}

	if len(req.FocusedBlockId) > 0 {
		block := cb.Pick(req.FocusedBlockId)
		if block != nil {
			if b := block.Model().GetText(); b != nil && b.Style == model.BlockContentText_Code {
				return cb.pasteRawText(ctx, req, []string{req.TextSlot}, groupId)
			}
		}
	}

	mdText := whitespace.WhitespaceNormalizeString(req.TextSlot)

	blocks, _, err := anymark.MarkdownToBlocks([]byte(mdText), "", []string{})
	if err != nil {
		return cb.pasteRawText(ctx, req, textArr, groupId)
	}
	req.AnySlot = append(req.AnySlot, blocks...)

	return cb.pasteAny(ctx, req, groupId)
}

func (cb *clipboard) pasteRawText(ctx session.Context, req *pb.RpcBlockPasteRequest, textArr []string, groupId string) ([]string, []pb.RpcBlockUploadRequest, int32, bool, error) {
	req.AnySlot = make([]*model.Block, 0, len(textArr))
	for _, text := range textArr {
		if text != "" {
			req.AnySlot = append(req.AnySlot, &model.Block{
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Text: text},
				},
			})
		}
	}
	return cb.pasteAny(ctx, req, groupId)
}

// some types of blocks need a special duplication mechanism
type duplicatable interface {
	Duplicate(s *state.State) (newId string, visitedIds []string, blocks []simple.Block, err error)
}

func (cb *clipboard) pasteAny(
	ctx session.Context, req *pb.RpcBlockPasteRequest, groupID string,
) (
	blockIds []string, uploadArr []pb.RpcBlockUploadRequest, caretPosition int32, isSameBlockCaret bool, err error,
) {
	s := cb.NewStateCtx(ctx).SetGroupId(groupID)

	destState := state.NewDoc("", nil).(*state.State)

	for _, b := range req.AnySlot {
		if b.Id == "" {
			b.Id = bson.NewObjectId().Hex()
		}
		if b.Id == template.TitleBlockId || b.Id == template.DescriptionBlockId {
			delete(b.Fields.Fields, text.DetailsKeyFieldName)
		}
		if d, ok := b.Content.(*model.BlockContentOfDataview); ok {
			if err = cb.addRelationLinksToDataview(d.Dataview); err != nil {
				return
			}
		}
	}
	srcState := cb.blocksToState(req.AnySlot)
	visited := map[string]struct{}{}

	src := srcState.Blocks()
	srcBlocks := make([]simple.Block, 0, len(src))
	for _, b := range src {
		srcBlocks = append(srcBlocks, simple.New(b))
	}

	oldToNew := map[string]string{}
	// Handle blocks that have custom duplication code. For example, simple tables
	// have to have special ID for cells
	for _, b := range srcBlocks {
		if d, ok := b.(duplicatable); ok {
			id, visitedIds, blocks, err2 := d.Duplicate(srcState)
			if err2 != nil {
				err = fmt.Errorf("custom duplicate: %w", err2)
				return
			}

			oldToNew[b.Model().Id] = id
			for _, b := range blocks {
				destState.Add(b)
			}
			for _, id := range visitedIds {
				visited[id] = struct{}{}
			}
		}
	}

	// Collect and generate necessary IDs. Ignore ids of blocks that have been duplicated by custom code
	for _, b := range srcBlocks {
		if _, ok := visited[b.Model().Id]; ok {
			continue
		}
		oldToNew[b.Model().Id] = bson.NewObjectId().Hex()
	}

	// Remap IDs
	for _, b := range srcBlocks {
		if _, ok := visited[b.Model().Id]; ok {
			continue
		}
		b.Model().Id = oldToNew[b.Model().Id]
		for i, id := range b.Model().ChildrenIds {
			b.Model().ChildrenIds[i] = oldToNew[id]
		}
		destState.Add(b)
	}

	destState.BlocksInit(destState)
	state.CleanupLayouts(destState)
	if err = destState.Normalize(false); err != nil {
		return
	}

	relationLinks := destState.GetRelationLinks()
	var missingRelationKeys []string

	// collect missing relation keys to add it to state
	for _, b := range s.Blocks() {
		if r := b.GetRelation(); r != nil {
			if !relationLinks.Has(r.Key) {
				missingRelationKeys = append(missingRelationKeys, r.Key)
			}
		}
	}

	ctrl := &pasteCtrl{s: s, ps: destState}
	if err = ctrl.Exec(req); err != nil {
		return
	}
	caretPosition = ctrl.caretPos
	uploadArr = ctrl.uploadArr

	if len(missingRelationKeys) > 0 {
		if err = cb.AddRelationLinksToState(s, missingRelationKeys...); err != nil {
			return
		}
	}

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
		cbs.Add(simple.New(b))
	}
	cbs.Pick(cbs.RootId()).Model().ChildrenIds = rootIds
	cbs.BlocksInit(cbs)
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

func (cb *clipboard) pasteFiles(ctx session.Context, req *pb.RpcBlockPasteRequest) (blockIds []string, err error) {
	s := cb.NewStateCtx(ctx)
	for _, fs := range req.FileSlot {
		b := simple.New(&model.Block{
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{
					Name:  fs.Name,
					Style: model.BlockContentFile_Auto,
				},
			},
		})
		s.Add(b)
		if err = cb.file.UploadState(ctx, s, b.Model().Id, file.FileSource{
			Bytes:  fs.Data,
			Path:   fs.LocalPath,
			Name:   fs.Name,
			Origin: objectorigin.Ptr(model.ObjectOrigin_clipboard),
		}, false); err != nil {
			return
		}
		blockIds = append(blockIds, b.Model().Id)
	}

	if err = s.InsertTo(req.FocusedBlockId, cb.getFileBlockPosition(req), blockIds...); err != nil {
		return
	}
	return blockIds, cb.Apply(s)
}

func (cb *clipboard) getFileBlockPosition(req *pb.RpcBlockPasteRequest) model.BlockPosition {
	b := cb.Pick(req.FocusedBlockId)
	if b == nil {
		return model.Block_Bottom
	}
	if txt := b.Model().GetText(); txt != nil && txt.Text == "" {
		return model.Block_Replace
	}
	return model.Block_Bottom
}

func (cb *clipboard) addRelationLinksToDataview(d *model.BlockContentDataview) (err error) {
	relationKeys := make(map[string]struct{})
	if len(d.RelationLinks) != 0 || len(d.Views) == 0 {
		return
	}
	for _, v := range d.Views {
		for _, r := range v.Relations {
			if _, found := relationKeys[r.Key]; !found {
				relationKeys[r.Key] = struct{}{}
			}
		}
	}
	if len(relationKeys) == 0 {
		return
	}

	relationKeysList := make([]string, len(relationKeys))
	for k := range relationKeys {
		relationKeysList = append(relationKeysList, k)
	}
	relations, err := cb.systemObjectService.FetchRelationByKeys(cb.SpaceID(), relationKeysList...)
	if err != nil {
		return fmt.Errorf("failed to fetch relation keys of dataview: %v", err)
	}
	links := make([]*model.RelationLink, 0, len(relations))
	for _, r := range relations {
		links = append(links, r.RelationLink())
	}

	d.RelationLinks = links
	return
}

func renderText(s *state.State) string {
	texts := make([]string, 0)
	texts, _ = renderBlock(s, texts, s.RootId(), -1, 0)

	if len(texts) > 0 {
		return strutil.JoinWithTrailingEnd(texts, "\n")
	}

	return ""
}

func renderBlock(s *state.State, texts []string, id string, level int, numberedCount int) ([]string, int) {
	block := s.Pick(id).Model()
	texts, numberedCount = extractTextWithStyleAndTabs(block, texts, level, numberedCount)
	childrenIds := s.Pick(id).Model().ChildrenIds
	texts = renderChildren(s, texts, childrenIds, level, 0)
	return texts, numberedCount
}

func renderChildren(s *state.State, texts []string, childrenIds []string, level int, numberedCount int) []string {
	var oldNumberedCount int
	for _, id := range childrenIds {
		oldNumberedCount = numberedCount
		texts, numberedCount = renderBlock(s, texts, id, level+1, numberedCount)
		if oldNumberedCount == numberedCount {
			numberedCount = 0
		}
	}
	return texts
}

func extractTextWithStyleAndTabs(block *model.Block, texts []string, level int, numberedCount int) ([]string, int) {
	if text := block.GetText(); text != nil {
		switch text.Style {
		case model.BlockContentText_Quote:
			texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "> ", text.Text))
		case model.BlockContentText_Code:
			texts = append(texts, fmt.Sprintf("%s%s%s%s", strings.Repeat("\t", level), "```", text.Text, "```"))
		case model.BlockContentText_Checkbox:
			texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "- [ ] ", text.Text))
		case model.BlockContentText_Marked:
			texts = append(texts, fmt.Sprintf("%s%s%s", strings.Repeat("\t", level), "- ", text.Text))
		case model.BlockContentText_Numbered:
			numberedCount++
			texts = append(texts, fmt.Sprintf("%s%d%s%s", strings.Repeat("\t", level), numberedCount, ". ", text.Text))
		case model.BlockContentText_Callout:
			texts = append(texts, fmt.Sprintf("%s%s%s%s", strings.Repeat("\t", level), text.IconEmoji, " ", text.Text))
		default:
			texts = append(texts, fmt.Sprintf("%s%s", strings.Repeat("\t", level), text.Text))
		}
	}
	return texts, numberedCount
}
