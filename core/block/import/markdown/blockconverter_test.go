package markdown

import (
	"os"
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
		converter := newMDConverter(&MockTempDir{})
		_, err := os.Create("./testdata/test.pdf")
		assert.Nil(t, err)
		defer os.Remove("./testdata/test.pdf")
		_, err = os.Create("./testdata/test.mov")
		assert.Nil(t, err)
		defer os.Remove("./testdata/test.mov")

		source := source.GetSource("./testdata")
		files := converter.processFiles("./testdata", pb.RpcObjectImportRequest_IGNORE_ERRORS.String(), converter2.NewError(), source)

		assert.Len(t, files, 3)
		assert.Contains(t, files, "test.pdf")
		assert.Contains(t, files, "test.mov")
		assert.Contains(t, files, "test.md")

		defer func() {
			for _, file := range files {
				if file.ReadCloser != nil {
					file.Close()
				}
			}
		}()

		fileBlocks := lo.Filter(files["test.md"].ParsedBlocks, func(item *model.Block, index int) bool {
			return item.GetFile() != nil
		})

		assert.Len(t, fileBlocks, 2)
		assert.Equal(t, fileBlocks[0].GetFile().Name, "testdata/test.pdf")
		assert.Equal(t, fileBlocks[1].GetFile().Name, "testdata/test.mov")
	})

	t.Run("imported directory include without mov and pdf files - no file blocks", func(t *testing.T) {
		converter := newMDConverter(&MockTempDir{})

		source := source.GetSource("./testdata")
		files := converter.processFiles("./testdata", pb.RpcObjectImportRequest_IGNORE_ERRORS.String(), converter2.NewError(), source)

		assert.Len(t, files, 1)
		assert.NotContains(t, files, "test.pdf")
		assert.NotContains(t, files, "test.mov")
		assert.Contains(t, files, "test.md")

		defer func() {
			for _, file := range files {
				if file.ReadCloser != nil {
					file.Close()
				}
			}
		}()

		fileBlocks := lo.Filter(files["test.md"].ParsedBlocks, func(item *model.Block, index int) bool {
			return item.GetFile() != nil
		})

		assert.Len(t, fileBlocks, 0)
	})
}
