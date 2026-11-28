package v2service

import (
	"errors"
	"testing"

	"github.com/anyproto/anytype-publish-server/publishclient/publishapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

func TestPublishObject(t *testing.T) {
	const id = "page1"
	req := v2model.PublishRequest{Slug: "meeting-notes", IncludeInvite: true}

	t.Run("dry run checks the object without calling the publisher", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.publisher = mock_apicore.NewMockWebPublisher(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, id).Return(testObjectRead(), nil)
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, id).Return(false, nil)

		got, err := fx.PublishObject(t.Context(), testSpaceId, id, req, true)
		require.NoError(t, err)
		assert.Equal(t, &v2model.PublishResult{DryRun: true}, got)
	})

	t.Run("both grant checks precede object access", func(t *testing.T) {
		fx := newV2Fixture(t)
		_, err := fx.PublishObject(grantCtx(util.GrantPermsRead, testSpaceId), testSpaceId, id, req, false)
		requireV2Code(t, err, v2model.CodeWriteNotGranted)
		_, err = fx.PublishObject(grantCtx(util.GrantPermsReadWrite, "elsewhere"), testSpaceId, id, req, true)
		requireV2Code(t, err, v2model.CodeSpaceNotGranted)
	})

	t.Run("only a single unreserved URL segment is accepted", func(t *testing.T) {
		fx := newV2Fixture(t)
		for _, slug := range []string{"", ".", "..", "a/b", "a?b", "a#b", "%2F", "two words"} {
			_, err := fx.PublishObject(t.Context(), testSpaceId, id, v2model.PublishRequest{Slug: slug}, false)
			assert.Equal(t, "/slug", v2Err(t, err).Issues[0].Path, slug)
		}
	})

	t.Run("publishing restrictions also apply on dry runs", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := testObjectRead()
		read.PublishRefused = errors.New("restricted")
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, id).Return(read, nil)
		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, id).Return(false, nil).Once()
		_, err := fx.PublishObject(t.Context(), testSpaceId, id, req, true)
		requireV2Code(t, err, v2model.CodeForbidden)

		fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, id).Return(true, nil)
		_, err = fx.PublishObject(t.Context(), testSpaceId, id, req, false)
		requireV2Code(t, err, v2model.CodeObjectArchived)
	})

	for _, tc := range []struct {
		name       string
		publishErr error
		wantCode   string
	}{
		{"public URL", nil, ""},
		{"size limit", domain.ErrPublishLimitExceeded, v2model.CodePublicationTooLarge},
		{"slug conflict", domain.ErrPublishURLTaken, v2model.CodePublishURLTaken},
		{"unexpected failure", errors.New("offline"), v2model.CodeInternalError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			publisher := mock_apicore.NewMockWebPublisher(t)
			fx.publisher = publisher
			fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, id).Return(testObjectRead(), nil)
			fx.readerMock.EXPECT().InBin(mock.Anything, testSpaceId, id).Return(false, nil)
			const url = "https://example.any.org/meeting-notes"
			publisher.EXPECT().Publish(mock.Anything, testSpaceId, id, req.Slug, true).
				Return(domain.PublishResult{Url: url, InviteIncluded: true}, tc.publishErr).Once()

			got, err := fx.PublishObject(t.Context(), testSpaceId, id, req, false)
			if tc.wantCode != "" {
				requireV2Code(t, err, tc.wantCode)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, url, got.URL)
			require.NotNil(t, got.InviteIncluded)
			assert.True(t, *got.InviteIncluded)
		})
	}
}

func TestListAndRemovePublications(t *testing.T) {
	fx := newV2Fixture(t)
	publisher := mock_apicore.NewMockWebPublisher(t)
	fx.publisher = publisher
	reader := grantCtx(util.GrantPermsRead, testSpaceId)
	publisher.EXPECT().ListPublications(mock.Anything, testSpaceId).Return([]domain.Publication{
		{ObjectId: "b", Slug: "beta"}, {ObjectId: "a", Slug: "alpha"},
	}, nil)
	rows, err := fx.ListPublications(reader, testSpaceId, 0, 1)
	require.NoError(t, err)
	require.Len(t, rows.Data, 1)
	assert.Equal(t, "a", rows.Data[0].ObjectId)
	assert.Equal(t, 2, rows.Total)
	assert.True(t, rows.HasMore)

	_, err = fx.RemovePublication(reader, testSpaceId, "a", false)
	requireV2Code(t, err, v2model.CodeWriteNotGranted)
	_, err = fx.ListPublications(reader, "another-space", 0, 25)
	requireV2Code(t, err, v2model.CodeSpaceNotGranted)

	publisher.EXPECT().GetStatus(mock.Anything, testSpaceId, "a").Return(&pb.RpcPublishingPublishState{SpaceId: testSpaceId, ObjectId: "a"}, nil).Twice()
	preview, err := fx.RemovePublication(t.Context(), testSpaceId, "a", true)
	require.NoError(t, err)
	assert.True(t, preview.DryRun)
	publisher.EXPECT().Unpublish(mock.Anything, testSpaceId, "a").Return(nil).Once()
	result, err := fx.RemovePublication(t.Context(), testSpaceId, "a", false)
	require.NoError(t, err)
	assert.False(t, result.DryRun)
	publisher.EXPECT().GetStatus(mock.Anything, testSpaceId, "a").Return(nil, publishapi.ErrNotFound).Once()
	_, err = fx.RemovePublication(t.Context(), testSpaceId, "a", false)
	requireV2Code(t, err, v2model.CodeNotFound)
}
