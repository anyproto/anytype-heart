package acl

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"
)

type participantGetter interface {
	Run(ctx context.Context)
	Close() error
}

type participantRemover interface {
	ApproveLeave(ctx context.Context, spaceId string, identities []crypto.PubKey) error
}

type processLoop[T any] struct {
	batcher    *mb.MB[string]
	updateFunc func(ctx context.Context, msg T) error
	evaluate   func(err error) bool
	updates    map[string]T
	mx         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
}

func newProcessLoop[T any](
	updateFunc func(ctx context.Context, msg T) error,
	evaluate func(err error) bool,
) *processLoop[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &processLoop[T]{
		batcher:    mb.New[string](0),
		updateFunc: updateFunc,
		evaluate:   evaluate,
		updates:    make(map[string]T),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (pl *processLoop[T]) AddUpdate(id string, value T) error {
	pl.mx.Lock()
	if _, ok := pl.updates[id]; ok {
		pl.updates[id] = value
		pl.mx.Unlock()
		return nil
	}

	pl.updates[id] = value
	pl.mx.Unlock()

	err := pl.batcher.TryAdd(id)
	if err != nil {
		pl.mx.Lock()
		delete(pl.updates, id)
		pl.mx.Unlock()
	}
	return err
}

func (pl *processLoop[T]) RemoveUpdate(id string) {
	pl.mx.Lock()
	delete(pl.updates, id)
	pl.mx.Unlock()
}

func (pl *processLoop[T]) Run() {
	go pl.process()
}

func (pl *processLoop[T]) process() {
	for {
		id, err := pl.batcher.WaitOne(pl.ctx)
		if err != nil {
			return
		}

		pl.mx.Lock()
		msg, exists := pl.updates[id]
		pl.mx.Unlock()

		if !exists {
			continue
		}

		if err := pl.updateFunc(pl.ctx, msg); err != nil {
			if pl.evaluate(err) {
				time.Sleep(time.Millisecond * 100)
				pl.batcher.Add(pl.ctx, id)
			}
		}
	}
}

func (pl *processLoop[T]) Close() error {
	pl.cancel()
	return pl.batcher.Close()
}

type participantManager[T any] struct {
	processLoop *processLoop[T]
	onRemove    func(identity crypto.PubKey, spaceId string)
	onAdd       func(identity crypto.PubKey, spaceId string)
	ctx         context.Context
	cancel      context.CancelFunc
}

type Message struct {
	Id       string
	Identity crypto.PubKey
}

type participantGetterFunc = func(
	processLoop *processLoop[Message],
	onRemove func(identity crypto.PubKey, spaceId string),
	onAdd func(identity crypto.PubKey, spaceId string),
) participantGetter

type aclWatcher struct {
	loop              *processLoop[Message]
	participantGetter participantGetter
	remover           participantRemover
}

func newParticipantService(getter participantGetterFunc, remover participantRemover) *aclWatcher {
	loop := newProcessLoop[Message](
		func(ctx context.Context, msg Message) error {
			return remover.ApproveLeave(ctx, msg.Id, []crypto.PubKey{msg.Identity})
		},
		func(err error) bool {
			return !errors.Is(err, ErrRequestNotExists)
		},
	)

	participantGetter := getter(
		loop,
		func(identity crypto.PubKey, spaceId string) {
			loop.RemoveUpdate(spaceId)
		},
		func(identity crypto.PubKey, spaceId string) {
			err := loop.AddUpdate(spaceId, Message{
				Id:       spaceId,
				Identity: identity,
			})
			if err != nil {
				log.Debug("failed to add update", zap.String("spaceId", spaceId), zap.Error(err))
			}
		},
	)

	return &aclWatcher{
		loop:              loop,
		participantGetter: participantGetter,
	}
}

func (ps *aclWatcher) Run(ctx context.Context) {
	ps.loop.Run()
	ps.participantGetter.Run(ctx)
}

func (ps *aclWatcher) Close() error {
	if err := ps.participantGetter.Close(); err != nil {
		return err
	}
	return ps.loop.Close()
}
