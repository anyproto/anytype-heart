package restriction

import (
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CName    = "restriction"
	noLayout = -1
)

var (
	ErrRestricted = errors.New("restricted")

	log = logging.Logger("anytype-mw-restrictions")
)

type Service interface {
	GetRestrictions(RestrictionHolder) Restrictions
	CheckRestrictions(rh RestrictionHolder, cr ...model.RestrictionsObjectRestriction) error
	app.Component
}

type service struct {
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetRestrictions(rh RestrictionHolder) (r Restrictions) {
	return Restrictions{
		Object:   s.getObjectRestrictions(rh),
		Dataview: s.getDataviewRestrictions(rh),
	}
}

func (s *service) CheckRestrictions(rh RestrictionHolder, cr ...model.RestrictionsObjectRestriction) error {
	r := s.getObjectRestrictions(rh)
	if err := r.Check(cr...); err != nil {
		return err
	}
	return nil
}
