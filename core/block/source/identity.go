package source

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/system_object"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// the implementation of this source
func NewIdentity(accountService accountservice.Service, objectStore objectstore.ObjectStore, ss system_object.Service, spaceService spacecore.SpaceCoreService, id string) (s Source) {
	return &identity{
		accountService: accountService,
		spaceService:   spaceService,
		objectStore:    objectStore,
		systemObjects:  ss,
		id:             id,
	}
}

type identity struct {
	accountService  accountservice.Service
	spaceService    spacecore.SpaceCoreService
	objectStore     objectstore.ObjectStore
	systemObjects   system_object.Service
	sub             database.Subscription
	closeSub        func()
	spaceId         string
	profileObjectId string

	id string
}

func (v *identity) ListIds() ([]string, error) {
	// todo: later
	return []string{addr.IdentityPrefix + v.accountService.Account().SignKey.GetPublic().Account()}, nil
}

func (v *identity) ReadOnly() bool {
	return true
}

func (v *identity) Id() string {
	return v.id
}

func (v *identity) SpaceID() string {
	spaceId, err := v.spaceService.DeriveID(context.Background(), spacecore.TechSpaceType)
	if err != nil {
		log.Errorf("failed to derive tech space id: %v", err)
		return ""
	}

	return spaceId
}

func (v *identity) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeIdentity
}

func (v *identity) addRelationLinks(details *types.Struct, st *state.State) error {
	for key := range details.Fields {
		rel, err := bundle.GetRelation(domain.RelationKey(key))
		if err != nil {
			return err
		}
		st.AddRelationLinks(&model.RelationLink{
			Key:    rel.Key,
			Format: rel.Format,
		})
	}
	return nil
}

func (v *identity) getDetailsFromProfile(id, spaceId string, details *types.Struct) *types.Struct {
	name := pbtypes.Get(details, bundle.RelationKeyName.String())
	image := pbtypes.Get(details, bundle.RelationKeyIconImage.String())
	iconOption := pbtypes.Get(details, bundle.RelationKeyIconOption.String())
	profileId := pbtypes.Get(details, bundle.RelationKeyId.String())
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():                name,
		bundle.RelationKeyIconImage.String():           image,
		bundle.RelationKeyIconOption.String():          iconOption,
		bundle.RelationKeyId.String():                  pbtypes.String(id),
		bundle.RelationKeyIsReadonly.String():          pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():          pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():            pbtypes.Bool(false),
		bundle.RelationKeySpaceId.String():             pbtypes.String(spaceId),
		bundle.RelationKeyType.String():                pbtypes.String(bundle.TypeKeyObjectType.BundledURL()), // todo: we dont
		bundle.RelationKeyIdentityProfileLink.String(): profileId,
		bundle.RelationKeyLayout.String():              pbtypes.Float64(float64(model.ObjectType_profile)),
	}}
}

func (v *identity) init(ctx context.Context) error {
	if v.profileObjectId != "" {
		// no need to derive again
		return nil
	}

	personalSpaceId, err := v.spaceService.DeriveID(ctx, spacecore.SpaceType)
	if err != nil {
		return err
	}

	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeProfilePage, "")
	if err != nil {
		return err
	}

	v.profileObjectId, err = v.systemObjects.GetObjectIdByUniqueKey(ctx, personalSpaceId, uniqueKey)
	if err != nil {
		return err
	}

	return nil
}

func (v *identity) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	if v.closeSub != nil {
		v.closeSub()
		v.closeSub = nil
	}

	if v.id != addr.IdentityPrefix+v.accountService.Account().SignKey.GetPublic().Account() {
		return nil, fmt.Errorf("only your personal profileId is supported right now")
	}

	if err = v.init(ctx); err != nil {
		return nil, err
	}

	techSpaceId, err := v.spaceService.DeriveID(ctx, spacecore.TechSpaceType)
	if err != nil {
		return nil, err
	}

	recordsCh := make(chan *types.Struct, 1)
	v.sub = database.NewSubscription(nil, recordsCh)
	var records []database.Record
	records, v.closeSub, err = v.objectStore.QueryByIDAndSubscribeForChanges([]string{v.profileObjectId}, v.sub)
	if err != nil {
		return nil, err
	}

	s := state.NewDoc(v.id, nil).(*state.State)
	s.SetObjectTypeKey(bundle.TypeKeyProfile)

	profileLinkRel := simple.New(&model.Block{
		Id: "profilelink",
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: bundle.RelationKeyIdentityProfileLink.String(),
			},
		},
	})

	s.Add(profileLinkRel)
	if len(records) > 0 {
		details := v.getDetailsFromProfile(v.id, techSpaceId, records[0].Details)
		s.SetDetails(details)
		err = v.addRelationLinks(details, s)
		if err != nil {
			return nil, err
		}
	}

	go func() {
		for {
			rec, ok := <-recordsCh
			if !ok {
				return
			}
			s2 := s.Copy()
			details := v.getDetailsFromProfile(v.id, techSpaceId, rec)
			s2.SetDetails(details)
			err := v.addRelationLinks(details, s2)
			if err != nil {
				log.Errorf("failed to add relation links: %v", err)
			}
			receiver.StateRebuild(s2)
		}
	}()

	return s, nil
}

func (v *identity) ReadMeta(ctx context.Context, r ChangeReceiver) (doc state.Doc, err error) {
	return v.ReadDoc(ctx, r, false)
}

func (v *identity) Close() (err error) {
	return
}

func (v *identity) Heads() []string {
	err := v.init(context.Background())
	if err != nil {
		log.Errorf("failed to init: %v", err)
		return []string{""}
	}
	headHash, err := v.objectStore.GetLastIndexedHeadsHash(v.profileObjectId)
	if err != nil {
		return []string{""}
	}
	return []string{headHash}
}

func (s *identity) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *identity) PushChange(params PushChangeParams) (id string, err error) {
	return
}

func (s *identity) GetCreationInfo() (creator string, createdDate int64, err error) {
	return s.id, 0, nil
}
