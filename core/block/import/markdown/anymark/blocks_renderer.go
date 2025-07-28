package anymark

import (
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/text"
)

var (
	markdownLink = regexp.MustCompile(`(?:__|[*#])|\[(.*?)\]\(.*?\)`)
)

type textBlock struct {
	model.Block
	textBuffer      string
	marksBuffer     []*model.BlockContentTextMark
	marksStartQueue []int
}

type blocksRenderer struct {
	baseFilepath      string
	allFileShortPaths []string

	textBuffer       string
	marksBuffer      []*model.BlockContentTextMark
	marksStartQueue  []int
	openedTextBlocks []*textBlock
	blocks           []*model.Block
	rootBlockIDs     []string
	curStyledBlock   model.BlockContentTextStyle

	inTable       bool
	listParentID  string
	listNestIsNum []bool
}

func newBlocksRenderer(baseFilepath string, allFileShortPaths []string, inTable bool) *blocksRenderer {
	return &blocksRenderer{
		baseFilepath:      baseFilepath,
		allFileShortPaths: allFileShortPaths,
		inTable:           inTable,
	}
}

func (r *blocksRenderer) GetAllFileShortPaths() []string {
	return r.allFileShortPaths
}

func (r *blocksRenderer) GetBaseFilepath() string {
	return r.baseFilepath
}

func (r *blocksRenderer) SetMarkStart() {
	if len(r.openedTextBlocks) > 0 {
		last := r.openedTextBlocks[len(r.openedTextBlocks)-1]
		last.marksStartQueue = append(last.marksStartQueue, text.UTF16RuneCountString(last.textBuffer))
		return
	}

	r.marksStartQueue = append(r.marksStartQueue, text.UTF16RuneCountString(r.textBuffer))
}

func (r *blocksRenderer) AddTextByte(b []byte) {
	if len(r.openedTextBlocks) > 0 {
		last := r.openedTextBlocks[len(r.openedTextBlocks)-1]
		last.textBuffer += string(b)
		return
	}

	r.textBuffer += string(b)
}

func (r *blocksRenderer) GetMarkStart() int {
	if len(r.openedTextBlocks) > 0 {
		last := r.openedTextBlocks[len(r.openedTextBlocks)-1]
		if last.marksStartQueue != nil && len(last.marksStartQueue) > 0 {
			return last.marksStartQueue[len(last.marksStartQueue)-1]
		}
		return 0
	}

	if r.marksStartQueue != nil && len(r.marksStartQueue) > 0 {
		return r.marksStartQueue[len(r.marksStartQueue)-1]
	}
	return 0
}

func (r *blocksRenderer) AddMark(mark model.BlockContentTextMark) {
	if len(r.openedTextBlocks) > 0 {
		last := r.openedTextBlocks[len(r.openedTextBlocks)-1]

		// IMPORTANT: ignore if current block is not support markup.
		if last.GetText().Style != model.BlockContentText_Header1 &&
			last.GetText().Style != model.BlockContentText_Header2 &&
			last.GetText().Style != model.BlockContentText_Header3 &&
			last.GetText().Style != model.BlockContentText_Header4 {

			last.marksBuffer = append(last.marksBuffer, &mark)
		}
		return
	}

	s := r.marksStartQueue
	if len(s) > 0 {
		r.marksStartQueue = s[:len(s)-1]
	}

	r.marksBuffer = append(r.marksBuffer, &mark)
}

func (r *blocksRenderer) OpenNewTextBlock(style model.BlockContentTextStyle, fields *types.Struct) {
	if style != model.BlockContentText_Paragraph {
		r.curStyledBlock = style
	}

	id := bson.NewObjectId().Hex()

	newBlock := model.Block{
		Id:     id,
		Fields: fields,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Style: style,
			},
		},
	}

	r.openedTextBlocks = append(r.openedTextBlocks, &textBlock{Block: newBlock})
}

func (r *blocksRenderer) GetBlocks() []*model.Block {
	r.blocks = preprocessBlocks(r.blocks)
	return r.blocks
}

func (r *blocksRenderer) GetRootBlockIDs() []string {
	return r.rootBlockIDs
}

func (r *blocksRenderer) addChildID(cID string) {
	for i := range r.blocks {
		if r.blocks[i].Id == r.listParentID && len(cID) > 0 {
			r.blocks[i].ChildrenIds = append(r.blocks[i].ChildrenIds, cID)
		}
	}
}

func (r *blocksRenderer) SetListState(entering bool, isNumbered bool) {
	if entering {
		r.listNestIsNum = append(r.listNestIsNum, isNumbered)
	} else if len(r.listNestIsNum) > 0 {
		r.listNestIsNum = r.listNestIsNum[:len(r.listNestIsNum)-1]
	}

	if len(r.listNestIsNum) > 1 {
		if len(r.blocks) > 0 {
			r.listParentID = r.blocks[len(r.blocks)-1].Id
		} else {
			r.listParentID = ""
		}
	} else {
		r.listParentID = ""
	}
}

