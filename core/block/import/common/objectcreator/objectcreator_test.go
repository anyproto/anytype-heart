package objectcreator

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/common"
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
		detailsService := mock_detailservice.NewMockService(t)
		mockService := mock_space.NewMockService(t)
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockSpace.EXPECT().IsReadOnly().Return(true)
		mockService.EXPECT().Get(context.Background(), spaceID).Return(mockSpace, nil)

		importedSpaceId := "importedSpaceID"
		identity := "identity"
		participantId := domain.NewParticipantId(spaceID, identity)
		importedSpaceIdParticipantId := domain.NewParticipantId(importedSpaceId, identity)

		oldToNew := map[string]string{importedSpaceIdParticipantId: participantId}
		dataObject := NewDataObject(context.Background(), oldToNew, nil, objectorigin.Import(model.Import_Pb), spaceID)
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

		getter := newDumbObjectGetter(map[string]smartblock.SmartBlock{
			participantId: testParticipant,
		})

		service := New(detailsService, nil, nil, nil, mockService, objectcreator.NewCreator(), getter)

		// when
		create, id, err := service.Create(dataObject, sn)

		// then
		assert.Nil(t, err)
		assert.Nil(t, create)
		assert.Equal(t, participantId, id)
		assert.Equal(t, testDetails, testParticipant.CombinedDetails())
	})
}

func TestObjectCreator_updateKeys(t *testing.T) {
	t.Run("updateKeys - update relation key", func(t *testing.T) {
		// given
		oc := ObjectCreator{}
		oldToNew := map[string]string{"oldId": "newId", "oldKey": "newKey"}
		doc := state.NewDoc("oldId", nil).(*state.State)
		doc.SetDetails(&types.Struct{Fields: map[string]*types.Value{
			"oldKey": pbtypes.String("test"),
		}})
		doc.AddRelationLinks(&model.RelationLink{
			Key: "oldKey",
		})
		// when
		oc.updateKeys(doc, oldToNew)

		// then
		assert.Nil(t, doc.Details().GetFields()["oldKey"])
		assert.Equal(t, pbtypes.String("test"), doc.Details().GetFields()["newKey"])
		assert.True(t, doc.HasRelation("newKey"))
	})
	t.Run("updateKeys - update object type key", func(t *testing.T) {
		// given
		oc := ObjectCreator{}
		oldToNew := map[string]string{"oldId": "newId", "oldKey": "newKey"}
		doc := state.NewDoc("oldId", nil).(*state.State)
		doc.SetObjectTypeKey("oldKey")

		// when
		oc.updateKeys(doc, oldToNew)

		// then
		assert.Equal(t, domain.TypeKey("newKey"), doc.ObjectTypeKey())
	})
	t.Run("nothing to update - update object type key", func(t *testing.T) {
		// given
		oc := ObjectCreator{}
		oldToNew := map[string]string{"oldId": "newId", "oldKey": "newKey"}
		doc := state.NewDoc("oldId", nil).(*state.State)

		// when
		oc.updateKeys(doc, oldToNew)

		// then
		assert.Nil(t, doc.Details().GetFields()["newKey"])
		assert.Equal(t, domain.TypeKey(""), doc.ObjectTypeKey())
	})
	t.Run("keys are the same", func(t *testing.T) {
		// given
		oc := ObjectCreator{}
		oldToNew := map[string]string{"oldId": "newId", "key": "key"}
		doc := state.NewDoc("oldId", nil).(*state.State)
		doc.SetDetails(&types.Struct{Fields: map[string]*types.Value{
			"key": pbtypes.String("test"),
		}})
		doc.AddRelationLinks(&model.RelationLink{
			Key: "key",
		})
		// when
		oc.updateKeys(doc, oldToNew)

		// then
		assert.Equal(t, pbtypes.String("test"), doc.Details().GetFields()["key"])
		assert.True(t, doc.HasRelation("key"))
	})
}

type dumbObjectGetter struct {
	objects map[string]smartblock.SmartBlock
}

func newDumbObjectGetter(objects map[string]smartblock.SmartBlock) *dumbObjectGetter {
	return &dumbObjectGetter{
		objects: objects,
	}
}

func (g *dumbObjectGetter) Init(_ *app.App) error {
	return nil
}

func (g *dumbObjectGetter) Name() string {
	return "dumbObjectGetter"
}

func (g *dumbObjectGetter) GetObject(_ context.Context, id string) (smartblock.SmartBlock, error) {
	if b, ok := g.objects[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("object not found")
}

func (g *dumbObjectGetter) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return g.GetObject(ctx, id.ObjectID)
}

func (g *dumbObjectGetter) DeleteObject(id string) error {
	delete(g.objects, id)
	return nil
}
