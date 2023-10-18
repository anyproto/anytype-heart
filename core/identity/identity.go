package identity

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/slice"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "identity"

var (
	log = logging.Logger("anytype-identity")
)

type Service interface {
	// SubscribeToIdentities subscribes to identities and updates them directly into the objectStore
	SubscribeToIdentities(identities []string) (err error)
	// GetDetails returns the last store details of the identity and provides a way to receive updates via updateHook
	GetDetails(ctx context.Context, identity string) (details *types.Struct, err error)
	// SpaceId returns the spaceId used to store the identities in the objectStore
	SpaceId() string
	app.ComponentRunnable
}

type DetailsModifier interface {
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
}

type spaceIdDeriver interface {
	DeriveID(ctx context.Context, spaceType string) (id string, err error)
}

type service struct {
	spaceService    space.Service
	objectStore     objectstore.ObjectStore
	accountService  account.Service
	spaceIdDeriver  spaceIdDeriver
	detailsModifier DetailsModifier
	closing         chan struct{}
	identities      []string
	techSpaceId     string
	personalSpaceId string
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.accountService = app.MustComponent[account.Service](a)
	s.spaceIdDeriver = app.MustComponent[spaceIdDeriver](a)
	s.detailsModifier = app.MustComponent[DetailsModifier](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.closing = make(chan struct{})
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.techSpaceId, err = s.spaceIdDeriver.DeriveID(ctx, spacecore.TechSpaceType)
	if err != nil {
		return err
	}

	s.personalSpaceId, err = s.spaceIdDeriver.DeriveID(ctx, spacecore.SpaceType)
	if err != nil {
		return err
	}

	err = s.indexIdentityObject(ctx)
	if err != nil {
		return err
	}

	err = s.runLocalProfileSubscriptions(ctx)
	if err != nil {
		return err
	}
	return
}

func (s *service) indexIdentityObject(ctx context.Context) error {
	// Index profile
	techSpace, err := s.spaceService.Get(ctx, s.techSpaceId)
	if err != nil {
		return fmt.Errorf("get tech space: %w", err)
	}
	err = techSpace.Do(s.accountService.IdentityObjectId(), func(_ smartblock.SmartBlock) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("touch profile to index: %w", err)
	}
	return nil
}

func (s *service) Close(ctx context.Context) (err error) {
	close(s.closing)
	return nil
}

func (s *service) SpaceId() string {
	return s.techSpaceId
}

func (s *service) GetDetails(ctx context.Context, profileId string) (details *types.Struct, err error) {
	rec, err := s.objectStore.GetDetails(profileId)
	if err != nil {
		return nil, err
	}

	return rec.Details, nil
}

func getDetailsFromProfile(id, spaceId string, details *types.Struct) *types.Struct {
	name := pbtypes.GetString(details, bundle.RelationKeyName.String())
	description := pbtypes.GetString(details, bundle.RelationKeyDescription.String())
	image := pbtypes.GetString(details, bundle.RelationKeyIconImage.String())
	profileId := pbtypes.GetString(details, bundle.RelationKeyId.String())
	d := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():                pbtypes.String(name),
		bundle.RelationKeyDescription.String():         pbtypes.String(description),
		bundle.RelationKeyId.String():                  pbtypes.String(id),
		bundle.RelationKeyIsReadonly.String():          pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():          pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():            pbtypes.Bool(false),
		bundle.RelationKeySpaceId.String():             pbtypes.String(spaceId),
		bundle.RelationKeyType.String():                pbtypes.String(bundle.TypeKeyProfile.BundledURL()),
		bundle.RelationKeyIdentityProfileLink.String(): pbtypes.String(profileId),
		bundle.RelationKeyLayout.String():              pbtypes.Float64(float64(model.ObjectType_profile)),
		bundle.RelationKeyLastModifiedBy.String():      pbtypes.String(id),
	}}

	if image != "" {
		d.Fields[bundle.RelationKeyIconImage.String()] = pbtypes.String(image)
	}

	// deprecated, but we have exciting profiles which use this, so let's it be up for clients to decide either to render it or not
	iconOption := pbtypes.Get(details, bundle.RelationKeyIconOption.String())
	if iconOption != nil {
		d.Fields[bundle.RelationKeyIconOption.String()] = iconOption
	}

	return d
}

func (s *service) runLocalProfileSubscriptions(ctx context.Context) (err error) {
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeProfilePage, "")
	if err != nil {
		return err
	}
	personalSpace, err := s.spaceService.GetPersonalSpace(ctx)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	profileObjectId, err := personalSpace.DeriveObjectID(ctx, uniqueKey)
	if err != nil {
		return err
	}

	recordsCh := make(chan *types.Struct)
	sub := database.NewSubscription(nil, recordsCh)

	var (
		records  []database.Record
		closeSub func()
	)

	identityObjectId := s.accountService.IdentityObjectId()
	records, closeSub, err = s.objectStore.QueryByIDAndSubscribeForChanges([]string{profileObjectId}, sub)
	if err != nil {
		return err
	}

	go func() {
		select {
		case <-s.closing:
			closeSub()
			return
		}
	}()

	if len(records) > 0 {
		details := getDetailsFromProfile(identityObjectId, s.techSpaceId, records[0].Details)

		err = s.detailsModifier.ModifyDetails(identityObjectId, func(current *types.Struct) (*types.Struct, error) {
			return pbtypes.StructMerge(current, details, false), nil
		})
		if err != nil {
			return fmt.Errorf("initial modify details: %w", err)
		}
	}

	go func() {
		for {
			rec, ok := <-recordsCh
			if !ok {
				return
			}

			details := getDetailsFromProfile(identityObjectId, s.techSpaceId, rec)
			err = s.detailsModifier.ModifyDetails(identityObjectId, func(current *types.Struct) (*types.Struct, error) {
				return pbtypes.StructMerge(current, details, false), nil
			})
			if err != nil {
				log.Errorf("error updating identity object: %v", err)
			}
		}
	}()

	return nil
}

func (s *service) SubscribeToIdentities(identities []string) (err error) {
	for _, identity := range identities {
		if identity != s.accountService.AccountID() {
			return fmt.Errorf("only your personal profileId is supported right now")
		}
		if slice.FindPos(s.identities, identity) == -1 {
			s.identities = append(s.identities, identity)
		}
	}

	// todo: later this method will restart the regular update from the identity registry
	return
}
