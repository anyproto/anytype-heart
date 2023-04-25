package restriction

import (
	"errors"
	"fmt"

	"github.com/anytypeio/any-sync/app"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
)

const CName = "restriction"

var ErrRestricted = errors.New("restricted")

var log = logging.Logger("anytype-mw-restrictions")

type Service interface {
	ObjectRestrictionsByObj(obj Object) (r ObjectRestrictions)
	RestrictionsByObj(obj Object) (r Restrictions)
	RestrictionsById(id string) (r Restrictions, err error)
	CheckRestrictions(id string, cr ...model.RestrictionsObjectRestriction) error
	app.Component
}

type service struct {
	anytype     core.Service
	sbtProvider typeprovider.SmartBlockTypeProvider
}

func New(sbtProvider typeprovider.SmartBlockTypeProvider) Service {
	return &service{
		sbtProvider: sbtProvider,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
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
	sbType, err := s.sbtProvider.Type(id)
	if err != nil {
		return Restrictions{}, fmt.Errorf("get smartblock type: %w", err)
	}
	obj, err := newSimpleObject(id, sbType)
	if err != nil {
		return Restrictions{}, err
	}
	return s.RestrictionsByObj(obj), nil
}

type simpleObject struct {
	id string
	tp model.SmartBlockType
}

func newSimpleObject(id string, sbType smartblock.SmartBlockType) (Object, error) {
	return &simpleObject{
		id: id,
		tp: sbType.ToProto(),
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

func (r Restrictions) Equal(r2 Restrictions) bool {
	return r.Object.Equal(r2.Object) && r.Dataview.Equal(r2.Dataview)
}

func (r Restrictions) Copy() Restrictions {
	return Restrictions{
		Object:   r.Object.Copy(),
		Dataview: r.Dataview.Copy(),
	}
}
