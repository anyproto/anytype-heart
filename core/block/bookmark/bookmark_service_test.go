package bookmark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/linkpreview/mock_linkpreview"
)

func TestService_FetchBookmarkContent(t *testing.T) {
	t.Run("link to html page - create blocks", func(t *testing.T) {
		// given
		preview := mock_linkpreview.NewMockLinkPreview(t)
		preview.EXPECT().Fetch(mock.Anything, "http://test.com").Return(model.LinkPreview{}, []byte(testHtml), false, nil)

		s := &service{linkPreview: preview}

		// when
		updaters := s.FetchBookmarkContent("space", "http://test.com", true)

		// then
		content := updaters()
		assert.Len(t, content.Blocks, 2)
	})
	t.Run("link to file - create one block with file", func(t *testing.T) {
		// given
		preview := mock_linkpreview.NewMockLinkPreview(t)
		preview.EXPECT().Fetch(mock.Anything, "http://test.com").Return(model.LinkPreview{}, nil, true, nil)

		s := &service{linkPreview: preview}

		// when
		updaters := s.FetchBookmarkContent("space", "http://test.com", true)

		// then
		content := updaters()
		assert.Len(t, content.Blocks, 1)
		assert.NotNil(t, content.Blocks[0].GetFile())
		assert.Equal(t, "http://test.com", content.Blocks[0].GetFile().GetName())
	})
}

const testHtml = `<html><head>
<title>Title</title>

Test
</head></html>`
