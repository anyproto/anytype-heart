package editor

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func TestParticipant_ModifyProfileDetails(t *testing.T) {
	// given
	fx := newParticipantFixture(t)
	defer fx.finish()
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:        domain.String("name"),
		bundle.RelationKeyDescription: domain.String("description"),
		bundle.RelationKeyIconImage:   domain.String("icon"),
		bundle.RelationKeyId:          domain.String("profile"),
		bundle.RelationKeyGlobalName:  domain.String("global"),
	})

	// when
	err := fx.ModifyProfileDetails(details)

	// then
	require.NoError(t, err)
	details.Set(bundle.RelationKeyIdentityProfileLink, domain.String("profile"))
	details.Delete(bundle.RelationKeyId)
	participantDetails := fx.CombinedDetails()
	for key, _ := range details.Iterate() {
		require.True(t, details.Get(key).Equal(participantDetails.Get(key)))
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
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:                     domain.String("id"),
		bundle.RelationKeyIdentity:               domain.String("identity"),
		bundle.RelationKeySpaceId:                domain.String("spaceId"),
		bundle.RelationKeyLastModifiedBy:         domain.String("id"),
		bundle.RelationKeyParticipantPermissions: domain.Int64(model.ParticipantPermissions_Owner),
		bundle.RelationKeyParticipantStatus:      domain.Int64(model.ParticipantStatus_Active),
		bundle.RelationKeyIsHiddenDiscovery:      domain.Bool(false),
	})
	participantDetails := fx.CombinedDetails()
	for key, _ := range details.Iterate() {
		require.True(t, details.Get(key).Equal(participantDetails.Get(key)))
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
	details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:        domain.String("name"),
		bundle.RelationKeyDescription: domain.String("description"),
		bundle.RelationKeyIconImage:   domain.String("icon"),
		bundle.RelationKeyGlobalName:  domain.String("global"),
	})
	participantDetails := fx.CombinedDetails()
	for key, _ := range details.Iterate() {
		require.True(t, details.Get(key).Equal(participantDetails.Get(key)))
	}
}

func TestParticipant_Init(t *testing.T) {
	t.Run("title block not empty, because name detail is in store", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		store := newStoreFixture(t)
		store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId: domain.String("spaceId"),
			bundle.RelationKeyId:      domain.String("root"),
			bundle.RelationKeyName:    domain.String("test"),
		}})

		basicComponent := basic.NewBasic(sb, store, nil, nil)
		p := &participant{
			SmartBlock:       sb,
			DetailsUpdatable: basicComponent,
			objectStore:      store,
		}

		initCtx := &smartblock.InitContext{
			IsNewObject: true,
		}

		// when
		err := p.Init(initCtx)
		assert.NoError(t, err)
		migration.RunMigrations(p, initCtx)
		err = p.Apply(initCtx.State)
		assert.NoError(t, err)

		// then
		assert.NotNil(t, p.NewState().Get(state.TitleBlockID))
		assert.Equal(t, "test", p.NewState().Get(state.TitleBlockID).Model().GetText().GetText())
	})
	t.Run("title block is empty", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		store := newStoreFixture(t)

		basicComponent := basic.NewBasic(sb, store, nil, nil)
		p := &participant{
			SmartBlock:       sb,
			DetailsUpdatable: basicComponent,
			objectStore:      store,
		}

		initCtx := &smartblock.InitContext{
			IsNewObject: true,
		}

		// when
		err := p.Init(initCtx)
		assert.NoError(t, err)
		migration.RunMigrations(p, initCtx)
		err = p.Apply(initCtx.State)
		assert.NoError(t, err)

		// then
		assert.NotNil(t, p.NewState().Get(state.TitleBlockID))
		assert.Equal(t, "", p.NewState().Get(state.TitleBlockID).Model().GetText().GetText())
	})
}

func newStoreFixture(t *testing.T) *spaceindex.StoreFixture {
	store := spaceindex.NewStoreFixture(t)

	for _, rel := range []domain.RelationKey{
		bundle.RelationKeyFeaturedRelations, bundle.RelationKeyIdentity, bundle.RelationKeyName,
		bundle.RelationKeyIdentityProfileLink, bundle.RelationKeyIsReadonly, bundle.RelationKeyIsArchived,
		bundle.RelationKeyDescription, bundle.RelationKeyIsHidden, bundle.RelationKeyResolvedLayout,
		bundle.RelationKeyLayoutAlign, bundle.RelationKeyIconImage, bundle.RelationKeyGlobalName,
		bundle.RelationKeyId, bundle.RelationKeyParticipantPermissions, bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeySpaceId, bundle.RelationKeyParticipantStatus, bundle.RelationKeyIsHiddenDiscovery,
	} {
		store.AddObjects(t, []objectstore.TestObject{{
			bundle.RelationKeySpaceId:     domain.String("space1"),
			bundle.RelationKeyUniqueKey:   domain.String(rel.URL()),
			bundle.RelationKeyId:          domain.String(rel.String()),
			bundle.RelationKeyRelationKey: domain.String(rel.String()),
		}})
	}

	return store
}

type accountServiceStub struct{}

func (a accountServiceStub) AccountID() string {
	return ""
}

func (a accountServiceStub) PersonalSpaceID() string {
	return ""
}

func (a accountServiceStub) MyParticipantId(spaceId string) string {
	return "myId"
}

func (a accountServiceStub) Keys() *accountdata.AccountKeys {
	return nil
}

func (a accountServiceStub) GetAccountObjectId() (string, error) {
	return "accObjId", nil
}

func newParticipantTest(t *testing.T) (*participant, error) {
	sb := smarttest.New("root")
	store := newStoreFixture(t)
	basicComponent := basic.NewBasic(sb, store, nil, nil)
	p := &participant{
		SmartBlock:       sb,
		DetailsUpdatable: basicComponent,
		objectStore:      store,
		accountService:   accountServiceStub{},
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
