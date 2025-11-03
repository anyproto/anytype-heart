package restriction

import (
	"errors"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var ErrRestricted = errors.New("restricted")

type RestrictionHolder interface {
	Type() smartblock.SmartBlockType
	Layout() (model.ObjectTypeLayout, bool)
	UniqueKey() domain.UniqueKey
	LocalDetails() *domain.Details
}

func GetRestrictions(rh RestrictionHolder) (r Restrictions) {
	return Restrictions{
		Object:   getObjectRestrictions(rh),
		Dataview: getDataviewRestrictions(rh),
	}
}

func CheckRestrictions(rh RestrictionHolder, cr ...model.RestrictionsObjectRestriction) error {
	r := getObjectRestrictions(rh)
	if err := r.Check(cr...); err != nil {
		return err
	}
	return nil
}

type Restrictions struct {
	Object   ObjectRestrictions
	Dataview DataviewRestrictions
}

func (r Restrictions) Proto() *model.Restrictions {
	res := &model.Restrictions{
		Object: r.Object.ToProto(),
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
