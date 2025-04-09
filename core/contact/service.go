package contact

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

var log = logging.Logger(CName)

const CName = "core.contact.service"

type Service interface {
	app.ComponentRunnable
	SaveContact(ctx context.Context, identity string, profileSymKey string) error
	DeleteContact(ctx context.Context, identity string) error
}

type identityService interface {
	app.Component
	RegisterIdentity(spaceId string, identity string, encryptionKey crypto.SymKey, observer func(identity string, profile *model.IdentityProfile)) error
	UnregisterIdentity(spaceId string, identity string)
	GetIdentityKey(identity string) crypto.SymKey
}

type service struct {
	identityService identityService
	ctx             context.Context
	cancel          context.CancelFunc
	spaceService    space.Service
	objectGetter    cache.ObjectGetter
	store           objectstore.ObjectStore
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	s.identityService = app.MustComponent[identityService](a)
	s.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var (
		contacts []*userdataobject.Contact
		listErr  error
	)
	err = s.spaceService.TechSpace().DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		contacts, listErr = userDataObject.ListContacts(ctx)
		if listErr != nil {
			return listErr
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, contact := range contacts {
		err = s.registerContact(ctx, contact)
		if err != nil {
			log.Error(err)
			continue
		}
	}
	return nil
}

func (s *service) registerContact(ctx context.Context, contact *userdataobject.Contact) error {
	key, err := decodeAESKeyFromBase64(contact.Key())
	if err != nil {
		return fmt.Errorf("failed to register contact identity %w", err)
	}
	err = s.identityService.RegisterIdentity(s.spaceService.TechSpaceId(), contact.Identity(), key, s.handleIdentityUpdate)
	if err != nil {
		return fmt.Errorf("failed to register contact identity %w", err)
	}
	s.applyContactStoreData(contact)
	return nil
}

func decodeAESKeyFromBase64(encodedProfileSymKey string) (*crypto.AESKey, error) {
	profileSymKey, err := base64.StdEncoding.DecodeString(encodedProfileSymKey)
	if err != nil {
		return nil, err
	}
	key, err := crypto.UnmarshallAESKey(profileSymKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *service) handleIdentityUpdate(identity string, profile *model.IdentityProfile) {
	err := s.updateContactDetails(s.ctx, domain.FullID{
		ObjectID: domain.NewContactId(identity),
		SpaceID:  s.spaceService.TechSpaceId(),
	}, func(state *state.State) {
		s.applyIdentityProfileData(state, profile)
	})
	if err != nil {
		log.Errorf("failed to update contact for identity %s: %v", identity, err)
	}
}

func (s *service) updateContactDetails(ctx context.Context, id domain.FullID, updater func(*state.State)) error {
	return cache.DoContextFullID(s.objectGetter, ctx, id, func(contactObject smartblock.SmartBlock) error {
		state := contactObject.NewState()
		updater(state)
		return contactObject.Apply(state)
	})
}

func (s *service) applyIdentityProfileData(state *state.State, identityProfile *model.IdentityProfile) {
	state.SetDetail(bundle.RelationKeyIdentity, domain.String(identityProfile.Identity))
	details := state.Details()
	name := details.GetString(bundle.RelationKeyName)
	if name != identityProfile.Name {
		state.SetDetail(bundle.RelationKeyName, domain.String(identityProfile.Name))
	}
	globalName := details.GetString(bundle.RelationKeyGlobalName)
	if globalName != identityProfile.GlobalName {
		state.SetDetail(bundle.RelationKeyGlobalName, domain.String(identityProfile.GlobalName))
	}
	icon := details.GetString(bundle.RelationKeyIconImage)
	if icon != identityProfile.IconCid {
		state.SetDetail(bundle.RelationKeyIconImage, domain.String(identityProfile.IconCid))
	}
}

func (s *service) applyContactStoreData(contact *userdataobject.Contact) {
	err := s.updateContactDetails(s.ctx, domain.FullID{
		ObjectID: domain.NewContactId(contact.Identity()),
		SpaceID:  s.spaceService.TechSpaceId(),
	}, func(state *state.State) {
		state.SetDetail(bundle.RelationKeyDescription, domain.String(contact.Description()))
	})
	if err != nil {
		log.Errorf("update contact: %v", err)
	}
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.cancel != nil {
		s.cancel()
	}
	return
}

func (s *service) SaveContact(ctx context.Context, identity string, profileSymKey string) error {
	if identity == "" {
		return fmt.Errorf("identity is empty")
	}
	symKey, err := s.fetchIdentityKey(identity, profileSymKey)
	if err != nil {
		return err
	}
	contact, err := s.buildContact(identity, symKey)
	if err != nil {
		return err
	}
	err = s.saveContactData(ctx, contact)
	if err != nil {
		return err
	}
	return s.identityService.RegisterIdentity(s.spaceService.TechSpaceId(), identity, symKey, s.handleIdentityUpdate)
}

func (s *service) fetchIdentityKey(identity string, profileSymKey string) (crypto.SymKey, error) {
	var (
		symKey crypto.SymKey
		err    error
	)
	if len(profileSymKey) == 0 {
		symKey = s.identityService.GetIdentityKey(identity)
	} else {
		symKey, err = unmarshalSymmetricKey(profileSymKey)
		if err != nil {
			return nil, fmt.Errorf("get aes key for identity %s: %w", identity, err)
		}
	}
	if symKey == nil {
		return nil, fmt.Errorf("no symkey for identity %s", identity)
	}
	return symKey, nil
}

func unmarshalSymmetricKey(profileSymKey string) (*crypto.AESKey, error) {
	key, err := crypto.UnmarshallAESKeyString(profileSymKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *service) buildContact(identity string, symKey crypto.SymKey) (*userdataobject.Contact, error) {
	rawKey, err := symKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("get raw sym key: %w", err)
	}
	encodedKey := base64.StdEncoding.EncodeToString(rawKey)
	contact := userdataobject.NewContact(identity, encodedKey)
	return contact, nil
}

func (s *service) saveContactData(ctx context.Context, contact *userdataobject.Contact) error {
	return s.spaceService.TechSpace().DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.SaveContact(ctx, contact)
	})
}

func (s *service) DeleteContact(ctx context.Context, identity string) error {
	s.identityService.UnregisterIdentity(s.spaceService.TechSpaceId(), identity)
	err := s.spaceService.TechSpace().DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.DeleteContact(ctx, identity)
	})
	if err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}
	return s.store.SpaceIndex(s.spaceService.TechSpaceId()).DeleteDetails(ctx, []string{domain.NewContactId(identity)})
}
