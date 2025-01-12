package bookmark

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator/mock_objectcreator"
	"github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/linkpreview/mock_linkpreview"
)

const (
	spaceId    = "space1"
	bookmarkId = "ot-bookmark"
)

type detailsSetter struct{}

func (ds *detailsSetter) SetDetails(session.Context, string, []domain.Detail) error {
	return nil
}

type fixture struct {
	s *service

	creator      *mock_objectcreator.MockService
	space        *mock_clientspace.MockSpace
	spaceService *mock_space.MockService
	store        *objectstore.StoreFixture
}

func newFixture(t *testing.T) *fixture {
	spc := mock_clientspace.NewMockSpace(t)
	spc.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyBookmark).Return(bookmarkId, nil).Once()
	spaceSvc := mock_space.NewMockService(t)
	spaceSvc.EXPECT().Get(mock.Anything, spaceId).Return(spc, nil).Once()

	store := objectstore.NewStoreFixture(t)
	creator := mock_objectcreator.NewMockService(t)

	s := &service{
		detailsSetter: &detailsSetter{},
		creator:       creator,
		store:         store,
		spaceService:  spaceSvc,
	}

	return &fixture{
		s:            s,
		creator:      creator,
		space:        spc,
		spaceService: spaceSvc,
		store:        store,
	}
}

func TestService_CreateBookmarkObject(t *testing.T) {
	t.Run("new bookmark object creation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		details := domain.NewDetails()
		fx.creator.EXPECT().CreateSmartBlockFromState(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, spcId string, keys []domain.TypeKey, state *state.State) (string, *domain.Details, error) {
				assert.Equal(t, spaceId, spcId)
				assert.Equal(t, []domain.TypeKey{bundle.TypeKeyBookmark}, keys)
				assert.Equal(t, details, state.Details())

				return "some_id", nil, nil
			},
		).Once()

		// when
		_, _, err := fx.s.CreateBookmarkObject(nil, spaceId, details, func() *bookmark.ObjectContent { return nil })

		// then
		assert.NoError(t, err)
	})

	t.Run("bookmark with existing url is created", func(t *testing.T) {
		// given
		fx := newFixture(t)
		url := "https://url.com"
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySource: domain.String(url),
		})
		fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
			bundle.RelationKeyId:     domain.String("bk"),
			bundle.RelationKeySource: domain.String(url),
			bundle.RelationKeyType:   domain.String(bookmarkId),
		}})

		// when
		id, _, err := fx.s.CreateBookmarkObject(nil, spaceId, details, func() *bookmark.ObjectContent {
			return &bookmark.ObjectContent{BookmarkContent: &model.BlockContentBookmark{}}
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, "bk", id)
	})
}

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
	t.Run("link to file - create one block with file, image is base64", func(t *testing.T) {
		// given
		preview := mock_linkpreview.NewMockLinkPreview(t)
		preview.EXPECT().Fetch(mock.Anything, "http://test.com").Return(model.LinkPreview{}, []byte(testHtmlBase64), false, nil)

		s := &service{linkPreview: preview}

		// when
		updaters := s.FetchBookmarkContent("space", "http://test.com", true)

		// then
		content := updaters()
		assert.Len(t, content.Blocks, 1)
		assert.NotNil(t, content.Blocks[0].GetFile())
	})
}

const testHtml = `<html><head>
<title>Title</title>

Test
</head></html>`

const testHtmlBase64 = "<img src=\"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABHNCSVQICAgIfAhkiAAAAAlwSFlzAAAApgAAAKYB3X3/OAAAABl0RVh0U29mdHdhcmUAd3d3Lmlua3NjYXBlLm9yZ5vuPBoAAANCSURBVEiJtZZPbBtFFMZ/M7ubXdtdb1xSFyeilBapySVU8h8OoFaooFSqiihIVIpQBKci6KEg9Q6H9kovIHoCIVQJJCKE1ENFjnAgcaSGC6rEnxBwA04Tx43t2FnvDAfjkNibxgHxnWb2e/u992bee7tCa00YFsffekFY+nUzFtjW0LrvjRXrCDIAaPLlW0nHL0SsZtVoaF98mLrx3pdhOqLtYPHChahZcYYO7KvPFxvRl5XPp1sN3adWiD1ZAqD6XYK1b/dvE5IWryTt2udLFedwc1+9kLp+vbbpoDh+6TklxBeAi9TL0taeWpdmZzQDry0AcO+jQ12RyohqqoYoo8RDwJrU+qXkjWtfi8Xxt58BdQuwQs9qC/afLwCw8tnQbqYAPsgxE1S6F3EAIXux2oQFKm0ihMsOF71dHYx+f3NND68ghCu1YIoePPQN1pGRABkJ6Bus96CutRZMydTl+TvuiRW1m3n0eDl0vRPcEysqdXn+jsQPsrHMquGeXEaY4Yk4wxWcY5V/9scqOMOVUFthatyTy8QyqwZ+kDURKoMWxNKr2EeqVKcTNOajqKoBgOE28U4tdQl5p5bwCw7BWquaZSzAPlwjlithJtp3pTImSqQRrb2Z8PHGigD4RZuNX6JYj6wj7O4TFLbCO/Mn/m8R+h6rYSUb3ekokRY6f/YukArN979jcW+V/S8g0eT/N3VN3kTqWbQ428m9/8k0P/1aIhF36PccEl6EhOcAUCrXKZXXWS3XKd2vc/TRBG9O5ELC17MmWubD2nKhUKZa26Ba2+D3P+4/MNCFwg59oWVeYhkzgN/JDR8deKBoD7Y+ljEjGZ0sosXVTvbc6RHirr2reNy1OXd6pJsQ+gqjk8VWFYmHrwBzW/n+uMPFiRwHB2I7ih8ciHFxIkd/3Omk5tCDV1t+2nNu5sxxpDFNx+huNhVT3/zMDz8usXC3ddaHBj1GHj/As08fwTS7Kt1HBTmyN29vdwAw+/wbwLVOJ3uAD1wi/dUH7Qei66PfyuRj4Ik9is+hglfbkbfR3cnZm7chlUWLdwmprtCohX4HUtlOcQjLYCu+fzGJH2QRKvP3UNz8bWk1qMxjGTOMThZ3kvgLI5AzFfo379UAAAAASUVORK5CYII=\">"
