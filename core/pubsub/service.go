package pubsub

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/anyproto/any-sync/app"
	anysyncpubsub "github.com/anyproto/any-sync/commonspace/pubsub"
	"github.com/anyproto/any-sync/commonspace/pubsub/pubsubproto"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/rpc/server"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const (
	CName                   = "client.pubsub"
	maxPatternsPerSpace     = 100
	maxEncryptedPayloadSize = 64 * 1024
	// MaxPayloadSize leaves room for the Space read key's AES-GCM nonce and tag.
	MaxPayloadSize = maxEncryptedPayloadSize - 28
)

var log = logging.Logger(CName)

var (
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrEmptyTopics          = errors.New("no topics given")
)

// Service exposes the any-sync ephemeral pub/sub engine to middleware clients.
// It keeps a subId-keyed registry of client subscriptions on top of the engine
// (which refcounts patterns itself) and emits received messages as
// Event.Pubsub.Message on the event bus. It also implements the engine's
// client-side deps: payloads are encrypted with the space's current ACL read
// key (the same key encrypting object changes), inbound LAN subscribers are
// gated on space membership, and publishes go to the responsible node plus
// LAN-discovered peers.
type Service interface {
	app.ComponentRunnable
	Publish(ctx context.Context, spaceId, topic string, payload []byte) error
	Subscribe(ctx context.Context, spaceId string, topics []string, subId string) (string, error)
	Unsubscribe(subId string) error
	// CloseSpace drops all subscriptions of the space; called on space close.
	CloseSpace(spaceId string)
	// EngineDeps wires this component as the engine's client-side dependencies;
	// used only at bootstrap.
	EngineDeps() anysyncpubsub.Deps
}

func New() Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &service{
		ctx:      ctx,
		cancel:   cancel,
		subs:     make(map[string]*subscription),
		patterns: make(map[string]map[string]*patternEntry),
	}
}

type service struct {
	ctx         context.Context
	cancel      context.CancelFunc
	engine      anysyncpubsub.Service
	spaces      space.Service
	spaceCore   spacecore.SpaceCoreService
	peerStore   peerstore.PeerStore
	pool        pool.Pool
	eventSender event.Sender

	mu       sync.Mutex
	subs     map[string]*subscription            // subId -> subscription
	patterns map[string]map[string]*patternEntry // spaceId -> pattern -> entry
}

type subscription struct {
	spaceId  string
	patterns []string
}

// patternEntry is one engine subscription shared by every subId subscribed to
// the same pattern; a received message is emitted once per pattern with the
// current subIds.
type patternEntry struct {
	subIds map[string]struct{}
	unsub  func()
}

// EngineDeps returns the engine's client-side dependencies backed by this
// component. Field access happens at call time, after Init resolved them.
func (s *service) EngineDeps() anysyncpubsub.Deps {
	return anysyncpubsub.Deps{
		Config:     anysyncpubsub.Config{MaxPatternsPerSpace: maxPatternsPerSpace, MaxPayloadSize: maxEncryptedPayloadSize},
		Membership: s,
		Crypto:     s,
		Peers:      s,
		OnStatus:   s.onStatus,
	}
}

