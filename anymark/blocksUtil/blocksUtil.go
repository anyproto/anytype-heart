// Package renderer renders the given AST to certain formats.
package blocksUtil

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/anyblocks"
)

var (
	markdownLink = regexp.MustCompile(`(?:__|[*#])|\[(.*?)\]\(.*?\)`)
)

// A RWriter is a subset of the bufio.Writer .
type RWriter interface {
	// TODO: LEGACY, remove it later
	io.Writer
	Available() int
	Buffered() int
	Flush() error
	WriteByte(c byte) error
	WriteRune(r rune) (size int, err error)
	WriteString(s string) (int, error)

	// Main part
	GetText() string
	AddTextToBuffer(s string)
	AddTextByte(b []byte)

	GetRootBlockIDs() []string
	GetBlocks() []*model.Block

	GetMarkStart() int
	SetMarkStart()

	AddMark(mark model.BlockContentTextMark)

	ProcessMarkdownArtifacts()

	AddImageBlock(url string)
	OpenNewTextBlock(model.BlockContentTextStyle)
	CloseTextBlock(model.BlockContentTextStyle)
	ForceCloseTextBlock()

	SetIsNumberedList(isNumbered bool)
	GetIsNumberedList() (isNumbered bool)

	GetAllFileShortPaths() []string
	GetBaseFilepath() string

	AddDivider()
}

type textBlock struct {
	model.Block
	textBuffer      string
	marksBuffer     []*model.BlockContentTextMark
	marksStartQueue []int
}

type rWriter struct {
	*bufio.Writer
	baseFilepath      string
	allFileShortPaths []string

	// is next added list will be a numbered one
	isNumberedList   bool
	textBuffer       string
	marksBuffer      []*model.BlockContentTextMark
	marksStartQueue  []int
	openedTextBlocks []*textBlock
	blocks           []*model.Block
	rootBlockIDs     []string
	curStyledBlock   model.BlockContentTextStyle
	idGetter         fmt.Stringer
}

func (rw *rWriter) GetAllFileShortPaths() []string {
	return rw.allFileShortPaths
}

func (rw *rWriter) GetBaseFilepath() string {
	return rw.baseFilepath
}

func (rw *rWriter) SetMarkStart() {
	if len(rw.openedTextBlocks) > 0 {
		last := rw.openedTextBlocks[len(rw.openedTextBlocks)-1]
		last.marksStartQueue = append(last.marksStartQueue, utf8.RuneCountInString(last.textBuffer))
		return
	}

	rw.marksStartQueue = append(rw.marksStartQueue, utf8.RuneCountInString(rw.textBuffer))
}

func (rw *rWriter) AddTextByte(b []byte) {
	if len(rw.openedTextBlocks) > 0 {
		last := rw.openedTextBlocks[len(rw.openedTextBlocks)-1]
		last.textBuffer += string(b)
		return
	}

	rw.textBuffer += string(b)
}

func (rw *rWriter) GetMarkStart() int {
	if len(rw.openedTextBlocks) > 0 {
		last := rw.openedTextBlocks[len(rw.openedTextBlocks)-1]
		if last.marksStartQueue != nil && len(last.marksStartQueue) > 0 {
			return last.marksStartQueue[len(last.marksStartQueue)-1]
		} else {
			return 0
		}
	}

	if rw.marksStartQueue != nil && len(rw.marksStartQueue) > 0 {
		return rw.marksStartQueue[len(rw.marksStartQueue)-1]
	} else {
		return 0
	}
}

func (rw *rWriter) AddMark(mark model.BlockContentTextMark) {
	if len(rw.openedTextBlocks) > 0 {
		last := rw.openedTextBlocks[len(rw.openedTextBlocks)-1]

		// IMPORTANT: ignore if current block is not support markup.
		if last.GetText().Style != model.BlockContentText_Header1 &&
			last.GetText().Style != model.BlockContentText_Header2 &&
			last.GetText().Style != model.BlockContentText_Header3 &&
			last.GetText().Style != model.BlockContentText_Header4 {

			last.marksBuffer = append(last.marksBuffer, &mark)
		}
		return
	}

	s := rw.marksStartQueue
	if len(s) > 0 {
		rw.marksStartQueue = s[:len(s)-1]
	}

	rw.marksBuffer = append(rw.marksBuffer, &mark)
}

func (rw *rWriter) OpenNewTextBlock(style model.BlockContentTextStyle) {
	if style != model.BlockContentText_Paragraph {
		rw.curStyledBlock = style
	}

	id := rw.idGetter.String()

	newBlock := model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Style: style,
			},
		},
	}

	rw.openedTextBlocks = append(rw.openedTextBlocks, &textBlock{Block: newBlock})
}

func (rw *rWriter) GetBlocks() []*model.Block {
	rw.blocks = anyblocks.PreprocessBlocks(rw.blocks)
	return rw.blocks
}

func (rw *rWriter) GetRootBlockIDs() []string {
	return rw.rootBlockIDs
}

func (rw *rWriter) SetIsNumberedList(isNumbered bool) {
	rw.isNumberedList = isNumbered
}

func (rw *rWriter) GetIsNumberedList() (isNumbered bool) {
	return rw.isNumberedList
}

func NewRWriter(writer *bufio.Writer, baseFilepath string, allFileShortPaths []string, idGetter fmt.Stringer) RWriter {
	return &rWriter{Writer: writer, baseFilepath: baseFilepath, allFileShortPaths: allFileShortPaths, idGetter: idGetter}
}

func (rw *rWriter) GetText() string {
	if len(rw.openedTextBlocks) > 0 {
		last := rw.openedTextBlocks[len(rw.openedTextBlocks)-1]
		return last.textBuffer
	}
	return rw.textBuffer
}

