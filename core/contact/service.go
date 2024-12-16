package contact

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/editor/userdataobject"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "core.contact.service"

type Service interface {
	app.Component
	SaveContact(ctx context.Context, identity string, profileSymKey []byte) error
	DeleteContact(ctx context.Context, identity string) error
}

type service struct {
	techSpace techspace.TechSpace
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.techSpace = app.MustComponent[techspace.TechSpace](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) SaveContact(ctx context.Context, identity string, profileSymKey []byte) error {
	return s.techSpace.DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.SaveContact(ctx, identity, profileSymKey)
	})
}

func (s *service) DeleteContact(ctx context.Context, identity string) error {
	return s.techSpace.DoUserDataObject(ctx, func(userDataObject userdataobject.UserDataObject) error {
		return userDataObject.DeleteContact(ctx, identity)
	})
}
