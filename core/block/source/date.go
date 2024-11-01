package source

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	dateutil "github.com/anyproto/anytype-heart/util/date"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func NewDate(space Space, id domain.FullID) (s Source) {
	return &date{
		id:      id.ObjectID,
		spaceId: id.SpaceID,
		space:   space,
	}
}

type date struct {
	space       Space
	id, spaceId string
	t           time.Time
}

func (v *date) ListIds() ([]string, error) {
	return []string{}, nil
}

func (v *date) ReadOnly() bool {
	return true
}

func (v *date) Id() string {
	return v.id
}

func (v *date) SpaceID() string {
	if v.space != nil {
		return v.space.Id()
	}
	if v.spaceId != "" {
		return v.spaceId
	}
	return ""
}

func (v *date) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeDate
}

func (v *date) getDetails(ctx context.Context, withType bool) (*types.Struct, error) {
	t, err := dateutil.ParseDateId(v.id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date id: %w", err)
	}

	details := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():       pbtypes.String(dateutil.TimeToDateName(t)),
		bundle.RelationKeyId.String():         pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String(): pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():   pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():     pbtypes.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji.String():  pbtypes.String("📅"),
		bundle.RelationKeySpaceId.String():    pbtypes.String(v.SpaceID()),
		bundle.RelationKeyTimestamp.String():  pbtypes.Int64(t.Unix()),
	}}

	if withType {
		if v.space == nil {
			return nil, fmt.Errorf("get date type id: space is nil")
		}
		dateTypeId, err := v.space.GetTypeIdByKey(ctx, bundle.TypeKeyDate)
		if err != nil {
			return nil, fmt.Errorf("get date type id: %w", err)
		}
		details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(dateTypeId)
	}
	return details, nil
}

func (v *date) DetailsFromId() (*types.Struct, error) {
	return v.getDetails(nil, false)
}

func (v *date) parseId() error {
	t, err := dateutil.ParseDateId(v.id)
	if err != nil {
		return err
	}
	v.t = t
	return nil
}

func (v *date) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	d, err := v.getDetails(ctx, true)
	if err != nil {
		return
	}

	s := state.NewDoc(v.id, nil).(*state.State)
	template.InitTemplate(s,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithAllBlocksEditsRestricted,
	)
	s.SetDetails(d)
	s.SetObjectTypeKey(bundle.TypeKeyDate)
	return s, nil
}

func (v *date) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *date) Close() (err error) {
	return
}

func (v *date) Heads() []string {
	return []string{v.id}
}

func (v *date) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (v *date) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
