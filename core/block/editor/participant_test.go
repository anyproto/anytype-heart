package editor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestParticipant_ModifyProfileDetails(t *testing.T) {
	fx := newParticipantFixture(t)
	defer fx.finish()
	details := pbtypes.ToStruct(map[string]interface{}{
		bundle.RelationKeyName.String():        "name",
		bundle.RelationKeyDescription.String(): "description",
		bundle.RelationKeyIconImage.String():   "icon",
		bundle.RelationKeyId.String():          "profile",
		bundle.RelationKeyGlobalName.String():  "global",
	})
	err := fx.ModifyProfileDetails(details)
	require.NoError(t, err)
	details.Fields[bundle.RelationKeyIdentityProfileLink.String()] = pbtypes.String("profile")
	equalKeys := []string{
		bundle.RelationKeyName.String(),
		bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIconImage.String(),
		bundle.RelationKeyIdentityProfileLink.String(),
		bundle.RelationKeyGlobalName.String(),
	}
	fields := details.GetFields()
	participantFields := fx.CombinedDetails().GetFields()
	for _, key := range equalKeys {
		require.Equal(t, fields[key], participantFields[key])
	}
}

func TestParticipant_ModifyParticipantAclState(t *testing.T) {
	fx := newParticipantFixture(t)
	defer fx.finish()
	err := fx.ModifyParticipantAclState(spaceinfo.ParticipantAclInfo{
		Id:          "id",
		SpaceId:     "spaceId",
		Identity:    "identity",
		Permissions: model.ParticipantPermissions_Owner,
		Status:      model.ParticipantStatus_Active,
	})
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
	equalKeys := []string{
		bundle.RelationKeyId.String(),
		bundle.RelationKeyIdentity.String(),
		bundle.RelationKeySpaceId.String(),
		bundle.RelationKeyLastModifiedBy.String(),
		bundle.RelationKeyParticipantPermissions.String(),
		bundle.RelationKeyParticipantStatus.String(),
		bundle.RelationKeyIsHiddenDiscovery.String(),
	}
	fields := details.GetFields()
	participantFields := fx.CombinedDetails().GetFields()
	for _, key := range equalKeys {
		require.Equal(t, fields[key], participantFields[key])
	}
}

func TestParticipant_ModifyIdentityDetails(t *testing.T) {
	fx := newParticipantFixture(t)
	defer fx.finish()
	identity := &model.IdentityProfile{
		Name:        "name",
		Description: "description",
		IconCid:     "icon",
		GlobalName:  "global",
	}
	err := fx.ModifyIdentityDetails(identity)
	require.NoError(t, err)
	details := pbtypes.ToStruct(map[string]interface{}{
		bundle.RelationKeyName.String():        "name",
		bundle.RelationKeyDescription.String(): "description",
		bundle.RelationKeyIconImage.String():   "icon",
		bundle.RelationKeyGlobalName.String():  "global",
	})
	equalKeys := []string{
		bundle.RelationKeyName.String(),
		bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIconImage.String(),
		bundle.RelationKeyGlobalName.String(),
	}
	fields := details.GetFields()
	participantFields := fx.CombinedDetails().GetFields()
	for _, key := range equalKeys {
		require.Equal(t, fields[key], participantFields[key])
	}
}

func newParticipantTest() (*participant, error) {
	sb := smarttest.New("root")
	p := &participant{
		SmartBlock: sb,
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
	p, err := newParticipantTest()
	require.NoError(t, err)
	return &participantFixture{
		p,
	}
}

func (f *participantFixture) finish() {
}