func (rw *rWriter) AddTextToBuffer(text string) {
	if len(rw.openedTextBlocks) > 0 {
		last := rw.openedTextBlocks[len(rw.openedTextBlocks)-1]
		last.textBuffer += strings.ReplaceAll(text, "*", "")
		return
	}

	rw.textBuffer += strings.ReplaceAll(text, "*", "")
}

func (rw *rWriter) AddImageBlock(source string) {
	sourceUnescaped, err := url.PathUnescape(source)
	if err != nil {
		sourceUnescaped = source
	}

	if !strings.HasPrefix(strings.ToLower(source), "http://") && !strings.HasPrefix(strings.ToLower(source), "https://") {
		sourceUnescaped = filepath.Join(rw.GetBaseFilepath(), sourceUnescaped)
	}

	newBlock := model.Block{
		Content: &model.BlockContentOfFile{

			File: &model.BlockContentFile{
				Name:  sourceUnescaped,
				State: model.BlockContentFile_Empty,
				Type:  model.BlockContentFile_Image,
			}},
	}

	rw.blocks = append(rw.blocks, &newBlock)
}

func (rw *rWriter) AddDivider() {
	rw.marksStartQueue = []int{}
	rw.marksBuffer = []*model.BlockContentTextMark{}
	rw.textBuffer = ""

	rw.blocks = append(rw.blocks, &model.Block{
		Content: &model.BlockContentOfDiv{
			Div: &model.BlockContentDiv{
				Style: model.BlockContentDiv_Line,
			},
		},
	})
}

func isBlockCanHaveChild(block model.Block) bool {
	if text := block.GetText(); text != nil {
		return text.Style == model.BlockContentText_Numbered ||
			text.Style == model.BlockContentText_Marked ||
			text.Style == model.BlockContentText_Toggle
	}

	return false
}

func (rw *rWriter) CloseTextBlock(content model.BlockContentTextStyle) {
	var style = content
	var closingBlock *textBlock
	var parentBlock *textBlock

	if len(rw.openedTextBlocks) > 0 {
		closingBlock = rw.openedTextBlocks[len(rw.openedTextBlocks)-1]
		rw.openedTextBlocks = rw.openedTextBlocks[:len(rw.openedTextBlocks)-1]

		for i := len(rw.openedTextBlocks) - 1; i >= 0; i-- {
			if isBlockCanHaveChild(rw.openedTextBlocks[i].Block) {
				parentBlock = rw.openedTextBlocks[i]

				break
			}
		}

	}

	if closingBlock == nil {
		closingBlock = &textBlock{
			Block: model.Block{
				Id: rw.idGetter.String(),
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{},
				},
			},
			textBuffer:      rw.textBuffer,
			marksBuffer:     rw.marksBuffer,
			marksStartQueue: rw.marksStartQueue,
		}
		rw.textBuffer = ""
		rw.marksBuffer = []*model.BlockContentTextMark{}
		rw.marksStartQueue = []int{}
	}

	if style == rw.curStyledBlock {
		rw.curStyledBlock = model.BlockContentText_Paragraph
	} else if rw.curStyledBlock != model.BlockContentText_Paragraph {
		style = rw.curStyledBlock
	}

	rw.ProcessMarkdownArtifacts()
	text := closingBlock.GetText()
	if text.Marks == nil || len(text.Marks.Marks) == 0 {
		text.Marks = &model.BlockContentTextMarks{
			Marks: closingBlock.marksBuffer,
		}
	}

	if text.Text == "" {
		text.Text = closingBlock.textBuffer
	}

	if len(text.Text) >= 3 && text.Text[:3] == "[ ]" {
		text.Text = strings.TrimLeft(text.Text[3:], " ")
		text.Style = model.BlockContentText_Checkbox

	} else if len(text.Text) >= 2 && text.Text[:2] == "[]" {
		text.Text = strings.TrimLeft(text.Text[2:], " ")
		text.Style = model.BlockContentText_Checkbox

	} else if len(text.Text) >= 3 && text.Text[:3] == "[x]" {
		text.Text = strings.TrimLeft(text.Text[3:], " ")
		text.Style = model.BlockContentText_Checkbox
		text.Checked = true
	}

	if parentBlock != nil {
		if parentText := parentBlock.GetText(); parentText != nil && parentText.Text == "" && !isBlockCanHaveChild(closingBlock.Block) && text.Text != "" {
			parentText.Marks = text.Marks
			parentText.Checked = text.Checked
			parentText.Color = text.Color
			parentText.Text = text.Text
			text.Text = ""
		} else {
			parentBlock.ChildrenIds = append(parentBlock.ChildrenIds, closingBlock.Id)
		}
	}

	// IMPORTANT: do not create a new block if textBuffer is empty
	if len(text.Text) > 0 || len(closingBlock.ChildrenIds) > 0 {
		rw.blocks = append(rw.blocks, &(closingBlock.Block))
	}
}

func (rw *rWriter) ForceCloseTextBlock() {
	s := rw.openedTextBlocks
	style := model.BlockContentText_Paragraph

	if len(rw.openedTextBlocks) > 0 {
		style, rw.openedTextBlocks = s[len(s)-1].GetText().Style, s[:len(s)-1]
	}

	rw.CloseTextBlock(style)
}

func (rw *rWriter) ProcessMarkdownArtifacts() {
	res := markdownLink.FindAllStringSubmatchIndex(rw.textBuffer, -1)
	if len(res) != 0 {
		for i, _ := range res {
			fmt.Println(res[i])
		}
	}
}
