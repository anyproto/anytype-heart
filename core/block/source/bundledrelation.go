package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func NewBundledRelation(id string) (s Source) {
	return &bundledRelation{
		id:     id,
		relKey: bundle.RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)),
	}
}

type bundledRelation struct {
	id     string
	relKey bundle.RelationKey
}

func (v *bundledRelation) ReadOnly() bool {
	return true
}

func (v *bundledRelation) Id() string {
	return v.id
}

func (v *bundledRelation) Type() model.SmartBlockType {
	return model.SmartBlockType_BundledRelation
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
	details := bundle.GetDetailsForBundledRelation(rel)
	details.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(addr.AnytypeMarketplaceWorkspace)
	details.Fields[bundle.RelationKeySpaceId.String()] = pbtypes.String(addr.AnytypeMarketplaceWorkspace)
	details.Fields[bundle.RelationKeyIsReadonly.String()] = pbtypes.Bool(true)
	details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(bundle.TypeKeyRelation.BundledURL())
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)

	return details, nil
}

func (v *bundledRelation) ReadDoc(_ context.Context, _ ChangeReceiver, empty bool) (doc state.Doc, err error) {
	// we use STRelation instead of BundledRelation for a reason we want to have the same prefix
	// ideally the whole logic should be done on the level of spaceService to return the virtual space for marketplace
	uk, err := uniquekey.NewUniqueKey(model.SmartBlockType_STRelation, v.relKey.String())
	if err != nil {
		return nil, err
	}

	s := state.NewDocWithUniqueKey(v.id, nil, uk).(*state.State)
	d, err := v.getDetails(v.id)
	if err != nil {
		return nil, err
	}
	for k, v := range d.Fields {
		s.SetDetailAndBundledRelation(bundle.RelationKey(k), v)
	}
	s.SetObjectType(bundle.TypeKeyRelation.String())
	s.InjectDerivedDetails(&bundledTypeIdGetter{}, addr.AnytypeMarketplaceWorkspace, uk.SmartblockType())
	return s, nil
}

func (v *bundledRelation) PushChange(params PushChangeParams) (id string, err error) {
	if params.State.ChangeId() == "" {
		// allow the first changes created by Init
		return "virtual", nil
	}
	return "", ErrReadOnly
}

func (v *bundledRelation) ListIds() ([]string, error) {
	return bundle.ListRelationsUrls(), nil
}

func (v *bundledRelation) Close() (err error) {
	return
}

func (v *bundledRelation) Heads() []string {
	return []string{"todo"} // todo hash of model
}

func (s *bundledRelation) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *bundledRelation) GetCreationInfo() (creator string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
