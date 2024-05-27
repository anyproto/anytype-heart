package restriction

import (
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "restriction"

var ErrRestricted = errors.New("restricted")

type RestrictionHolder interface {
	Type() smartblock.SmartBlockType
	Layout() (model.ObjectTypeLayout, bool)
	UniqueKey() domain.UniqueKey
}

type Service interface {
	GetRestrictions(RestrictionHolder) Restrictions
	CheckRestrictions(rh RestrictionHolder, cr ...model.RestrictionsObjectRestriction) error
	app.Component
}

type service struct{}

func New() Service {
	return &service{}
}

func (s *service) Init(*app.App) (err error) {
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) GetRestrictions(rh RestrictionHolder) (r Restrictions) {
	return Restrictions{
		Object:   getObjectRestrictions(rh),
		Dataview: getDataviewRestrictions(rh),
	}
}

func (s *service) CheckRestrictions(rh RestrictionHolder, cr ...model.RestrictionsObjectRestriction) error {
	r := getObjectRestrictions(rh)
	if err := r.Check(cr...); err != nil {
		return err
	}
	return nil
}