func (s *service) Init(a *app.App) error {
	s.engine = app.MustComponent[anysyncpubsub.Service](a)
	s.spaces = app.MustComponent[space.Service](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.peerStore = app.MustComponent[peerstore.PeerStore](a)
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.eventSender = app.MustComponent[event.Sender](a)
	// serve inbound pubsub streams from LAN peers on the shared DRPC server
	if err := anysyncpubsub.RegisterRpc(a.MustComponent(server.CName).(server.DRPCServer), s.engine); err != nil {
		return fmt.Errorf("register pubsub rpc: %w", err)
	}
	return nil
}

func (s *service) Name() string { return CName }

func (s *service) Run(context.Context) error { return nil }

func (s *service) Close(context.Context) error {
	s.cancel()
	return nil
}

func (s *service) Publish(ctx context.Context, spaceId, topic string, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	// The engine queues delivery and may open a persistent stream after this
	// RPC returns. Its context must survive the end of the unary request.
	if err := s.engine.Publish(s.ctx, spaceId, topic, payload); err != nil {
		return fmt.Errorf("publish to topic %s: %w", topic, err)
	}
	return nil
}

// Subscribe registers subId for the given patterns; an existing subId's
// pattern set is replaced. Returns the (possibly generated) subId.
func (s *service) Subscribe(ctx context.Context, spaceId string, topics []string, subId string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.ctx.Err(); err != nil {
		return "", err
	}
	if len(topics) == 0 {
		return "", fmt.Errorf("subscribe: %w", ErrEmptyTopics)
	}
	for _, t := range topics {
		if err := anysyncpubsub.ValidatePattern(t); err != nil {
			return "", fmt.Errorf("validate pattern %s: %w", t, err)
		}
	}
	// Only a local request may load a Space. Inbound membership checks keep
	// using Pick, so a remote peer cannot trigger arbitrary Space loads.
	loadCtx, cancelLoad := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancelLoad)
	_, loadErr := s.spaces.Get(loadCtx, spaceId)
	stop()
	cancelLoad()
	if loadErr != nil {
		return "", fmt.Errorf("load subscribed space: %w", loadErr)
	}
	if subId == "" {
		subId = bson.NewObjectId().Hex()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.ctx.Err(); err != nil {
		return "", err
	}
	prev := s.subs[subId]
	// Validate the final distinct pattern count before withdrawing anything.
	// A failed replacement must leave the previous subscription intact, while
	// a replacement of the same size must still work at the engine's limit.
	finalPatterns := make(map[string]struct{}, len(s.patterns[spaceId])+len(topics))
	for pattern, entry := range s.patterns[spaceId] {
		_, belongsToSub := entry.subIds[subId]
		if !belongsToSub || len(entry.subIds) > 1 {
			finalPatterns[pattern] = struct{}{}
		}
	}
	for _, pattern := range topics {
		finalPatterns[pattern] = struct{}{}
	}
	if len(finalPatterns) > maxPatternsPerSpace {
		return "", fmt.Errorf("subscribe: %w", pubsubproto.ErrTooManyTopics)
	}
	if prev != nil {
		s.removeLocked(subId, prev)
	}
	sub := &subscription{spaceId: spaceId, patterns: slices.Clone(topics)}
	for _, pattern := range topics {
		if err := s.addPatternLocked(spaceId, pattern, subId); err != nil {
			// roll back the patterns added so far
			s.removeLocked(subId, sub)
			if prev != nil {
				for _, oldPattern := range prev.patterns {
					if restoreErr := s.addPatternLocked(prev.spaceId, oldPattern, subId); restoreErr != nil {
						err = errors.Join(err, fmt.Errorf("restore pattern %s: %w", oldPattern, restoreErr))
					}
				}
				s.subs[subId] = prev
			}
			return "", fmt.Errorf("subscribe pattern %s: %w", pattern, err)
		}
	}
	s.subs[subId] = sub
	return subId, nil
}

func (s *service) addPatternLocked(spaceId, pattern, subId string) error {
	byPattern, ok := s.patterns[spaceId]
	if !ok {
		byPattern = make(map[string]*patternEntry)
		s.patterns[spaceId] = byPattern
	}
	entry, ok := byPattern[pattern]
	if !ok {
		unsub, err := s.engine.Subscribe(spaceId, pattern, s.makeHandler(pattern))
		if err != nil {
			if len(byPattern) == 0 {
				delete(s.patterns, spaceId)
			}
			return err
		}
		entry = &patternEntry{subIds: make(map[string]struct{}), unsub: unsub}
		byPattern[pattern] = entry
	}
	entry.subIds[subId] = struct{}{}
	return nil
}

func (s *service) Unsubscribe(subId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[subId]
	if !ok {
		return fmt.Errorf("unsubscribe %s: %w", subId, ErrSubscriptionNotFound)
	}
	s.removeLocked(subId, sub)
	return nil
}

// removeLocked withdraws subId from every pattern of sub, tearing down engine
// subscriptions that lost their last subscriber. Caller holds s.mu.
func (s *service) removeLocked(subId string, sub *subscription) {
	delete(s.subs, subId)
	byPattern := s.patterns[sub.spaceId]
	for _, pattern := range sub.patterns {
		entry, ok := byPattern[pattern]
		if !ok {
			continue
		}
		delete(entry.subIds, subId)
		if len(entry.subIds) == 0 {
			entry.unsub()
			delete(byPattern, pattern)
		}
	}
	if len(byPattern) == 0 {
		delete(s.patterns, sub.spaceId)
	}
}

func (s *service) CloseSpace(spaceId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for subId, sub := range s.subs {
		if sub.spaceId == spaceId {
			delete(s.subs, subId)
		}
	}
	byPattern := s.patterns[spaceId]
	delete(s.patterns, spaceId)
	for _, entry := range byPattern {
		entry.unsub()
	}
	s.engine.CloseSpace(spaceId)
}

// makeHandler returns the engine handler for one pattern; it snapshots the
// pattern's current subIds and broadcasts the message as an event. Handlers
// run on the engine's bounded dispatch queue and must not block.
func (s *service) makeHandler(pattern string) anysyncpubsub.Handler {
	return func(spaceId, topic string, identity crypto.PubKey, payload []byte) {
		s.mu.Lock()
		var subIds []string
		if entry, ok := s.patterns[spaceId][pattern]; ok {
			subIds = make([]string, 0, len(entry.subIds))
			for subId := range entry.subIds {
				subIds = append(subIds, subId)
			}
		}
		s.mu.Unlock()
		if len(subIds) == 0 {
			return
		}
		s.eventSender.Broadcast(event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfPubsubMessage{
			PubsubMessage: &pb.EventPubsubMessage{
				Topic:    topic,
				Payload:  payload,
				Identity: identity.Account(),
				SubIds:   subIds,
			},
		}))
	}
}

func (s *service) onStatus(peerId string, status *pubsubproto.Status) {
	log.With("peerId", peerId, "spaceId", status.SpaceId, "topics", status.Topics, "code", status.Code.String()).
		Info("pubsub rejection from serving peer")
}
