package source

import (
	"context"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func NewDate(id string, coreService core.Service) (s Source) {
	return &date{
		id:          id,
		coreService: coreService,
	}
}

type date struct {
	id          string
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

func (v *date) Type() model.SmartBlockType {
	return model.SmartBlockType_Date
}

func (v *date) getDetails() (p *types.Struct) {
	return &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyName.String():        pbtypes.String(v.t.Format("Mon Jan  2 2006")),
		bundle.RelationKeyId.String():          pbtypes.String(v.id),
		bundle.RelationKeyIsReadonly.String():  pbtypes.Bool(true),
		bundle.RelationKeyIsArchived.String():  pbtypes.Bool(false),
		bundle.RelationKeyType.String():        pbtypes.String(bundle.TypeKeyDate.URL()),
		bundle.RelationKeyIsHidden.String():    pbtypes.Bool(false),
		bundle.RelationKeyLayout.String():      pbtypes.Float64(float64(model.ObjectType_basic)),
		bundle.RelationKeyIconEmoji.String():   pbtypes.String("📅"),
		bundle.RelationKeyWorkspaceId.String(): pbtypes.String(v.coreService.PredefinedBlocks().Account),
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

	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyDate.URL())
	return s, nil
}

func (v *date) ReadMeta(ctx context.Context, _ ChangeReceiver) (doc state.Doc, err error) {
	if err = v.parseId(); err != nil {
		return
	}
	s := &state.State{}
	d := v.getDetails()

	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyDate.URL())
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
