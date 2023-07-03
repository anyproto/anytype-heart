package restriction

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"

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
	sbtProvider typeprovider.SmartBlockTypeProvider
	store       objectstore.ObjectStore
}

func New(sbtProvider typeprovider.SmartBlockTypeProvider, objectStore objectstore.ObjectStore) Service {
	return &service{
		sbtProvider: sbtProvider,
		store:       objectStore,
	}
}

func (s *service) Init(*app.App) (err error) {
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
