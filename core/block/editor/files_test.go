package editor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

type fileFixture struct {
	*File
	fileObjectService *mock_fileobject.MockService
}

func newFileFixture(t *testing.T, objectId string) *fileFixture {
	fos := mock_fileobject.NewMockService(t)
	return &fileFixture{
		File: &File{
			SmartBlock:        smarttest.New(objectId),
			fileObjectService: fos,
		},
		fileObjectService: fos,
	}
}

func TestFile_markUploadedHook(t *testing.T) {
	makeApplyInfo := func(st *state.State, detailKey string) smartblock.ApplyInfo {
		return smartblock.ApplyInfo{
			State: st,
			Changes: []*pb.ChangeContent{
				{Value: &pb.ChangeContentValueOfDetailsSet{DetailsSet: &pb.ChangeDetailsSet{
					Key: detailKey,
				}}},
			},
		}
	}

	t.Run("calls MarkFileUploaded when status changes to Synced", func(t *testing.T) {
		fx := newFileFixture(t, "obj1")
		called := make(chan struct{})
		fx.fileObjectService.EXPECT().MarkFileUploaded("obj1").Run(func(string) { close(called) }).Return(nil).Once()

		st := state.NewDoc("obj1", nil).NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Synced)))

		err := fx.markUploadedHook(makeApplyInfo(st, bundle.RelationKeyFileBackupStatus.String()))
		require.NoError(t, err)

		select {
		case <-called:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for MarkFileUploaded to be called")
		}
	})

	t.Run("skips when status changes to non-Synced", func(t *testing.T) {
		fx := newFileFixture(t, "obj1")

		st := state.NewDoc("obj1", nil).NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Syncing)))

		err := fx.markUploadedHook(makeApplyInfo(st, bundle.RelationKeyFileBackupStatus.String()))
		require.NoError(t, err)
	})

	t.Run("skips when no FileBackupStatus change", func(t *testing.T) {
		fx := newFileFixture(t, "obj1")

		st := state.NewDoc("obj1", nil).NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Synced)))

		err := fx.markUploadedHook(makeApplyInfo(st, bundle.RelationKeyName.String()))
		require.NoError(t, err)
	})

	t.Run("skips when no changes", func(t *testing.T) {
		fx := newFileFixture(t, "obj1")

		err := fx.markUploadedHook(smartblock.ApplyInfo{
			State:   state.NewDoc("obj1", nil).NewState(),
			Changes: nil,
		})
		require.NoError(t, err)
	})
}
