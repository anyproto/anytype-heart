package source

import (
	"context"
	"fmt"

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
	// TODO: GO-4494 - Remove links relation id
	LinksRelationId string
}

func NewDate(params DateSourceParams) (s Source) {
	return &date{
		id:      params.Id.ObjectID,
		spaceId: params.Id.SpaceID,
		typeId:  params.DateObjectTypeId,
		linksId: params.LinksRelationId,
	}
}

type date struct {
	id, spaceId, typeId string
	// TODO: GO-4494 - Remove links relation id
	linksId string
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
	t, err := dateutil.ParseDateId(d.id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date id: %w", err)
	}

	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{

		bundle.RelationKeyName:       domain.String(dateutil.TimeToDateName(t)),
		bundle.RelationKeyId:         domain.String(d.id),
		bundle.RelationKeyType:       domain.String(d.typeId),
		bundle.RelationKeyIsReadonly: domain.Bool(true),
		bundle.RelationKeyIsArchived: domain.Bool(false),
		bundle.RelationKeyIsHidden:   domain.Bool(false),
		bundle.RelationKeyLayout:     domain.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji:  domain.String("📅"),
		bundle.RelationKeySpaceId:    domain.String(d.SpaceID()),
		bundle.RelationKeyTimestamp:  domain.Int64(t.Unix()),
		// TODO: GO-4494 - Remove links relation id
		bundle.RelationKeySetOf: domain.StringList([]string{d.linksId}),
	}), nil
}

func (d *date) DetailsFromId() (*domain.Details, error) {
	return d.getDetails()
}

func (d *date) ReadDoc(context.Context, ChangeReceiver, bool) (doc state.Doc, err error) {
	details, err := d.getDetails()
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
							Value:       pbtypes.String(d.id),
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

	s := state.NewDoc(d.id, nil).(*state.State)
	template.InitTemplate(s,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		// TODO: GO-4494 - Remove dataview block insertion
		template.WithDataview(dataview, true),
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
