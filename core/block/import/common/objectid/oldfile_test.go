package objectid

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/require"
)

func TestLegacyFileUsesAnyBlockRemoteMetadata(t *testing.T) {
	store := objectstore.NewStoreFixture(t)
	service := mock_fileobject.NewMockService(t)
	origin := objectorigin.Import(model.Import_Pb)
	service.EXPECT().CreateFromImport(domain.FullFileId{SpaceId: "destination", FileId: "blob-cid"}, origin, (*domain.Details)(nil)).Return("new-file", nil)
	sn := &common.Snapshot{Snapshot: &common.SnapshotModel{Data: &common.StateSnapshot{
		Details:  domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{bundle.RelationKeyId: domain.String("source-document")}),
		FileInfo: &model.FileInfo{FileId: "blob-cid", EncryptionKeys: []*model.FileEncryptionKey{{Path: "/0/", Key: "encryption-key"}}},
	}}}
	importer := &oldFile{objectStore: store, fileObjectService: service}
	id, _, err := importer.GetIDAndPayload(context.Background(), "destination", sn, time.Now(), false, origin)
	require.NoError(t, err)
	require.Equal(t, "new-file", id)
}