func (r *blocksRenderer) GetIsNumberedList() (isNumbered bool) {
	return r.listNestIsNum[len(r.listNestIsNum)-1]
}

func (r *blocksRenderer) GetText() string {
	if len(r.openedTextBlocks) > 0 {
		last := r.openedTextBlocks[len(r.openedTextBlocks)-1]
		return last.textBuffer
	}
	return r.textBuffer
}

func (r *blocksRenderer) AddTextToBuffer(text string) {
	if len(r.openedTextBlocks) > 0 {
		last := r.openedTextBlocks[len(r.openedTextBlocks)-1]
		last.textBuffer += text
		return
	}

	r.textBuffer += text
}

func (r *blocksRenderer) TextBufferLen() int {
	if len(r.openedTextBlocks) > 0 {
		return len(r.openedTextBlocks[len(r.openedTextBlocks)-1].textBuffer)

	}

	return len(r.textBuffer)
}

func (r *blocksRenderer) AddImageBlock(source string) {
	sourceUnescaped, err := url.PathUnescape(source)
	if err != nil {
		sourceUnescaped = source
	}

	if !isUrl(sourceUnescaped) {
		// Treat as a file path if no URL scheme
		sourceUnescaped = filepath.Join(r.GetBaseFilepath(), sourceUnescaped)
	}

	newBlock := model.Block{
		Id: bson.NewObjectId().Hex(),
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:  sourceUnescaped,
				State: model.BlockContentFile_Empty,
				Type:  model.BlockContentFile_Image,
			}},
	}

	r.blocks = append(r.blocks, &newBlock)
	r.addChildIDToParentBlock(newBlock.Id)
}

func (r *blocksRenderer) AddDivider() {
	r.marksStartQueue = []int{}
	r.marksBuffer = []*model.BlockContentTextMark{}
	r.textBuffer = ""

	divider := &model.Block{
		Id: bson.NewObjectId().Hex(),
		Content: &model.BlockContentOfDiv{
			Div: &model.BlockContentDiv{
				Style: model.BlockContentDiv_Line,
			},
		},
	}
	r.blocks = append(r.blocks, divider)
	r.addChildIDToParentBlock(divider.Id)
}

func isBlockCanHaveChild(block model.Block) bool {
	if t := block.GetText(); t != nil {
		return t.Style == model.BlockContentText_Numbered ||
			t.Style == model.BlockContentText_Marked ||
			t.Style == model.BlockContentText_Toggle ||
			t.Style == model.BlockContentText_Quote ||
			t.Style == model.BlockContentText_Checkbox
	}

	return false
}

func (r *blocksRenderer) openTextBlockWithStyle(entering bool, style model.BlockContentTextStyle, fields *types.Struct) {
	if r.inTable {
		style = model.BlockContentText_Paragraph
	}
	if entering {
		r.OpenNewTextBlock(style, fields)
	} else {
		r.CloseTextBlock(style)
	}
}

