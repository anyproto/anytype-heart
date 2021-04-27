package restriction

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const CName = "restriction"

func New() Service {
	return new(service)
}

var ErrRestricted = errors.New("restricted")

var log = logging.Logger("anytype-mw-restrictions")

type Object interface {
	Id() string
	Type() pb.SmartBlockType
}

type Service interface {
	ObjectRestrictionsByObj(obj Object) (r ObjectRestrictions)
	RestrictionsByObj(obj Object) (r Restrictions)
	app.Component
}

type service struct{}

func (s *service) Init(a *app.App) (err error) {
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) RestrictionsByObj(obj Object) (r Restrictions) {
	return Restrictions{
		Object:   s.ObjectRestrictionsByObj(obj),
		Dataview: s.DataviewRestrictionsByObj(obj),
	}
}



type Restrictions struct {
	Object   ObjectRestrictions
	Dataview DataviewRestrictions
}

func (r Restrictions) Proto() *model.Restrictions {
	return &model.Restrictions{
		Object: r.Object,
	}
}
