package restriction

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relation"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	CheckRestrictions(spaceID string, id string, cr ...model.RestrictionsObjectRestriction) error
	app.Component
}

type service struct {
	sbtProvider     typeprovider.SmartBlockTypeProvider
	objectStore     objectstore.ObjectStore
	relationService relation.Service
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) (err error) {
	s.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.relationService = app.MustComponent[relation.Service](a)
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

func (s *service) CheckRestrictions(spaceID, id string, cr ...model.RestrictionsObjectRestriction) error {
	r, err := s.getRestrictionsById(spaceID, id)
	if err != nil {
		return err
	}
	if err = r.Object.Check(cr...); err != nil {
		return err
	}
	return nil
}

func (s *service) getRestrictionsById(spaceID string, id string) (r Restrictions, err error) {
	sbType, err := s.sbtProvider.Type(spaceID, id)
	if err != nil {
		return Restrictions{}, fmt.Errorf("get smartblock type: %w", err)
	}
	layout := model.ObjectTypeLayout(noLayout)
	d, err := s.objectStore.GetDetails(id)
	var ot string
	if err == nil {
		if pbtypes.HasField(d.GetDetails(), bundle.RelationKeyLayout.String()) {
			layoutIndex := pbtypes.GetInt64(d.GetDetails(), bundle.RelationKeyLayout.String())
			if _, ok := model.ObjectTypeLayout_name[int32(layoutIndex)]; ok {
				layout = model.ObjectTypeLayout(layoutIndex)
			}
		}
		ot = pbtypes.GetString(d.GetDetails(), bundle.RelationKeyType.String())
	}
	var uk domain.UniqueKey
	if u := pbtypes.GetString(d.GetDetails(), bundle.RelationKeyUniqueKey.String()); u != "" {
		uk, err = domain.UnmarshalUniqueKey(u)
		if err != nil {
			log.Errorf("failed to parse unique key %s: %v", u, err)
		}
	}
	obj := newRestrictionHolder(sbType, layout, uk, ot)
	if err != nil {
		return Restrictions{}, err
	}

	return s.GetRestrictions(obj), nil
}
