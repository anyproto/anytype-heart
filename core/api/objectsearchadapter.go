package api

// objectsearchadapter.go is the live object search behind apicore's
// OpenObjectSearch: the backend of the API v2 search stream.
//
// One internal subscription per search, unlimited (the opening snapshot is
// the whole matching set, and a sorted limit would turn into a live window),
// with no dependency subscription and a caller-owned unbounded queue: the
// engine delivers on the space worker, and a full bounded queue would stall
// every subscription of the space. A worker translates the engine's detail
// and membership events into added, updated and removed changes and appends
// them to the search's own unbounded queue, so a slow reader only costs
// memory and is never disconnected.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/cheggaaa/mb/v3"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// objectSearchSubIdPrefix starts every subscription id the adapter mints.
const objectSearchSubIdPrefix = "api-object-search-"

// objectSearchBaseKeys are carried by every search besides the requested
// keys: what the base row renders, and the discussion reference, so a change
// to it is reported as an update.
var objectSearchBaseKeys = []string{
	bundle.RelationKeyId.String(),
	bundle.RelationKeyName.String(),
	bundle.RelationKeyType.String(),
	bundle.RelationKeyDiscussionId.String(),
}

// objectSearcher is the subscription engine, narrowed to what the adapter calls.
type objectSearcher interface {
	Search(req subscription.SubscribeRequest) (*subscription.SubscribeResponse, error)
	Unsubscribe(subIds ...string) error
}

type objectSearchAdapter struct {
	searcher objectSearcher
	seq      atomic.Uint64
}

func newObjectSearchAdapter(searcher objectSearcher) *objectSearchAdapter {
	return &objectSearchAdapter{searcher: searcher}
}