func (r *blocksRenderer) CloseTextBlock(content model.BlockContentTextStyle) {
	var style = content
	var closingBlock *textBlock
	var parentBlock *textBlock

	id := bson.NewObjectId().Hex()

	if len(r.openedTextBlocks) > 0 {
		closingBlock = r.openedTextBlocks[len(r.openedTextBlocks)-1]
		r.openedTextBlocks = r.openedTextBlocks[:len(r.openedTextBlocks)-1]

		for i := len(r.openedTextBlocks) - 1; i >= 0; i-- {
			if isBlockCanHaveChild(r.openedTextBlocks[i].Block) {
				parentBlock = r.openedTextBlocks[i]

				break
			}
		}
	}

	if closingBlock == nil {
		closingBlock = &textBlock{
			Block: model.Block{
				Id: id,
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{},
				},
			},
			textBuffer:      r.textBuffer,
			marksBuffer:     r.marksBuffer,
			marksStartQueue: r.marksStartQueue,
		}
		r.textBuffer = ""
		r.marksBuffer = []*model.BlockContentTextMark{}
		r.marksStartQueue = []int{}
	}

	if style == r.curStyledBlock {
		r.curStyledBlock = model.BlockContentText_Paragraph
	} else if r.curStyledBlock != model.BlockContentText_Paragraph {
		style = r.curStyledBlock
	}

	r.ProcessMarkdownArtifacts()
	t := closingBlock.GetText()
	if t.Marks == nil || len(t.Marks.Marks) == 0 {
		t.Marks = &model.BlockContentTextMarks{
			Marks: closingBlock.marksBuffer,
		}
	}

	if t.Text == "" {
		t.Text = closingBlock.textBuffer
	}

	switch {
	case strings.HasPrefix(t.Text, "[ ]"):
		parentBlock = r.normalizeCheckboxBlock(t, parentBlock, "[ ]")
	case strings.HasPrefix(t.Text, "[]"):
		parentBlock = r.normalizeCheckboxBlock(t, parentBlock, "[]")
	case strings.HasPrefix(t.Text, "[x]"):
		parentBlock = r.normalizeCheckboxBlock(t, parentBlock, "[x]")
		t.Checked = true
	}

	if parentBlock != nil {
		if parentText := parentBlock.GetText(); parentText != nil && parentText.Text == "" &&
			!isBlockCanHaveChild(closingBlock.Block) && t.Text != "" {
			parentText.Marks = t.Marks
			parentText.Checked = t.Checked
			parentText.Color = t.Color
			parentText.Text = t.Text
			t.Text = ""
		} else {
			parentBlock.ChildrenIds = append(parentBlock.ChildrenIds, closingBlock.Id)
		}
	}

	// IMPORTANT: do not create a new block if textBuffer is empty
	if len(t.Text) > 0 || len(closingBlock.ChildrenIds) > 0 || isBlockCanHaveChild(closingBlock.Block) ||
		t.Style == model.BlockContentText_Checkbox {
		// Nested list case:
		if len(r.listParentID) > 0 {
			r.addChildID(id)
		}

		r.blocks = append(r.blocks, &(closingBlock.Block))
	}
}

func (r *blocksRenderer) normalizeCheckboxBlock(t *model.BlockContentText, parentBlock *textBlock, checkboxPattern string) *textBlock {
	textBefore := t.Text
	t.Text = strings.TrimLeft(strings.TrimPrefix(textBefore, checkboxPattern), " ")
	r.adjustMarkdownRange(t, len(textBefore)-len(t.Text))
	t.Style = model.BlockContentText_Checkbox
	return r.removeMarkedBlock(parentBlock)
}

// removeMarkedBlock removes unnecessary marked block because goldmark parses notion checkbox as marked block,
// because of "-" before checkbox (-[x]/-[]), so we remove this block and makes parent block nil
// if parent block is the removed block
func (r *blocksRenderer) removeMarkedBlock(parentBlock *textBlock) *textBlock {
	if len(r.openedTextBlocks) == 0 {
		return parentBlock
	}
	markedBlock := r.openedTextBlocks[len(r.openedTextBlocks)-1]
	if markedBlock != nil && r.isEmptyMarkedBlock(markedBlock) {
		r.openedTextBlocks = r.openedTextBlocks[:len(r.openedTextBlocks)-1]
		if reflect.DeepEqual(parentBlock, markedBlock) {
			parentBlock = nil
		}
	}
	return parentBlock
}

func (r *blocksRenderer) isEmptyMarkedBlock(markedBlock *textBlock) bool {
	return markedBlock.GetText() != nil && markedBlock.GetText().Text == "" &&
		markedBlock.GetText().Style == model.BlockContentText_Marked && markedBlock.textBuffer == ""
}

func (r *blocksRenderer) adjustMarkdownRange(t *model.BlockContentText, adjustNumber int) {
	for _, mark := range t.Marks.Marks {
		if mark.Range != nil {
			mark.Range.To -= int32(adjustNumber)
			mark.Range.From -= int32(adjustNumber)
			if mark.Range.From < 0 {
				mark.Range.From = 0
			}
		}
	}
}

func (r *blocksRenderer) addChildIDToParentBlock(id string) {
	if len(r.openedTextBlocks) > 0 {
		var parentBlock *textBlock
		for i := len(r.openedTextBlocks) - 1; i >= 0; i-- {
			if isBlockCanHaveChild(r.openedTextBlocks[i].Block) {
				parentBlock = r.openedTextBlocks[i]
				break
			}
		}
		if parentBlock != nil {
			parentBlock.ChildrenIds = append(parentBlock.ChildrenIds, id)
		}
	}
}

func (r *blocksRenderer) ForceCloseTextBlock() {
	s := r.openedTextBlocks
	style := model.BlockContentText_Paragraph

	if len(r.openedTextBlocks) > 0 {
		style, r.openedTextBlocks = s[len(s)-1].GetText().Style, s[:len(s)-1]
	}
	r.openTextBlockWithStyle(false, style, nil)
}

func (r *blocksRenderer) ProcessMarkdownArtifacts() {
	res := markdownLink.FindAllStringSubmatchIndex(r.textBuffer, -1)
	if len(res) != 0 {
		for i := range res {
			fmt.Println(res[i])
		}
	}
}
