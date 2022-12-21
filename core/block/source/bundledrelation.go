package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewBundledRelation(a core.Service, id string) (s Source) {
	return &bundledRelation{
		id: id,
		a:  a,
	}
}

type bundledRelation struct {
	id string
	a  core.Service
}

func (v *bundledRelation) ReadOnly() bool {
	return true
}

func (v *bundledRelation) Id() string {
	return v.id
}

func (v *bundledRelation) Anytype() core.Service {
	return v.a
}

func (v *bundledRelation) Type() model.SmartBlockType {
	return model.SmartBlockType_BundledRelation
}

func (v *bundledRelation) Virtual() bool {
	return true
}

func (v *bundledRelation) getDetails(id string) (p *types.Struct, err error) {
	if !strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return nil, fmt.Errorf("incorrect relation id: not a bundled relation id")
	}

	rel, err := bundle.GetRelation(bundle.RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)))
	if err != nil {
		return nil, err
	}
	rel.Creator = addr.AnytypeProfileId
	details := bundle.GetDetailsForRelation(true, rel)
	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(addr.AnytypeMarketplaceWorkspace)
	details.Fields[bundle.RelationKeyIsReadonly.String()] = pbtypes.Bool(true)
	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyRelation.BundledURL())

	return details, nil
}

func (v *bundledRelation) ReadDoc(_ context.Context, _ ChangeReceiver, empty bool) (doc state.Doc, err error) {
	s := state.NewDoc(v.id, nil).(*state.State)

	d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}
	for k, v := range d.Fields {
		s.SetDetailAndBundledRelation(bundle.RelationKey(k), v)
	}
	s.SetObjectType(bundle.TypeKeyRelation.BundledURL())
	return s, nil
}

func (v *bundledRelation) PushChange(params PushChangeParams) (id string, err error) {
	if params.State.ChangeId() == "" {
		// allow the first changes created by Init
		return "virtual", nil
	}
	return "", ErrReadOnly
}

func (v *bundledRelation) FindFirstChange(ctx context.Context) (c *change.Change, err error) {
	return nil, change.ErrEmpty
}

func (v *bundledRelation) ListIds() ([]string, error) {
	return bundle.ListRelationsUrls(), nil
}

func (v *bundledRelation) Close() (err error) {
	return
}

func (v *bundledRelation) LogHeads() map[string]string {
	return nil
}

func (s *bundledRelation) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}
