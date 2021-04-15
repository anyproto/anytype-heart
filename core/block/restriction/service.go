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
	ObjectRestrictionsById(obj Object) (r ObjectRestrictions)
	app.Component
}

type service struct{}

func (s *service) Init(a *app.App) (err error) {
	return
}

func (s *service) Name() (name string) {
	return CName
}

type Restrictions struct {
	Object ObjectRestrictions
}

func (r Restrictions) Proto() *model.Restrictions {
	return &model.Restrictions{
		Object: r.Object,
	}
}
