package objectcreator

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
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
		dataObject := NewDataObject(context.Background(), oldToNew, nil, nil, objectorigin.Import(model.Import_Pb), spaceID)
		sn := &common.Snapshot{
			Id: importedSpaceIdParticipantId,
			Snapshot: &common.SnapshotModel{
				SbType: coresb.SmartBlockTypeParticipant,
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyId:                     domain.String(importedSpaceIdParticipantId),
						bundle.RelationKeyIdentity:               domain.String(identity),
						bundle.RelationKeySpaceId:                domain.String(importedSpaceId),
						bundle.RelationKeyLastModifiedBy:         domain.String(identity),
						bundle.RelationKeyParticipantPermissions: domain.Int64(int64(model.ParticipantPermissions_Reader)),
						bundle.RelationKeyParticipantStatus:      domain.Int64(int64(model.ParticipantStatus_Active)),
					}),
				},
			},
		}

		testParticipant := smarttest.New(participantId)
		st := testParticipant.NewState()
		testDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:                     domain.String(participantId),
			bundle.RelationKeyIdentity:               domain.String(identity),
			bundle.RelationKeySpaceId:                domain.String(spaceID),
			bundle.RelationKeyLastModifiedBy:         domain.String(identity),
			bundle.RelationKeyParticipantPermissions: domain.Int64(int64(model.ParticipantPermissions_Owner)),
			bundle.RelationKeyParticipantStatus:      domain.Int64(int64(model.ParticipantStatus_Active)),
		})
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
		doc.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"oldKey": domain.String("test"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key: "oldKey",
		})
		// when
		oc.updateKeys(doc, oldToNew)

		// then
		assert.False(t, doc.Details().Has("oldKey"))
		assert.Equal(t, domain.String("test"), doc.Details().Get("newKey"))
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
		assert.False(t, doc.Details().Has("newKey"))
		assert.Equal(t, domain.TypeKey(""), doc.ObjectTypeKey())
	})
	t.Run("keys are the same", func(t *testing.T) {
		// given
		oc := ObjectCreator{}
		oldToNew := map[string]string{"oldId": "newId", "key": "key"}
		doc := state.NewDoc("oldId", nil).(*state.State)
		doc.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"key": domain.String("test"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key: "key",
		})
		// when
		oc.updateKeys(doc, oldToNew)

		// then
		assert.Equal(t, "test", doc.Details().GetString("key"))
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
