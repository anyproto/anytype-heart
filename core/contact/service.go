package contact

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
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
	AddObserver(spaceId, identity string, observer func(identity string, profile *model.IdentityProfile))
	WaitProfile(ctx context.Context, identity string) *model.IdentityProfile
}

type service struct {
	techSpace       techspace.TechSpace
	identityService identityService
	ctx             context.Context
	cancel          context.CancelFunc
}

func (s *service) Run(ctx context.Context) (err error) {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return
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
	spaceService := app.MustComponent[space.Service](a)
	s.techSpace = spaceService.TechSpace()
	s.identityService = app.MustComponent[identityService](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) SaveContact(ctx context.Context, identity string, profileSymKey string) error {
	err := s.registerIdentity(identity, profileSymKey)
	if err != nil {
		return err
	}
	profile := s.identityService.WaitProfile(ctx, identity)
	if profile == nil {
		return fmt.Errorf("no profile for identity %s", identity)
	}
	return s.techSpace.DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.SaveContact(ctx, profile)
	})
}

func (s *service) registerIdentity(identity string, profileSymKey string) error {
	handleIdentityUpdate := func(identity string, identityProfile *model.IdentityProfile) {
		err := s.techSpace.DoUserDataObject(s.ctx, func(userDataObject userdataobject.UserDataObject) error {
			return userDataObject.UpdateContactByIdentity(s.ctx, identityProfile)
		})
		if err != nil {
			log.Errorf("failed to update user data object for identity %s: %v", identity, err)
		}
	}
	if len(profileSymKey) == 0 {
		s.identityService.AddObserver(s.techSpace.TechSpaceId(), identity, handleIdentityUpdate)
	} else {
		key, err := getAesKey(profileSymKey)
		if err != nil {
			return fmt.Errorf("get aes key for identity %s: %w", identity, err)
		}
		err = s.identityService.RegisterIdentity(s.techSpace.TechSpaceId(), identity, key, handleIdentityUpdate)
		if err != nil {
			return fmt.Errorf("register identity %s: %v", identity, err)
		}
	}
	return nil
}

func getAesKey(profileSymKey string) (*crypto.AESKey, error) {
	key, err := crypto.UnmarshallAESKeyString(profileSymKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *service) DeleteContact(ctx context.Context, identity string) error {
	err := s.techSpace.DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.DeleteContact(ctx, identity)
	})
	if err != nil {
		return err
	}
	s.identityService.UnregisterIdentity(s.techSpace.TechSpaceId(), identity)
	return nil
}
