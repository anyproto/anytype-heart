package sourceimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func NewBundledRelation(id string) (s source.Source) {
	return &bundledRelation{
		id:     id,
		relKey: domain.RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)),
	}
}

type bundledRelation struct {
	id     string
	relKey domain.RelationKey
}

func (v *bundledRelation) ReadOnly() bool {
	return true
}

func (v *bundledRelation) Id() string {
	return v.id
}

func (v *bundledRelation) SpaceID() string {
	return addr.AnytypeMarketplaceWorkspace
}

func (v *bundledRelation) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeBundledRelation
}

func (v *bundledRelation) getDetails(id string) (p *domain.Details, err error) {
	if !strings.HasPrefix(id, addr.BundledRelationURLPrefix) {
		return nil, fmt.Errorf("incorrect relation id: not a bundled relation id")
	}

	rel, err := bundle.GetRelation(domain.RelationKey(strings.TrimPrefix(id, addr.BundledRelationURLPrefix)))
	if err != nil {
		return nil, err
	}
	rel.Creator = addr.AnytypeProfileId
	wrapperRelation := relationutils.Relation{Relation: rel}
	details := wrapperRelation.ToDetails() // bundle.GetDetailsForBundledRelation(rel)
	details.SetString(bundle.RelationKeySpaceId, addr.AnytypeMarketplaceWorkspace)
	details.SetBool(bundle.RelationKeyIsReadonly, true)
	details.SetString(bundle.RelationKeyType, bundle.TypeKeyRelation.BundledURL())
	details.SetString(bundle.RelationKeyId, id)
	details.SetInt64(bundle.RelationKeyOrigin, int64(model.ObjectOrigin_builtin))

	return details, nil
}

func (v *bundledRelation) ReadDoc(_ context.Context, _ source.ChangeReceiver, empty bool) (doc state.Doc, err error) {
	// we use STRelation instead of BundledRelation for a reason we want to have the same prefix
	// ideally the whole logic should be done on the level of spaceService to return the virtual space for marketplace
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, v.relKey.String())
	if err != nil {
		return nil, err
	}

	s := state.NewDocWithUniqueKey(v.id, nil, uk).(*state.State)
	d, err := v.getDetails(v.id)
	if err != nil {
		// it is either not found or invalid id. We return not found for both cases

		return nil, domain.ErrObjectNotFound
	}
	for k, v := range d.Iterate() {
		s.SetDetailAndBundledRelation(k, v)
	}
	s.SetObjectTypeKey(bundle.TypeKeyRelation)
	return s, nil
}

func (v *bundledRelation) PushChange(params source.PushChangeParams) (id string, err error) {
	if params.State.ChangeId() == "" {
		// allow the first changes created by Init
		return "virtual", nil
	}
	return "", source.ErrReadOnly
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

func (s *bundledRelation) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
