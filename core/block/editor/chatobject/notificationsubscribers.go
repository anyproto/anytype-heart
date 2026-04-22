package chatobject

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

const notificationSubscribersKey = "notificationSubscribers"

func (s *storeObject) AddNotificationSubscriber(ctx context.Context, identity string) error {
	if identity == "" {
		return fmt.Errorf("identity is required")
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	participantId := domain.NewParticipantId(s.storeSource.SpaceID(), identity)

	builder := storestate.Builder{}
	if err := builder.Modify(
		EditorCollectionName,
		DetailsDocumentId,
		[]string{notificationSubscribersKey},
		pb.ModifyOp_AddToSet,
		arena.NewString(participantId),
	); err != nil {
		return fmt.Errorf("add to set: %w", err)
	}
	if _, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	}); err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) RemoveNotificationSubscriber(ctx context.Context, identity string) error {
	if identity == "" {
		return fmt.Errorf("identity is required")
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	participantId := domain.NewParticipantId(s.storeSource.SpaceID(), identity)

	builder := storestate.Builder{}
	if err := builder.Modify(
		EditorCollectionName,
		DetailsDocumentId,
		[]string{notificationSubscribersKey},
		pb.ModifyOp_Pull,
		arena.NewString(participantId),
	); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	if _, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	}); err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) GetNotificationSubscribers(ctx context.Context) ([]string, error) {
	participantIds, err := s.readNotificationSubscribers(ctx)
	if err != nil {
		return nil, err
	}
	if len(participantIds) == 0 {
		return nil, nil
	}
	prefix := domain.NewParticipantId(s.storeSource.SpaceID(), "")
	identities := make([]string, 0, len(participantIds))
	for _, pid := range participantIds {
		if !strings.HasPrefix(pid, prefix) {
			continue
		}
		identities = append(identities, strings.TrimPrefix(pid, prefix))
	}
	return identities, nil
}

func (s *storeObject) readNotificationSubscribers(ctx context.Context) ([]string, error) {
	coll, err := s.store.Collection(ctx, EditorCollectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	doc, err := coll.FindId(ctx, DetailsDocumentId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find details: %w", err)
	}
	arr := doc.Value().GetArray(notificationSubscribersKey)
	if len(arr) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if v.Type() == anyenc.TypeString {
			out = append(out, string(v.GetStringBytes()))
		}
	}
	return out, nil
}
