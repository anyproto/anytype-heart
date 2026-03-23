package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

func TestService_validateHomepage(t *testing.T) {
	const spaceId = "space1"

	t.Run("empty homepage is valid", func(t *testing.T) {
		// given
		s := &Service{objectStore: objectstore.NewStoreFixture(t)}

		// when
		err := s.validateHomepage(spaceId, "")

		// then
		assert.NoError(t, err)
	})

	t.Run("widgets constant is valid", func(t *testing.T) {
		// given
		s := &Service{objectStore: objectstore.NewStoreFixture(t)}

		// when
		err := s.validateHomepage(spaceId, domain.HomepageWidgets)

		// then
		assert.NoError(t, err)
	})

	t.Run("graph constant is valid", func(t *testing.T) {
		// given
		s := &Service{objectStore: objectstore.NewStoreFixture(t)}

		// when
		err := s.validateHomepage(spaceId, domain.HomepageGraph)

		// then
		assert.NoError(t, err)
	})

	t.Run("lastOpened constant is valid", func(t *testing.T) {
		// given
		s := &Service{objectStore: objectstore.NewStoreFixture(t)}

		// when
		err := s.validateHomepage(spaceId, domain.HomepageLastOpened)

		// then
		assert.NoError(t, err)
	})

	t.Run("existing object id is valid", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("realObject"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		s := &Service{objectStore: store}

		// when
		err := s.validateHomepage(spaceId, "realObject")

		// then
		assert.NoError(t, err)
	})

	t.Run("non-existing object id is invalid", func(t *testing.T) {
		// given
		s := &Service{objectStore: objectstore.NewStoreFixture(t)}

		// when
		err := s.validateHomepage(spaceId, "nonExistentObj")

		// then
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "homepage object nonExistentObj not found")
	})
}

func TestService_SpaceSetHomepage(t *testing.T) {
	const spaceId = "space1"

	t.Run("set homepage to constant", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		detailsSvc.EXPECT().SetSpaceInfo(spaceId, mock.Anything).RunAndReturn(func(spcId string, details *domain.Details) error {
			assert.Equal(t, spaceId, spcId)
			assert.Equal(t, domain.HomepageWidgets, details.GetString(bundle.RelationKeyHomepage))
			return nil
		})

		// when
		err := s.SpaceSetHomepage(spaceId, domain.HomepageWidgets)

		// then
		require.NoError(t, err)
	})

	t.Run("set homepage to empty string", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		detailsSvc.EXPECT().SetSpaceInfo(spaceId, mock.Anything).Return(nil)

		// when
		err := s.SpaceSetHomepage(spaceId, "")

		// then
		require.NoError(t, err)
	})

	t.Run("set homepage to existing object", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("obj1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		detailsSvc := mock_detailservice.NewMockService(t)
		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		detailsSvc.EXPECT().SetSpaceInfo(spaceId, mock.Anything).Return(nil)

		// when
		err := s.SpaceSetHomepage(spaceId, "obj1")

		// then
		require.NoError(t, err)
	})

	t.Run("set homepage to non-existing object returns error", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		detailsSvc := mock_detailservice.NewMockService(t)
		s := &Service{
			objectStore:    store,
			detailsService: detailsSvc,
		}

		// when
		err := s.SpaceSetHomepage(spaceId, "nonExistentObj")

		// then
		require.Error(t, err)
		detailsSvc.AssertNotCalled(t, "SetSpaceInfo", mock.Anything, mock.Anything)
	})
}
