package source

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type DateSourceParams struct {
	Id               domain.FullID
	DateObjectTypeId string
}

func NewDate(params DateSourceParams) (s Source) {
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

func (d *date) getDetails() (*types.Struct, error) {
	t, includeTime, err := dateutil.ParseDateId(d.id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date id: %w", err)
	}

	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():       pbtypes.String(dateutil.TimeToDateName(t, includeTime)),
		bundle.RelationKeyId.String():         pbtypes.String(d.id),
		bundle.RelationKeyType.String():       pbtypes.String(d.typeId),
		bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String(): pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():   pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():     pbtypes.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji.String():  pbtypes.String("📅"),
		bundle.RelationKeySpaceId.String():    pbtypes.String(d.SpaceID()),
		bundle.RelationKeyTimestamp.String():  pbtypes.Int64(t.Unix()),
	}}, nil
}

func (d *date) DetailsFromId() (*types.Struct, error) {
	return d.getDetails()
}

func (d *date) ReadDoc(context.Context, ChangeReceiver, bool) (doc state.Doc, err error) {
	details, err := d.getDetails()
	if err != nil {
		return
	}

	s := state.NewDoc(d.id, nil).(*state.State)
	template.InitTemplate(s,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithAllBlocksEditsRestricted,
	)
	s.SetDetails(details)
	s.SetObjectTypeKey(bundle.TypeKeyDate)
	return s, nil
}

func (d *date) PushChange(PushChangeParams) (id string, err error) {
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
