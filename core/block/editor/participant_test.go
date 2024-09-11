package editor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestParticipant_ModifyProfileDetails(t *testing.T) {
	// given
	fx := newParticipantFixture(t)
	defer fx.finish()
	details := pbtypes.ToStruct(map[string]interface{}{
		bundle.RelationKeyName.String():        "name",
		bundle.RelationKeyDescription.String(): "description",
		bundle.RelationKeyIconImage.String():   "icon",
		bundle.RelationKeyId.String():          "profile",
		bundle.RelationKeyGlobalName.String():  "global",
	})

	// when
	err := fx.ModifyProfileDetails(details)

	// then
	require.NoError(t, err)
	details.Fields[bundle.RelationKeyIdentityProfileLink.String()] = pbtypes.String("profile")
	delete(details.Fields, bundle.RelationKeyId.String())
	fields := details.GetFields()
	participantFields := fx.CombinedDetails().GetFields()
	participantRelationLinks := fx.GetRelationLinks()
	for key, _ := range details.Fields {
		require.Equal(t, fields[key], participantFields[key])
		require.True(t, participantRelationLinks.Has(key))
	}
}

func TestParticipant_ModifyParticipantAclState(t *testing.T) {
	// given
	fx := newParticipantFixture(t)
	defer fx.finish()

	// when
	err := fx.ModifyParticipantAclState(spaceinfo.ParticipantAclInfo{
		Id:          "id",
		SpaceId:     "spaceId",
		Identity:    "identity",
		Permissions: model.ParticipantPermissions_Owner,
		Status:      model.ParticipantStatus_Active,
	})

	// then
	require.NoError(t, err)
	details := pbtypes.ToStruct(map[string]interface{}{
		bundle.RelationKeyId.String():                     "id",
		bundle.RelationKeyIdentity.String():               "identity",
		bundle.RelationKeySpaceId.String():                "spaceId",
		bundle.RelationKeyLastModifiedBy.String():         "id",
		bundle.RelationKeyParticipantPermissions.String(): model.ParticipantPermissions_Owner,
		bundle.RelationKeyParticipantStatus.String():      model.ParticipantStatus_Active,
		bundle.RelationKeyIsHiddenDiscovery.String():      false,
	})
	fields := details.GetFields()
	participantFields := fx.CombinedDetails().GetFields()
	participantRelationLinks := fx.GetRelationLinks()
	for key, _ := range details.Fields {
		require.Equal(t, fields[key], participantFields[key])
		require.True(t, participantRelationLinks.Has(key))
	}
}

func TestParticipant_ModifyIdentityDetails(t *testing.T) {
	// given
	fx := newParticipantFixture(t)
	defer fx.finish()
	identity := &model.IdentityProfile{
		Name:        "name",
		Description: "description",
		IconCid:     "icon",
		GlobalName:  "global",
	}

	// when
	err := fx.ModifyIdentityDetails(identity)

	// then
	require.NoError(t, err)
	details := pbtypes.ToStruct(map[string]interface{}{
		bundle.RelationKeyName.String():        "name",
		bundle.RelationKeyDescription.String(): "description",
		bundle.RelationKeyIconImage.String():   "icon",
		bundle.RelationKeyGlobalName.String():  "global",
	})
	fields := details.GetFields()
	participantFields := fx.CombinedDetails().GetFields()
	participantRelationLinks := fx.GetRelationLinks()
	for key, _ := range details.Fields {
		require.Equal(t, fields[key], participantFields[key])
		require.True(t, participantRelationLinks.Has(key))
	}
}

func newStoreFixture(t *testing.T) *objectstore.StoreFixture {
	store := objectstore.NewStoreFixture(t)

	for _, rel := range []domain.RelationKey{
		bundle.RelationKeyFeaturedRelations, bundle.RelationKeyIdentity, bundle.RelationKeyName,
		bundle.RelationKeyIdentityProfileLink, bundle.RelationKeyIsReadonly, bundle.RelationKeyIsArchived,
		bundle.RelationKeyDescription, bundle.RelationKeyIsHidden, bundle.RelationKeyLayout,
		bundle.RelationKeyLayoutAlign, bundle.RelationKeyIconImage, bundle.RelationKeyGlobalName,
		bundle.RelationKeyId, bundle.RelationKeyParticipantPermissions, bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeySpaceId, bundle.RelationKeyParticipantStatus, bundle.RelationKeyIsHiddenDiscovery,
	} {
		store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:     pbtypes.String(""),
			bundle.RelationKeyUniqueKey:   pbtypes.String(rel.URL()),
			bundle.RelationKeyId:          pbtypes.String(rel.String()),
			bundle.RelationKeyRelationKey: pbtypes.String(rel.String()),
		}})
	}

	return store
}

func newParticipantTest(t *testing.T) (*participant, error) {
	sb := smarttest.New("root")
	store := newStoreFixture(t)
	basicComponent := basic.NewBasic(sb, store, nil, nil, nil)
	p := &participant{
		SmartBlock:       sb,
		DetailsUpdatable: basicComponent,
		objectStore:      store,
	}

	initCtx := &smartblock.InitContext{
		IsNewObject: true,
	}
	if err := p.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(p, initCtx)
	if err := p.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return p, nil
}

type participantFixture struct {
	*participant
}

func newParticipantFixture(t *testing.T) *participantFixture {
	p, err := newParticipantTest(t)
	require.NoError(t, err)
	return &participantFixture{
		p,
	}
}

func (f *participantFixture) finish() {
}
