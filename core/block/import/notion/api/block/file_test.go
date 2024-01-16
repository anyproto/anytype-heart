package block

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
)

func Test_VideoGetBlocks(t *testing.T) {
	t.Run("random url - we create file block", func(t *testing.T) {
		vo := &VideoBlock{
			File: api.FileObject{
				Name: "random url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://example.com/",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetFile())
	})
	t.Run("youtube url www.youtube.com/watch - we create embed block", func(t *testing.T) {
		vo := &VideoBlock{
			File: api.FileObject{
				Name: "youtube url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://www.youtube.com/watch?v=AAAAAAAAAAA",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("youtube url www.youtube.com/live - we create embed block", func(t *testing.T) {
		vo := &VideoBlock{
			File: api.FileObject{
				Name: "youtube url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://www.youtube.com/live/1",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("youtube url youtu.be - we create embed block", func(t *testing.T) {
		vo := &VideoBlock{
			File: api.FileObject{
				Name: "youtube url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://youtu.be/AAAAAAAAAAA",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("youtube url youtu.be with time- we create embed block", func(t *testing.T) {
		vo := &VideoBlock{
			File: api.FileObject{
				Name: "youtube url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://youtu.be/AAAAAAAAAAA?si=AAAAAAAAAAA&t=7212",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("vimeo url - we create embed block", func(t *testing.T) {
		vo := &VideoBlock{
			File: api.FileObject{
				Name: "vimeo url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://vimeo.com/1",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
}

func Test_AudioGetBlocks(t *testing.T) {
	t.Run("random url - we create file block", func(t *testing.T) {
		vo := &AudioBlock{
			File: api.FileObject{
				Name: "random url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://example.com/1",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetFile())
	})
	t.Run("soundcloud url soundcloud.com - we create embed block", func(t *testing.T) {
		vo := &AudioBlock{
			File: api.FileObject{
				Name: "soundcloud url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://soundcloud.com/1/1",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
	t.Run("soundcloud url on.soundcloud - we create embed block", func(t *testing.T) {
		vo := &AudioBlock{
			File: api.FileObject{
				Name: "soundcloud url",
				Type: api.External,
				External: api.FileProperty{
					URL: "https://on.soundcloud.com/1",
				},
			},
		}

		bl := vo.GetBlocks(nil, "")
		assert.NotNil(t, bl)
		assert.Len(t, bl.Blocks, 1)
		assert.NotNil(t, bl.Blocks[0].GetLatex())
	})
}
