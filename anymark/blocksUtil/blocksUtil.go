// Package renderer renders the given AST to certain formats.
package blocksUtil

import (
	"bufio"
	"io"
	"unicode/utf8"

	"github.com/anytypeio/go-anytype-library/pb/model"
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

	GetBlocks() []*model.Block

	GetMarkStart() int
	SetMarkStart()

	AddMark(mark model.BlockContentTextMark)

	AddImageBlock(url string)
	OpenNewTextBlock(model.BlockContentTextStyle)
	CloseTextBlock(model.BlockContentTextStyle)
	ForceCloseTextBlock()

	SetIsNumberedList(isNumbered bool)
	GetIsNumberedList() (isNumbered bool)

	GetAllFileShortPaths() []string
	AddDivider()
}

type rWriter struct {
	*bufio.Writer
	allFileShortPaths []string

	isNumberedList bool

	textBuffer      string
	marksBuffer     []*model.BlockContentTextMark
	marksStartQueue []int
	textStylesQueue []model.BlockContentTextStyle
	blocks          []*model.Block
	curStyledBlock  model.BlockContentTextStyle
}

func (rw *rWriter) GetAllFileShortPaths() []string {
	return rw.allFileShortPaths
}

func (rw *rWriter) SetMarkStart() {
	rw.marksStartQueue = append(rw.marksStartQueue, utf8.RuneCountInString(rw.textBuffer))
}

func (rw *rWriter) AddTextByte(b []byte) {
	rw.textBuffer += string(b)
}

func (rw *rWriter) GetMarkStart() int {
	if rw.marksStartQueue != nil && len(rw.marksStartQueue) > 0 {
		return rw.marksStartQueue[len(rw.marksStartQueue)-1]
	} else {
		return 0
	}
}

func (rw *rWriter) AddMark(mark model.BlockContentTextMark) {
	s := rw.marksStartQueue

	if len(s) > 0 {
		rw.marksStartQueue = s[:len(s)-1]
	}

	if len(rw.textStylesQueue) > 0 {
		// IMPORTANT: ignore if current block is not support markup.
		if rw.textStylesQueue != nil && len(rw.textStylesQueue) > 0 &&
			rw.textStylesQueue[len(rw.textStylesQueue)-1] != model.BlockContentText_Header1 &&
			rw.textStylesQueue[len(rw.textStylesQueue)-1] != model.BlockContentText_Header2 &&
			rw.textStylesQueue[len(rw.textStylesQueue)-1] != model.BlockContentText_Header3 &&
			rw.textStylesQueue[len(rw.textStylesQueue)-1] != model.BlockContentText_Header4 {

			rw.marksBuffer = append(rw.marksBuffer, &mark)
		}
	} else {
		rw.marksBuffer = append(rw.marksBuffer, &mark)
	}
}

func (rw *rWriter) OpenNewTextBlock(style model.BlockContentTextStyle) {
	if style != model.BlockContentText_Paragraph {
		rw.curStyledBlock = style
	}

	rw.textStylesQueue = append(rw.textStylesQueue, style)
}

func (rw *rWriter) GetBlocks() []*model.Block {
	return rw.blocks
}

func (rw *rWriter) SetIsNumberedList(isNumbered bool) {
	rw.isNumberedList = isNumbered
}

func (rw *rWriter) GetIsNumberedList() (isNumbered bool) {
	return rw.isNumberedList
}

func NewRWriter(writer *bufio.Writer, allFileShortPaths []string) RWriter {
	return &rWriter{Writer: writer, allFileShortPaths: allFileShortPaths}
}

func (rw *rWriter) GetText() string {
	return rw.textBuffer
}

func (rw *rWriter) AddTextToBuffer(text string) {
	rw.textBuffer += text
}

func (rw *rWriter) AddImageBlock(url string) {
	newBlock := model.Block{
		Content: &model.BlockContentOfFile{

			File: &model.BlockContentFile{
				Name:  url,
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

func (rw *rWriter) CloseTextBlock(content model.BlockContentTextStyle) {
	var style = content

	if len(rw.textStylesQueue) > 0 {
		if rw.textStylesQueue[len(rw.textStylesQueue)-1] != content {
			return
		}
		rw.textStylesQueue = rw.textStylesQueue[:len(rw.textStylesQueue)-1]
	}

	if style == rw.curStyledBlock {
		rw.curStyledBlock = model.BlockContentText_Paragraph
	} else if rw.curStyledBlock != model.BlockContentText_Paragraph {
		style = rw.curStyledBlock
	}

	newBlock := model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  rw.textBuffer,
				Style: style,
				Marks: &model.BlockContentTextMarks{
					Marks: rw.marksBuffer,
				},
			},
		},
	}

	// IMPORTANT: do not create a new block if textBuffer is empty
	if len(rw.textBuffer) > 0 {
		rw.blocks = append(rw.blocks, &newBlock)
	}
	rw.marksStartQueue = []int{}
	rw.marksBuffer = []*model.BlockContentTextMark{}
	rw.textBuffer = ""
}

func (rw *rWriter) ForceCloseTextBlock() {
	s := rw.textStylesQueue
	style := model.BlockContentText_Paragraph

	if len(rw.textStylesQueue) > 0 {
		style, rw.textStylesQueue = s[len(s)-1], s[:len(s)-1]
	}

	rw.CloseTextBlock(style)
}
