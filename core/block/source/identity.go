package source

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type identityService interface {
	SpaceId() string
	GetDetails(ctx context.Context, identity string) (details *types.Struct, err error)
}

func (s *service) NewIdentity(id string) Source {
	ctx, cancel := context.WithCancel(context.Background())
	return &identity{
		identityService: s.identityService,
		id:              id,
		closingCtx:      ctx,
		closingCtxFunc:  cancel,
	}
}

type identity struct {
	identityService identityService
	closingCtx      context.Context
	closingCtxFunc  context.CancelFunc
	id              string
}

func (v *identity) ListIds() ([]string, error) {
	// todo: later
	return []string{}, nil
}

func (v *identity) ReadOnly() bool {
	return true
}

func (v *identity) Id() string {
	return v.id
}

func (v *identity) SpaceID() string {
	return v.identityService.SpaceId()
}

func (v *identity) Type() smartblock.SmartBlockType {
	return smartblock.SmartBlockTypeIdentity
}

func (v *identity) detailsToState(details *types.Struct) (doc state.Doc) {
	t := state.NewDoc(v.id, nil).(*state.State)
	t.SetObjectTypeKey(bundle.TypeKeyProfile)
	t.SetDetails(details)

	return t
}

func (v *identity) ReadDoc(ctx context.Context, receiver ChangeReceiver, empty bool) (doc state.Doc, err error) {
	details, err := v.identityService.GetDetails(ctx, v.id)
	if err != nil {
		return nil, err
	}

	return v.detailsToState(details), nil
}

func (v *identity) ReadMeta(ctx context.Context, r ChangeReceiver) (doc state.Doc, err error) {
	return v.ReadDoc(ctx, r, false)
}

func (v *identity) Close() (err error) {
	v.closingCtxFunc()
	return
}

func (v *identity) Heads() []string {
	return []string{"todo"}
}

func (s *identity) GetFileKeysSnapshot() []*pb.ChangeFileKeys {
	return nil
}

func (s *identity) PushChange(params PushChangeParams) (id string, err error) {
	return
}

func (s *identity) GetCreationInfo() (creatorObjectId string, createdDate int64, err error) {
	return s.id, 0, nil
}
