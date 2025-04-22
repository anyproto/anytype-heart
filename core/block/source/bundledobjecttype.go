package source

import (
	"context"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func NewBundledObjectType(id string) (s Source) {
	return &bundledObjectType{
		id:            id,
		objectTypeKey: domain.TypeKey(strings.TrimPrefix(id, addr.BundledObjectTypeURLPrefix)),
	}
}

type bundledObjectType struct {
	id            string
	objectTypeKey domain.TypeKey
}

func (v *bundledObjectType) ReadOnly() bool {
	return true
}

func (v *bundledObjectType) Id() string {
	return v.id
}

func (v *bundledObjectType) SpaceID() string {
	return addr.AnytypeMarketplaceWorkspace
}

func (v *bundledObjectType) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeBundledObjectType
}

func getDetailsForBundledObjectType(id string) (extraRels []domain.RelationKey, p *domain.Details, err error) {
	ot, err := bundle.GetTypeByUrl(id)
	if err != nil {
		return nil, nil, err
	}

	for _, rl := range ot.RelationLinks {
		extraRels = append(extraRels, domain.RelationKey(rl.Key))
	}

	return extraRels, (&relationutils.ObjectType{ot}).BundledTypeDetails(), nil
}

func (v *bundledObjectType) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	// we use STType instead of BundledObjectType for a reason we want to have the same prefix
	// ideally the whole logic should be done on the level of spaceService to return the virtual space for marketplace
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, v.objectTypeKey.String())
	if err != nil {
		return nil, err
	}

	s := state.NewDocWithUniqueKey(v.id, nil, uk).(*state.State)
	rels, d, err := getDetailsForBundledObjectType(v.id)
	if err != nil {
		// it is either not found or invalid id. We return not found for both cases
		return nil, domain.ErrObjectNotFound
	}
	s.AddRelationKeys(rels...)
	s.SetDetails(d)
	s.SetDetail(bundle.RelationKeyOrigin, domain.Int64(model.ObjectOrigin_builtin))
	s.SetObjectTypeKey(bundle.TypeKeyObjectType)
	return s, nil
}

func (v *bundledObjectType) PushChange(params PushChangeParams) (id string, err error) {
	return "", nil
}

func (v *bundledObjectType) ListIds() ([]string, error) {
	var ids []string
	for _, tk := range bundle.ListTypesKeys() {
		ids = append(ids, tk.BundledURL())
	}
	return ids, nil
}

func (v *bundledObjectType) Close() (err error) {
	return
}

func (v *bundledObjectType) Heads() []string {
	return []string{"todo"} // todo hash of model
}

func (s *bundledObjectType) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *bundledObjectType) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}
