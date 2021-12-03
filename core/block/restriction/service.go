package restriction

import (
	"errors"
	smartblock2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const CName = "restriction"

func New() Service {
	return new(service)
}

var ErrRestricted = errors.New("restricted")

var log = logging.Logger("anytype-mw-restrictions")

type simpleObject struct {
	id string
	tp model.SmartBlockType
}

func newSimpleObject(id string) (Object, error) {
	tp, err := smartblock2.SmartBlockTypeFromID(id)
	if err != nil {
		return nil, err
	}
	return &simpleObject{
		id: id,
		tp: tp.ToProto(),
	}, nil
}

func (s *simpleObject) Id() string {
	return s.id
}

func (s *simpleObject) Type() model.SmartBlockType {
	return s.tp
}

type Object interface {
	Id() string
	Type() model.SmartBlockType
}

type Service interface {
	ObjectRestrictionsByObj(obj Object) (r ObjectRestrictions)
	RestrictionsByObj(obj Object) (r Restrictions)
	RestrictionsById(id string) (r Restrictions, err error)
	CheckRestrictions(id string, cr ...model.RestrictionsObjectRestriction) error
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

func (s *service) CheckRestrictions(id string, cr ...model.RestrictionsObjectRestriction) error {
	r, err := s.RestrictionsById(id)
	if err != nil {
		return err
	}
	if err = r.Object.Check(cr...); err != nil {
		return err
	}
	return nil
}

func (s *service) RestrictionsById(id string) (r Restrictions, err error) {
	obj, err := newSimpleObject(id)
	if err != nil {
		return Restrictions{}, err
	}
	return s.RestrictionsByObj(obj), nil
}

type Restrictions struct {
	Object   ObjectRestrictions
	Dataview DataviewRestrictions
}

func (r Restrictions) Proto() *model.Restrictions {
	res := &model.Restrictions{
		Object: r.Object,
	}
	if len(r.Dataview) > 0 {
		res.Dataview = make([]*model.RestrictionsDataviewRestrictions, 0, len(r.Dataview))
		for _, dvr := range r.Dataview {
			res.Dataview = append(res.Dataview, &dvr)
		}
	}
	return res
}

func (r Restrictions) Copy() Restrictions {
	return Restrictions{
		Object:   r.Object.Copy(),
		Dataview: r.Dataview.Copy(),
	}
}
