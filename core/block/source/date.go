package source

import (
	"context"
	"fmt"
	"strings"
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

func (v *date) getDetails(ctx context.Context) (*types.Struct, error) {
	linksRelationId, err := v.space.GetRelationIdByKey(ctx, bundle.RelationKeyLinks)
	if err != nil {
		return nil, fmt.Errorf("get links relation id: %w", err)
	}
	dateTypeId, err := v.space.GetTypeIdByKey(ctx, bundle.TypeKeyDate)
	if err != nil {
		return nil, fmt.Errorf("get date type id: %w", err)
	}
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():       pbtypes.String(v.t.Format("02 Jan 2006")),
		bundle.RelationKeyId.String():         pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String(): pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():   pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():     pbtypes.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji.String():  pbtypes.String("📅"),
		bundle.RelationKeySpaceId.String():    pbtypes.String(v.SpaceID()),
		bundle.RelationKeySetOf.String():      pbtypes.StringList([]string{linksRelationId}),
		bundle.RelationKeyType.String():       pbtypes.String(dateTypeId),
	}}, nil
}

// TODO Fix?
func (v *date) DetailsFromId() (*types.Struct, error) {
	if err := v.parseId(); err != nil {
		return nil, err
	}
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():       pbtypes.String(v.t.Format("02 Jan 2006")),
		bundle.RelationKeyId.String():         pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String(): pbtypes.Bool(false),
		bundle.RelationKeyIsHidden.String():   pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():     pbtypes.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji.String():  pbtypes.String("📅"),
		bundle.RelationKeySpaceId.String():    pbtypes.String(v.SpaceID()),
	}}, nil
}

func (v *date) parseId() error {
	t, err := time.Parse("2006-01-02", strings.TrimPrefix(v.id, addr.DatePrefix))
	if err != nil {
		return err
	}
	v.t = t
	return nil
}

func (v *date) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	if err = v.parseId(); err != nil {
		return
	}
	s := state.NewDoc(v.id, nil).(*state.State)
	d, err := v.getDetails(ctx)
	if err != nil {
		return
	}
	dataview := &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{
				{
					Key:    bundle.RelationKeyName.String(),
					Format: model.RelationFormat_shorttext,
				},
				{
					Key:    bundle.RelationKeyLastModifiedDate.String(),
					Format: model.RelationFormat_date,
				},
			},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "1",
					Type: model.BlockContentDataviewView_Table,
					Name: "Date backlinks",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: bundle.RelationKeyLastModifiedDate.String(),
							Type:        model.BlockContentDataviewSort_Desc,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{
						{
							RelationKey: bundle.RelationKeyLinks.String(),
							Condition:   model.BlockContentDataviewFilter_In,
							Value:       pbtypes.String(v.id),
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyLastModifiedDate.String(),
							IsVisible: true,
						},
					},
				},
			},
		},
	}

	template.InitTemplate(s,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDataview(dataview, true),
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
