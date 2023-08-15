package source

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func NewBundledObjectType(id string) (s Source) {
	return &bundledObjectType{
		id:      id,
		typeKey: bundle.TypeKey(strings.TrimPrefix(id, addr.BundledObjectTypeURLPrefix)),
	}
}

type bundledObjectType struct {
	id      string
	typeKey bundle.TypeKey
}

func (v *bundledObjectType) ReadOnly() bool {
	return true
}

func (v *bundledObjectType) Id() string {
	return v.id
}

func (v *bundledObjectType) Type() model.SmartBlockType {
	return model.SmartBlockType_BundledObjectType
}

func getDetailsForBundledObjectType(id string) (extraRels []*model.RelationLink, p *types.Struct, err error) {
	ot, err := bundle.GetTypeByUrl(id)
	if err != nil {
		return nil, nil, err
	}
	extraRels = []*model.RelationLink{bundle.MustGetRelationLink(bundle.RelationKeyRecommendedRelations), bundle.MustGetRelationLink(bundle.RelationKeyRecommendedLayout)}

	for _, rl := range ot.RelationLinks {
		relationLink := &model.RelationLink{
			Key:    rl.Key,
			Format: rl.Format,
		}
		extraRels = append(extraRels, relationLink)
	}

	return extraRels, (&relationutils.ObjectType{ot}).BundledTypeDetails(), nil
}

func (v *bundledObjectType) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	// we use STType instead of BundledObjectType for a reason we want to have the same prefix
	// ideally the whole logic should be done on the level of spaceService to return the virtual space for marketplace
	uk, err := uniquekey.New(model.SmartBlockType_STType, v.typeKey.String())
	if err != nil {
		return nil, err
	}

	s := state.NewDocWithUniqueKey(v.id, nil, uk).(*state.State)
	rels, d, err := getDetailsForBundledObjectType(v.id)
	if err != nil {
		return nil, err
	}
	for _, r := range rels {
		s.AddRelationLinks(&model.RelationLink{Format: r.Format, Key: r.Key})
	}
	s.SetDetails(d)
	s.SetObjectType(bundle.TypeKeyObjectType.String())

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

func (s *bundledObjectType) GetCreationInfo() (creator string, createdDate int64, err error) {
	return addr.AnytypeProfileId, 0, nil
}

type bundledTypeIdGetter struct {
}

func (b *bundledTypeIdGetter) GetTypeIdByKey(_ context.Context, spaceId string, key bundle.TypeKey) (id string, err error) {
	if spaceId != addr.AnytypeMarketplaceWorkspace {
		return "", fmt.Errorf("incorrect space id: should be %s", addr.AnytypeMarketplaceWorkspace)
	}
	return key.BundledURL(), nil
}
