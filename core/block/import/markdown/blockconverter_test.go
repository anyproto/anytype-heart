package markdown

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type MockTempDir struct{}

func (m MockTempDir) TempDir() string {
	return os.TempDir()
}

func Test_processFiles(t *testing.T) {
	t.Run("imported directory include mov and pdf files - md file has file blocks", func(t *testing.T) {
		// given
		converter := newMDConverter(&MockTempDir{})
		_, err := os.Create("./testdata/test.pdf")
		assert.Nil(t, err)
		defer os.Remove("./testdata/test.pdf")
		_, err = os.Create("./testdata/test.mov")
		assert.Nil(t, err)
		defer os.Remove("./testdata/test.mov")

		workingDir, err := os.Getwd()
		absolutePath := filepath.Join(workingDir, "./testdata")
		source := source.GetSource(absolutePath)

		// when
		files := converter.processFiles(absolutePath, common.NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS), source)

		// then
		assert.Len(t, files, 3)

		pdfFilePath := filepath.Join(absolutePath, "test.pdf")
		assert.Contains(t, files, pdfFilePath)

		movFilePath := filepath.Join(absolutePath, "test.mov")
		assert.Contains(t, files, movFilePath)

		mdFilePath := filepath.Join(absolutePath, "test.md")
		assert.Contains(t, files, mdFilePath)

		fileBlocks := lo.Filter(files[mdFilePath].ParsedBlocks, func(item *model.Block, index int) bool {
			return item.GetFile() != nil
		})

		assert.Len(t, fileBlocks, 2)
		assert.Equal(t, pdfFilePath, fileBlocks[0].GetFile().Name)
		assert.Equal(t, movFilePath, fileBlocks[1].GetFile().Name)
	})

	t.Run("imported directory include without mov and pdf files - no file blocks", func(t *testing.T) {
		// given
		converter := newMDConverter(&MockTempDir{})
		source := source.GetSource("./testdata")
		workingDir, err := os.Getwd()
		assert.Nil(t, err)
		absolutePath := filepath.Join(workingDir, "./testdata")

		// when
		files := converter.processFiles(absolutePath, common.NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS), source)

		// then
		assert.Len(t, files, 1)

		pdfFilePath := filepath.Join(absolutePath, "test.pdf")
		assert.NotContains(t, files, pdfFilePath)

		movFilePath := filepath.Join(absolutePath, "test.mov")
		assert.NotContains(t, files, movFilePath)

		mdFilePath := filepath.Join(absolutePath, "test.md")
		assert.Contains(t, files, mdFilePath)

		fileBlocks := lo.Filter(files[mdFilePath].ParsedBlocks, func(item *model.Block, index int) bool {
			return item.GetFile() != nil
		})

		assert.Len(t, fileBlocks, 0)
	})
}

func Test_processTextBlock(t *testing.T) {
	t.Run("block with single markdown - file doesn't exist", func(t *testing.T) {
		// given
		mc := mdConverter{}
		block := getTestTxtBlock("file.md")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{})

		// then
		assert.NotNil(t, block.GetBookmark())
	})
	t.Run("block with single markdown - file exists", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlock("file.md")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{"file.md": {PageID: "id"}})

		// then
		assert.NotNil(t, block.GetLink())
		assert.Equal(t, "file.md", block.GetLink().GetTargetBlockId())
	})
	t.Run("block with single markdown - file exists, but not md or csv", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlock("file.txt")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{"file.txt": {}})

		// then
		assert.NotNil(t, block.GetFile())
		assert.Equal(t, "file.txt", block.GetFile().GetName())
	})
	t.Run("block with single markdown - csv file exists", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlock("file.csv")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{"file.csv": {}})

		// then
		assert.NotNil(t, block.GetLink())
		assert.Equal(t, "file.csv", block.GetLink().GetTargetBlockId())
	})
	t.Run("block with multiple markdown - file doesn't exist", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlockWithMultipleMarks("file.md")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{})

		// then
		assert.NotNil(t, block.GetBookmark())
	})
	t.Run("block with multiple markdown - file exists", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlockWithMultipleMarks("file.md")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{"file.md": {PageID: "id"}})

		// then
		assert.NotNil(t, block.GetText())
		assert.Len(t, block.GetText().GetMarks().GetMarks(), 3)
		assert.Equal(t, "file.md", block.GetText().GetMarks().GetMarks()[1].Param)
		assert.Equal(t, model.BlockContentTextMark_Object, block.GetText().GetMarks().GetMarks()[1].Type)
	})
	t.Run("block with multiple markdown - file exists, but not md or csv", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlockWithMultipleMarks("file.txt")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{"file.txt": {}})

		// then
		assert.NotNil(t, block.GetFile())
	})
	t.Run("block with multiple markdown - csv file exists", func(t *testing.T) {
		mc := mdConverter{}
		block := getTestTxtBlockWithMultipleMarks("file.csv")

		// when
		mc.processTextBlock(block, map[string]*FileInfo{"file.csv": {}})

		// then
		// then
		assert.NotNil(t, block.GetText())
		assert.Len(t, block.GetText().GetMarks().GetMarks(), 3)
		assert.Equal(t, "file.csv", block.GetText().GetMarks().GetMarks()[1].Param)
		assert.Equal(t, model.BlockContentTextMark_Object, block.GetText().GetMarks().GetMarks()[1].Type)
	})
}

func getTestTxtBlock(filename string) *model.Block {
	return &model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "test",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   4,
							},
							Type:  model.BlockContentTextMark_Link,
							Param: filename,
						},
					},
				},
			},
		},
	}
}

func getTestTxtBlockWithMultipleMarks(filename string) *model.Block {
	return &model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "test",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Range: &model.Range{
								From: 0,
								To:   1,
							},
							Type: model.BlockContentTextMark_Bold,
						},
						{
							Range: &model.Range{
								From: 0,
								To:   4,
							},
							Type:  model.BlockContentTextMark_Link,
							Param: filename,
						},
						{
							Range: &model.Range{
								From: 0,
								To:   2,
							},
							Type: model.BlockContentTextMark_Italic,
						},
					},
				},
			},
		},
	}
}
