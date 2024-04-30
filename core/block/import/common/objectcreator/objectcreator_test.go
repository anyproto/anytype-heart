package objectcreator

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectcreator/mock_blockservice"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestObjectCreator_Create(t *testing.T) {
	t.Run("participant object - don't update it", func(t *testing.T) {
		// given
		spaceID := "spaceId"
		blockService := mock_blockservice.NewMockBlockService(t)
		mockService := mock_space.NewMockService(t)
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockSpace.EXPECT().IsReadOnly().Return(true)
		mockService.EXPECT().Get(context.Background(), spaceID).Return(mockSpace, nil)
		service := New(blockService, nil, nil, nil, nil, mockService, objectcreator.NewCreator())

		importedSpaceId := "importedSpaceID"
		identity := "identity"
		participantId := domain.NewParticipantId(spaceID, identity)
		importedSpaceIdParticipantId := domain.NewParticipantId(importedSpaceId, identity)

		oldToNew := map[string]string{importedSpaceIdParticipantId: participantId}
		dataObject := NewDataObject(context.Background(), oldToNew, nil, nil, objectorigin.Import(model.Import_Pb), spaceID)
		sn := &common.Snapshot{
			Id:     importedSpaceIdParticipantId,
			SbType: coresb.SmartBlockTypeParticipant,
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():                     pbtypes.String(importedSpaceIdParticipantId),
						bundle.RelationKeyIdentity.String():               pbtypes.String(identity),
						bundle.RelationKeySpaceId.String():                pbtypes.String(importedSpaceId),
						bundle.RelationKeyLastModifiedBy.String():         pbtypes.String(identity),
						bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
					},
					},
				},
			},
		}

		testParticipant := smarttest.New(participantId)
		st := testParticipant.NewState()
		testDetails := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():                     pbtypes.String(participantId),
			bundle.RelationKeyIdentity.String():               pbtypes.String(identity),
			bundle.RelationKeySpaceId.String():                pbtypes.String(spaceID),
			bundle.RelationKeyLastModifiedBy.String():         pbtypes.String(identity),
			bundle.RelationKeyParticipantPermissions.String(): pbtypes.Int64(int64(model.ParticipantPermissions_Owner)),
			bundle.RelationKeyParticipantStatus.String():      pbtypes.Int64(int64(model.ParticipantStatus_Active)),
		}}
		st.SetDetails(testDetails)
		err := testParticipant.Apply(st)
		assert.Nil(t, err)

		blockService.EXPECT().GetObject(context.Background(), participantId).Return(testParticipant, nil)

		// when
		create, id, err := service.Create(dataObject, sn)

		// then
		assert.Nil(t, err)
		assert.Nil(t, create)
		assert.Equal(t, participantId, id)
		assert.Equal(t, testDetails, testParticipant.CombinedDetails())
	})
}
