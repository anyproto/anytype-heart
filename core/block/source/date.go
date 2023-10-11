package source

import (
	"context"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func NewDate(spaceID string, id string, coreService core.Service) (s Source) {
	return &date{
		id:          id,
		spaceID:     spaceID,
		coreService: coreService,
	}
}

type date struct {
	id          string
	spaceID     string
	t           time.Time
	coreService core.Service
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
	return v.spaceID
}

func (v *date) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeDate
}

func (v *date) getDetails() (p *types.Struct) {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():       pbtypes.String(v.t.Format("Mon Jan  2 2006")),
		bundle.RelationKeyId.String():         pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String(): pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String(): pbtypes.Bool(false),
		bundle.RelationKeySetOf.String():      pbtypes.String(v.coreService.GetSystemRelationID(v.spaceID, bundle.RelationKeyLinks)),
		bundle.RelationKeyType.String():       pbtypes.String(v.coreService.GetSystemTypeID(v.spaceID, bundle.TypeKeyDate)),
		bundle.RelationKeyIsHidden.String():   pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():     pbtypes.Float64(float64(model.ObjectType_date)),
		bundle.RelationKeyIconEmoji.String():  pbtypes.String("📅"),
		bundle.RelationKeySpaceId.String():    pbtypes.String(v.spaceID),
	}}
}

func (v *date) DetailsFromId() (*types.Struct, error) {
	if err := v.parseId(); err != nil {
		return nil, err
	}
	return v.getDetails(), nil
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
	d := v.getDetails()
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

func (s *date) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *date) GetCreationInfo() (creator string, createdDate int64, err error) {
	return s.coreService.ProfileID(s.spaceID), 0, nil
}
