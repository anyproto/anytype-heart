package contact

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
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
	WaitProfile(ctx context.Context, identity string) *model.IdentityProfile
	GetIdentityKey(identity string) crypto.SymKey
}

type service struct {
	identityService identityService
	ctx             context.Context
	cancel          context.CancelFunc
	spaceService    space.Service
}

func (s *service) Run(ctx context.Context) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s.registerExistingContacts(ctx)
}

func (s *service) registerExistingContacts(ctx context.Context) error {
	var (
		contacts []*userdataobject.Contact
		listErr  error
	)
	err := s.spaceService.TechSpace().DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
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
		key, err := getAesKeyFromBase64(contact.Key())
		if err != nil {
			log.Errorf("failed to register contact identity %s", err)
			continue
		}
		err = s.identityService.RegisterIdentity(s.spaceService.TechSpaceId(), contact.Identity(), key, s.handleIdentityUpdate)
		if err != nil {
			log.Errorf("failed to register contact identity %s", err)
		}
	}
	return err
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.cancel != nil {
		s.cancel()
	}
	return
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.spaceService = app.MustComponent[space.Service](a)
	s.identityService = app.MustComponent[identityService](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) SaveContact(ctx context.Context, identity string, profileSymKey string) error {
	symKey, err := s.getIdentityKey(identity, profileSymKey)
	if err != nil {
		return err
	}
	err = s.identityService.RegisterIdentity(s.spaceService.TechSpaceId(), identity, symKey, s.handleIdentityUpdate)
	if err != nil {
		return fmt.Errorf("register identity %s: %v", identity, err)
	}
	profile := s.identityService.WaitProfile(ctx, identity)
	if profile == nil {
		return fmt.Errorf("no profile for identity %s", identity)
	}
	return s.spaceService.TechSpace().DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.SaveContact(ctx, profile, symKey)
	})
}

func (s *service) getIdentityKey(identity string, profileSymKey string) (crypto.SymKey, error) {
	var (
		symKey crypto.SymKey
		err    error
	)
	if len(profileSymKey) == 0 {
		symKey = s.identityService.GetIdentityKey(identity)
	} else {
		symKey, err = getAesKey(profileSymKey)
		if err != nil {
			return nil, fmt.Errorf("get aes key for identity %s: %w", identity, err)
		}
	}
	return symKey, nil
}

func getAesKeyFromBase64(encodedProfileSymKey string) (*crypto.AESKey, error) {
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

func getAesKey(profileSymKey string) (*crypto.AESKey, error) {
	key, err := crypto.UnmarshallAESKeyString(profileSymKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *service) DeleteContact(ctx context.Context, identity string) error {
	err := s.spaceService.TechSpace().DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.DeleteContact(ctx, identity)
	})
	if err != nil {
		return fmt.Errorf("failed to delete contact: %w", err)
	}
	s.identityService.UnregisterIdentity(s.spaceService.TechSpaceId(), identity)
	return nil
}

func (s *service) handleIdentityUpdate(identity string, identityProfile *model.IdentityProfile) {
	err := s.spaceService.TechSpace().DoUserDataObject(s.ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.UpdateContactByIdentity(s.ctx, identityProfile)
	})
	if err != nil {
		log.Errorf("failed to update contact for identity %s: %v", identity, err)
	}
}
