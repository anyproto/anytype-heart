package bookmark

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bookmarksvc "github.com/anyproto/anytype-heart/core/block/bookmark"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	simplebookmark "github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type fakeBookmarkService struct {
	gotDetails *domain.Details
}

func (f *fakeBookmarkService) FetchAsync(spaceID string, blockID string, params simplebookmark.FetchParams) {
}

func (f *fakeBookmarkService) CreateBookmarkObject(
	ctx context.Context, spaceId, templateId string, details *domain.Details, getContent bookmarksvc.ContentFuture,
) (string, *domain.Details, error) {
	f.gotDetails = details
	return "bookmarkObjectId", details, nil
}

func newBookmarkBlock(id, url string) simplebookmark.Block {
	return simplebookmark.NewBookmark(&model.Block{
		Id:      id,
		Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{Url: url}},
	}).(simplebookmark.Block)
}

func TestSbookmark_updateBlock(t *testing.T) {
	t.Run("records createdInContext (object) and ref (block) of the containing object", func(t *testing.T) {
		// given
		fake := &fakeBookmarkService{}
		b := &sbookmark{SmartBlock: smarttest.New("pageObjectId"), bookmarkSvc: fake}
		block := newBookmarkBlock("bookmarkBlockId", "http://example.com")

		// when
		err := b.updateBlock(block, "", func(simplebookmark.Block) error { return nil }, objectorigin.None())

		// then
		require.NoError(t, err)
		require.NotNil(t, fake.gotDetails)
		assert.Equal(t, "pageObjectId", fake.gotDetails.GetString(bundle.RelationKeyCreatedInContext))
		assert.Equal(t, "bookmarkBlockId", fake.gotDetails.GetString(bundle.RelationKeyCreatedInContextRef))
	})
}
