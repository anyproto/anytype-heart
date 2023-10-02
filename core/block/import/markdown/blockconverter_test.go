package markdown

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	converter2 "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/source"
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
		files := converter.processFiles(absolutePath, converter2.NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS), source)

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
		files := converter.processFiles(absolutePath, converter2.NewError(pb.RpcObjectImportRequest_IGNORE_ERRORS), source)

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
