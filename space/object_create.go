package space

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/util/crypto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/session"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// DeriveObject derives the object with id specified in the payload and triggers cache.Get
// DeriveTreeCreatePayload should be called first to prepare the payload and derive the tree
func (s *clientSpace) DeriveObject(
	ctx session.Context, payload *treestorage.TreeStorageCreatePayload, newAccount bool,
) (err error) {
	_, err = s.getDerivedObject(ctx.Context(), payload, newAccount, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{Ctx: ctx, State: state.NewDoc(id, nil).(*state.State)}
	})
	if err != nil {
		log.With(zap.Error(err)).Debug("derived object with error")
		return
	}
	return nil
}

func (s *clientSpace) CreateTreeObjectWithPayload(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	tr, err := s.Space.TreeBuilder().PutTree(ctx, payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	tr.Close()
	return s.cacheCreatedObject(ctx, payload.RootRawChange.Id, initFunc)
}

func (s *clientSpace) CreateTreeObject(ctx session.Context, tp coresb.SmartBlockType, initFunc InitFunc) (sb smartblock.SmartBlock, err error) {
	payload, err := s.CreateTreePayloadWithSpace(ctx.Context(), tp)
	if err != nil {
		return nil, err
	}

	tr, err := s.Space.TreeBuilder().PutTree(ctx.Context(), payload, nil)
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		err = fmt.Errorf("failed to put tree: %w", err)
		return
	}
	tr.Close()
	return s.cacheCreatedObject(ctx.Context(), payload.RootRawChange.Id, initFunc)
}

func (s *clientSpace) CreateTreePayloadWithSpace(ctx context.Context, tp coresb.SmartBlockType) (treestorage.TreeStorageCreatePayload, error) {
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx, tp, time.Now())
}

func (s *clientSpace) CreateTreePayloadWithSpaceAndCreatedTime(ctx context.Context, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	changePayload, err := createChangePayload(tp)
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	treePayload, err := createPayload(s.Id(), s.commonAccount.Account().SignKey, changePayload, createdTime.Unix())
	if err != nil {
		return treestorage.TreeStorageCreatePayload{}, err
	}
	return s.TreeBuilder().CreateTree(ctx, treePayload)
}

func (s *clientSpace) CreateTreePayload(ctx session.Context, tp coresb.SmartBlockType, createdTime time.Time) (treestorage.TreeStorageCreatePayload, error) {
	return s.CreateTreePayloadWithSpaceAndCreatedTime(ctx.Context(), tp, createdTime)
}

// DeriveTreeCreatePayload creates payload for the tree of derived object.
// Method should be called before DeriveObject to prepare payload
func (s *clientSpace) DeriveTreeCreatePayload(
	ctx session.Context, tp coresb.SmartBlockType,
) (*treestorage.TreeStorageCreatePayload, error) {
	changePayload, err := createChangePayload(tp)
	if err != nil {
		return nil, err
	}
	treePayload := derivePayload(s.Id(), s.commonAccount.Account().SignKey, changePayload)
	create, err := s.TreeBuilder().CreateTree(context.Background(), treePayload)
	return &create, err
}

func createChangePayload(sbType coresb.SmartBlockType) (data []byte, err error) {
	payload := &model.ObjectChangePayload{SmartBlockType: model.SmartBlockType(sbType)}
	return payload.Marshal()
}

func derivePayload(spaceId string, signKey crypto.PrivKey, changePayload []byte) objecttree.ObjectTreeCreatePayload {
	return objecttree.ObjectTreeCreatePayload{
		PrivKey:       signKey,
		ChangeType:    ChangeType,
		ChangePayload: changePayload,
		SpaceId:       spaceId,
		IsEncrypted:   true,
	}
}

func createPayload(spaceId string, signKey crypto.PrivKey, changePayload []byte, timestamp int64) (objecttree.ObjectTreeCreatePayload, error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return objecttree.ObjectTreeCreatePayload{}, err
	}
	return objecttree.ObjectTreeCreatePayload{
		PrivKey:       signKey,
		ChangeType:    ChangeType,
		ChangePayload: changePayload,
		SpaceId:       spaceId,
		IsEncrypted:   true,
		Timestamp:     timestamp,
		Seed:          seed,
	}, nil
}
