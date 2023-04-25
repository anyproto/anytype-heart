package restriction

import (
	"errors"
	"fmt"

	"github.com/anytypeio/any-sync/app"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const CName = "restriction"

var (
	ErrRestricted = errors.New("restricted")

	log = logging.Logger("anytype-mw-restrictions")
)

type Service interface {
	GetRestrictions(RestrictionHolder) Restrictions
	CheckRestrictions(id string, cr ...model.RestrictionsObjectRestriction) error
	app.Component
}

type service struct {
	anytype     core.Service
	sbtProvider typeprovider.SmartBlockTypeProvider
	store       objectstore.ObjectStore
}

func New(sbtProvider typeprovider.SmartBlockTypeProvider) Service {
	return &service{
		sbtProvider: sbtProvider,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.anytype = a.MustComponent(core.CName).(core.Service)
	s.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)

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

func (s *service) CheckRestrictions(id string, cr ...model.RestrictionsObjectRestriction) error {
	r, err := s.getRestrictionsById(id)
	if err != nil {
		return err
	}
	if err = r.Object.Check(cr...); err != nil {
		return err
	}
	return nil
}

func (s *service) getRestrictionsById(id string) (r Restrictions, err error) {
	sbType, err := s.sbtProvider.Type(id)
	if err != nil {
		return Restrictions{}, fmt.Errorf("get smartblock type: %w", err)
	}
	layout := model.ObjectTypeLayout(-1)
	d, err := s.store.GetDetails(id)
	if err == nil {
		if pbtypes.HasField(d.GetDetails(), bundle.RelationKeyLayout.String()) {
			layoutIndex := pbtypes.GetInt64(d.GetDetails(), bundle.RelationKeyLayout.String())
			if _, ok := model.ObjectTypeLayout_name[int32(layoutIndex)]; ok {
				layout = model.ObjectTypeLayout(layoutIndex)
			}
		}
	}
	obj := newRestrictionHolder(id, sbType, layout)
	if err != nil {
		return Restrictions{}, err
	}

	return s.GetRestrictions(obj), nil
}