// OpenObjectSearch subscribes and returns the search with its opening
// snapshot. The engine builds the snapshot and starts queueing under one
// lock, so a change racing the open lands in the snapshot or in the queue.
func (a *objectSearchAdapter) OpenObjectSearch(_ context.Context, req apicore.ObjectSearchOpen) (apicore.ObjectSearchSubscription, error) {
	if req.SpaceId == "" {
		return nil, errors.New("open object search: empty space id")
	}
	queue := mb.New[*pb.EventMessage](0)
	subId := fmt.Sprintf("%s%d", objectSearchSubIdPrefix, a.seq.Add(1))
	keys := slices.Clone(objectSearchBaseKeys)
	for _, key := range req.Keys {
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	resp, err := a.searcher.Search(subscription.SubscribeRequest{
		SpaceId:           req.SpaceId,
		SubId:             subId,
		Filters:           req.Filters,
		Sorts:             req.Sorts,
		Limit:             0,
		Keys:              keys,
		NoDepSubscription: true,
		Internal:          true,
		InternalQueue:     queue,
	})
	if err != nil {
		_ = queue.Close()
		return nil, fmt.Errorf("subscribe to object search: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &objectSearch{
		searcher: a.searcher,
		subId:    subId,
		queue:    queue,
		out:      newEventQueue[apicore.ObjectSearchChange](),
		members:  make(map[string]*domain.Details, len(resp.Records)),
		cancel:   cancel,
	}
	s.snapshot = make([]apicore.ObjectSearchChange, 0, len(resp.Records)+1)
	for _, details := range resp.Records {
		id := details.GetString(bundle.RelationKeyId)
		s.members[id] = details
		s.snapshot = append(s.snapshot, apicore.ObjectSearchChange{Type: apicore.ObjectSearchAdded, Id: id, Details: details})
	}
	s.snapshot = append(s.snapshot, apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete})
	s.wg.Add(1)
	go s.work(ctx)
	return s, nil
}

// objectSearch is one open search (apicore.ObjectSearchSubscription).
type objectSearch struct {
	searcher objectSearcher
	subId    string
	// queue receives the engine's events. The search owns it: Unsubscribe
	// does not close a caller-provided queue.
	queue *mb.MB[*pb.EventMessage]
	out   *eventQueue[apicore.ObjectSearchChange]

	snapshot []apicore.ObjectSearchChange
	// members holds each member's carried details, touched by the worker
	// only once the open has returned. A change replaces an entry with a new
	// value instead of editing it, so a delivered change is never mutated.
	members map[string]*domain.Details

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

func (s *objectSearch) Snapshot() []apicore.ObjectSearchChange { return s.snapshot }

func (s *objectSearch) Ready() <-chan struct{} { return s.out.ready() }

func (s *objectSearch) Drain() []apicore.ObjectSearchChange { return s.out.drain() }

// Close unsubscribes, closes the owned queue and joins the worker, in that
// order: nothing is queued for the search once the engine dropped it.
func (s *objectSearch) Close() {
	s.closeOnce.Do(func() {
		if err := s.searcher.Unsubscribe(s.subId); err != nil {
			log.Warnf("object search %s: unsubscribe: %v", s.subId, err)
		}
		s.cancel()
		_ = s.queue.Close()
		s.wg.Wait()
		s.out.close()
	})
}

func (s *objectSearch) work(ctx context.Context) {
	defer s.wg.Done()
	for {
		msgs, err := s.queue.Wait(ctx)
		if err != nil {
			return
		}
		if changes := s.translate(msgs); len(changes) > 0 {
			s.out.push(changes...)
		}
	}
}

// translate turns one batch of engine events into changes. DetailsSet is the
// add (the engine pairs it with a SubscriptionAdd, which carries no details,
// and resends it to resync a member); Position and Counters are not changes
// of the set.
func (s *objectSearch) translate(msgs []*pb.EventMessage) []apicore.ObjectSearchChange {
	var changes []apicore.ObjectSearchChange
	for _, msg := range msgs {
		switch v := msg.Value.(type) {
		case *pb.EventMessageValueOfObjectDetailsSet:
			set := v.ObjectDetailsSet
			if !slices.Contains(set.SubIds, s.subId) {
				continue
			}
			details := domain.NewDetailsFromProto(set.Details)
			current, member := s.members[set.Id]
			s.members[set.Id] = details
			switch {
			case !member:
				changes = append(changes, apicore.ObjectSearchChange{Type: apicore.ObjectSearchAdded, Id: set.Id, Details: details})
			case !current.Equal(details):
				changes = append(changes, apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: set.Id, Details: details})
			}
		case *pb.EventMessageValueOfObjectDetailsAmend:
			amend := v.ObjectDetailsAmend
			if !slices.Contains(amend.SubIds, s.subId) {
				continue
			}
			changes = s.update(changes, amend.Id, func(next *domain.Details) {
				for _, kv := range amend.Details {
					next.Set(domain.RelationKey(kv.Key), domain.ValueFromProto(kv.Value))
				}
			})
		case *pb.EventMessageValueOfObjectDetailsUnset:
			unset := v.ObjectDetailsUnset
			if !slices.Contains(unset.SubIds, s.subId) {
				continue
			}
			changes = s.update(changes, unset.Id, func(next *domain.Details) {
				for _, key := range unset.Keys {
					next.Delete(domain.RelationKey(key))
				}
			})
		case *pb.EventMessageValueOfSubscriptionRemove:
			remove := v.SubscriptionRemove
			if remove.SubId != s.subId {
				continue
			}
			if _, member := s.members[remove.Id]; member {
				delete(s.members, remove.Id)
				changes = append(changes, apicore.ObjectSearchChange{Type: apicore.ObjectSearchRemoved, Id: remove.Id})
			}
		}
	}
	return changes
}

// update applies an edit to a copy of a member's details and reports the
// copy, when the edit changed anything.
func (s *objectSearch) update(changes []apicore.ObjectSearchChange, id string, edit func(next *domain.Details)) []apicore.ObjectSearchChange {
	current, member := s.members[id]
	if !member {
		return changes
	}
	next := current.Copy()
	edit(next)
	if next.Equal(current) {
		return changes
	}
	s.members[id] = next
	return append(changes, apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: id, Details: next})
}
