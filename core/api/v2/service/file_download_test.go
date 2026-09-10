package v2service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testRawIconId = "bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"

func expectIconDownload(t *testing.T, svc *mock_apicore.MockFileObjectService) {
	t.Helper()
	img := mock_files.NewMockImage(t)
	file := mock_files.NewMockFile(t)
	svc.EXPECT().GetImageDataFromRawId(mock.Anything, domain.FileId(testRawIconId)).Return(img, nil).Once()
	img.EXPECT().GetOriginalFile().Return(file, nil).Once()
	file.EXPECT().Name().Return("icon.png")
	file.EXPECT().FileId().Return(domain.FileId(testRawIconId))
	file.EXPECT().MimeType().Return("image/png")
	file.EXPECT().Meta().Return(&files.FileMeta{Name: "icon.png", Media: "image/png"})
	file.EXPECT().Reader(mock.Anything).Return(strings.NewReader("icon bytes"), nil).Once()
}

func TestFileDownloadRawIcons(t *testing.T) {
	for _, tc := range []struct {
		name, sourceSpace string
		layout            model.ObjectTypeLayout
		property          domain.RelationKey
		allowed           bool
	}{
		{"participant icon", testSpaceId, model.ObjectType_participant, bundle.RelationKeyIconImage, true},
		{"space view icon", objectstore.TestTechSpaceId, model.ObjectType_spaceView, bundle.RelationKeyIconImage, true},
		{"ordinary object icon", testSpaceId, model.ObjectType_basic, bundle.RelationKeyIconImage, false},
		{"another space participant", "space2", model.ObjectType_participant, bundle.RelationKeyIconImage, false},
		{"another space view", objectstore.TestTechSpaceId, model.ObjectType_spaceView, bundle.RelationKeyIconImage, false},
		{"participant other property", testSpaceId, model.ObjectType_participant, bundle.RelationKeyCoverId, false},
		{"space view other property", objectstore.TestTechSpaceId, model.ObjectType_spaceView, bundle.RelationKeyCoverId, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			fileSvc := mock_apicore.NewMockFileObjectService(t)
			fx.fileService = fileSvc
			row := objectstore.TestObject{
				bundle.RelationKeyId:             domain.String("icon-owner"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(tc.layout)),
				tc.property:                      domain.String(testRawIconId),
			}
			if tc.layout == model.ObjectType_participant {
				row[bundle.RelationKeyId] = domain.String(domain.NewParticipantId(tc.sourceSpace, testAccountId))
				row[bundle.RelationKeyParticipantStatus] = domain.Int64(int64(model.ParticipantStatus_Active))
			}
			if tc.layout == model.ObjectType_spaceView {
				target := testSpaceId
				if tc.name == "another space view" {
					target = "space2"
				}
				row[bundle.RelationKeyId] = domain.String("spaceView_" + target)
				row[bundle.RelationKeyTargetSpaceId] = domain.String(target)
			}
			fx.objectStore.AddObjects(t, tc.sourceSpace, []objectstore.TestObject{row})
			if tc.allowed {
				expectIconDownload(t, fileSvc)
			}
			ctx := grantCtx(util.GrantPermsRead, testSpaceId)
			content, err := fx.GetFileContent(ctx, testSpaceId, testRawIconId, 0)
			if !tc.allowed {
				require.Nil(t, content)
				require.Equal(t, http.StatusNotFound, v2Err(t, err).Status)
				return // An unexpected storage read fails the strict mock.
			}
			require.NoError(t, err)
			body, err := io.ReadAll(content.Reader)
			require.NoError(t, err)
			assert.Equal(t, "icon bytes", string(body))
			assert.Equal(t, "image/png", content.MimeType)
			if tc.layout == model.ObjectType_spaceView {
				space, err := fx.GetSpace(ctx, testSpaceId)
				require.NoError(t, err)
				assert.Equal(t, testRawIconId, space.IconImage)
				spaces, _, _, err := fx.ListSpaces(ctx, 0, 25)
				require.NoError(t, err)
				require.Len(t, spaces, 1)
				assert.Equal(t, testRawIconId, spaces[0].IconImage)
			} else {
				members, _, _, err := fx.ListMembers(ctx, testSpaceId, 0, 25)
				require.NoError(t, err)
				require.Len(t, members, 1)
				assert.Equal(t, testRawIconId, members[0].IconImage)
				me, err := fx.GetMemberMe(ctx, testSpaceId)
				require.NoError(t, err)
				assert.Equal(t, testRawIconId, me.IconImage)
			}

			// Reusing the same service must not retain permission after the
			// participant/space changes its icon, even with cached file keys.
			row[tc.property] = domain.String("")
			fx.objectStore.AddObjects(t, tc.sourceSpace, []objectstore.TestObject{row})
			_, err = fx.GetFileContent(ctx, testSpaceId, testRawIconId, 0)
			require.Equal(t, http.StatusNotFound, v2Err(t, err).Status)
		})
	}
}

func TestFileDownloadAccessChecks(t *testing.T) {
	for _, tc := range []struct {
		name, requestedSpace, boundSpace, id string
		ctx                                  context.Context
		status                               int
	}{
		{"file from another space under granted URL", testSpaceId, "space2", "file1", grantCtx(util.GrantPermsRead, testSpaceId), 404},
		{"missing file binding", testSpaceId, "", "missing", grantCtx(util.GrantPermsRead, testSpaceId), 404},
		{"unreferenced raw cid", testSpaceId, "", testRawIconId, grantCtx(util.GrantPermsRead, testSpaceId), 404},
		{"ungranted space", testSpaceId, testSpaceId, "file1", grantCtx(util.GrantPermsRead, "space2"), 403},
		{"ungranted raw icon", testSpaceId, "", testRawIconId, grantCtx(util.GrantPermsRead, "space2"), 403},
		{"empty grant", testSpaceId, testSpaceId, "file1", grantCtx(util.GrantPermsRead), 403},
		{"nonexistent space", "absent", "absent", "file1", context.Background(), 404},
		{"all spaces excludes tech space", objectstore.TestTechSpaceId, objectstore.TestTechSpaceId, "file1", allSpacesGrantCtx(util.GrantPermsRead), 403},
		{"legacy key still checks file space", testSpaceId, "space2", "file1", context.Background(), 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newV2Fixture(t)
			fx.fileService = mock_apicore.NewMockFileObjectService(t)
			if tc.boundSpace != "" {
				require.NoError(t, fx.objectStore.BindSpaceId(context.Background(), tc.boundSpace, tc.id))
			}
			_, err := fx.GetFileContent(tc.ctx, tc.requestedSpace, tc.id, 0)
			require.Equal(t, tc.status, v2Err(t, err).Status)
		})
	}
}
