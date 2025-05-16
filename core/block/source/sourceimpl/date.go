package sourceimpl

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
)

type DateSourceParams struct {
	Id               domain.FullID
	DateObjectTypeId string
}

func NewDate(params DateSourceParams) (s source.Source) {
	return &date{
		id:      params.Id.ObjectID,
		spaceId: params.Id.SpaceID,
		typeId:  params.DateObjectTypeId,
	}
}

type date struct {
	id, spaceId, typeId string
}

func (d *date) ListIds() ([]string, error) {
	return []string{}, nil
}

func (d *date) ReadOnly() bool {
	return true
}

func (d *date) Id() string {
	return d.id
}

func (d *date) SpaceID() string {
	return d.spaceId
}

func (d *date) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeDate
}

func (d *date) getDetails() (*domain.Details, error) {
	dateObject, err := dateutil.BuildDateObjectFromId(d.id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date id: %w", err)
	}
	restrictions := restriction.GetRestrictionsBySBType(smartblock.SmartBlockTypeDate)

	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyName:           domain.String(dateObject.Name()),
		bundle.RelationKeyId:             domain.String(d.id),
		bundle.RelationKeyType:           domain.String(d.typeId),
		bundle.RelationKeyIsReadonly:     domain.Bool(true),
		bundle.RelationKeyIsArchived:     domain.Bool(false),
		bundle.RelationKeyIsHidden:       domain.Bool(false),
		bundle.RelationKeyResolvedLayout: domain.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyLayout:         domain.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji:      domain.String("📅"),
		bundle.RelationKeySpaceId:        domain.String(d.SpaceID()),
		bundle.RelationKeyTimestamp:      domain.Int64(dateObject.Time().Unix()),
		bundle.RelationKeyRestrictions:   restrictions,
	}), nil
}

func (d *date) DetailsFromId() (*domain.Details, error) {
	return d.getDetails()
}

func (d *date) ReadDoc(context.Context, source.ChangeReceiver, bool) (doc state.Doc, err error) {
	details, err := d.getDetails()
	if err != nil {
		return
	}

	s := state.NewDoc(d.id, nil).(*state.State)
	template.InitTemplate(s,
		template.WithTitle,
		template.WithAllBlocksEditsRestricted,
	)
	s.SetDetails(details)
	s.SetObjectTypeKey(bundle.TypeKeyDate)
	return s, nil
}

func (d *date) PushChange(source.PushChangeParams) (id string, err error) {
	return "", nil
}

func (d *date) Close() (err error) {
	return
}

func (d *date) Heads() []string {
	return []string{d.id}
}

func (d *date) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (d *date) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
