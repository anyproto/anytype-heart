package spaceindex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/anyenc/anyencutil"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *dsObjectStore) UpdateObjectDetails(ctx context.Context, id string, details *domain.Details) error {
	if details == nil || details.Len() == 0 {
		return fmt.Errorf("empty details")
	}
	// Ensure ID is set
	details.SetString(bundle.RelationKeyId, id)

	// Only id is set
	if details.Len() == 1 {
		return fmt.Errorf("should be more than just id")
	}

	arena := s.arenaPool.Get()
	defer func() {
		s.arenaPool.Put(arena)
	}()
	newVal := details.ToAnyEnc(arena)
	var isModified bool
	_, err := s.objects.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		if anyencutil.Equal(val, newVal) {
			return nil, false, nil
		}
		isModified = true
		return newVal, true, nil
	}))
	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}
	if isModified {
		s.sendUpdatesToSubscriptions(ctx, id, details)
	}

	return nil
}

func (s *dsObjectStore) SubscribeForAll(callback func(rec database.Record)) {
	s.lock.Lock()
	s.onChangeCallback = callback
	s.lock.Unlock()
}

// txNotifications buffers subscription notifications raised inside a write
// transaction. They are flushed only on successful commit (see notifyingTx):
// firing them earlier would announce writes that are not yet visible to
// readers — or never will be, if the tx rolls back. A subscriber that wires
// its callback and then re-queries the store could otherwise permanently miss
// a write that was notified before wiring but committed after the re-query.
type txNotifications struct {
	mu      sync.Mutex
	pending []database.Record
}

type txNotificationsKey struct{}

func (s *dsObjectStore) sendUpdatesToSubscriptions(ctx context.Context, id string, details *domain.Details) {
	if buf, ok := ctx.Value(txNotificationsKey{}).(*txNotifications); ok && buf != nil {
		detCopy := details.Copy()
		detCopy.SetString(bundle.RelationKeyId, id)
		buf.mu.Lock()
		buf.pending = append(buf.pending, database.Record{Details: detCopy})
		buf.mu.Unlock()
		return
	}
	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.onChangeCallback == nil && len(s.subscriptions) == 0 {
		return
	}
	detCopy := details.Copy()
	detCopy.SetString(bundle.RelationKeyId, id)
	if s.onChangeCallback != nil {
		s.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for _, sub := range s.subscriptions {
		_ = sub.PublishAsync(id, detCopy)
	}
}

func (s *dsObjectStore) flushTxNotifications(buf *txNotifications) {
	buf.mu.Lock()
	pending := buf.pending
	buf.pending = nil
	buf.mu.Unlock()

	s.lock.RLock()
	defer s.lock.RUnlock()
	for _, rec := range pending {
		id := rec.Details.GetString(bundle.RelationKeyId)
		if s.onChangeCallback != nil {
			s.onChangeCallback(rec)
		}
		for _, sub := range s.subscriptions {
			_ = sub.PublishAsync(id, rec.Details)
		}
	}
}

// unsafe, use under mutex
func (s *dsObjectStore) addSubscriptionIfNotExists(sub database.Subscription) (existed bool) {
	for _, s := range s.subscriptions {
		if s == sub {
			return true
		}
	}

	s.subscriptions = append(s.subscriptions, sub)
	return false
}

func (s *dsObjectStore) closeAndRemoveSubscription(subscription database.Subscription) {
	s.lock.Lock()
	defer s.lock.Unlock()
	subscription.Close()

	for i, sub := range s.subscriptions {
		if sub == subscription {
			s.subscriptions = append(s.subscriptions[:i], s.subscriptions[i+1:]...)
			break
		}
	}
}

func (s *dsObjectStore) UpdateObjectLinks(ctx context.Context, id string, links []string) error {
	added, removed, err := s.updateObjectLinks(ctx, id, links)
	if err != nil {
		return err
	}

	s.subManager.updateObjectLinks(domain.FullID{SpaceID: s.SpaceId(), ObjectID: id}, added, removed)

	return nil
}
func (s *dsObjectStore) UpdateObjectLinksDetailed(ctx context.Context, id string, outgoingLinks []OutgoingLink) error {
	added, removed, err := s.updateObjectLinksDetailed(ctx, id, outgoingLinks)
	if err != nil {
		return err
	}

	s.subManager.updateObjectLinks(domain.FullID{SpaceID: s.SpaceId(), ObjectID: id}, added, removed)

	return nil
}

func (s *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (newDetails *domain.Details, err error)) error {
	if proc == nil {
		return nil
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	txn, err := s.pendingDetails.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("write txn: %w", err)
	}

	defer func() {
		_ = txn.Rollback()
	}()

	var shouldDelete bool
	res, err := s.pendingDetails.UpsertId(txn.Context(), id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		currentDetails, err := domain.NewDetailsFromAnyEnc(val)
		if err != nil {
			return nil, false, fmt.Errorf("get old details: json to proto: %w", err)
		}

		newDetails, err := proc(currentDetails)
		if err != nil {
			return nil, false, fmt.Errorf("run a modifier: %w", err)
		}
		if newDetails == nil {
			shouldDelete = true
			return val, false, nil
		}
		newDetails.SetString(bundle.RelationKeyId, id)

		newVal := newDetails.ToAnyEnc(arena)
		if anyencutil.Equal(val, newVal) {
			return val, false, nil
		}
		return newVal, true, nil
	}))

	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}
	if res.Matched > 0 && shouldDelete {
		err = s.pendingDetails.DeleteId(txn.Context(), id)
		if err != nil {
			return fmt.Errorf("delete pending details: %w", err)
		}
	}
	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("commit txn: %w", err)
	}

	return nil
}

// ModifyObjectDetails updates details in store using modification function `proc`.
// When upsert is true, the object is created if it does not exist; when false, missing objects are silently skipped.
func (s *dsObjectStore) ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error), upsert bool) error {
	return s.ModifyObjectDetailsCtx(s.componentCtx, id, proc, upsert)
}

// ModifyObjectDetailsCtx is like ModifyObjectDetails but runs under the
// provided context instead of the store component context. Pass a tx-bearing
// context (see WriteTx) to batch many modifications into a single write tx and,
// when the context is non-cancelable, to skip any-store's per-statement
// SQLite interrupt handshake.
func (s *dsObjectStore) ModifyObjectDetailsCtx(ctx context.Context, id string, proc func(details *domain.Details) (*domain.Details, bool, error), upsert bool) error {
	if proc == nil {
		return nil
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	// notifyDetails is captured by the modifier and sent only after the write
	// op succeeds: notifying from inside ModifyFunc would announce a write
	// that may still fail or, under a write tx, not be committed yet
	var notifyDetails *domain.Details
	modifier := query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		notifyDetails = nil
		inputDetails, err := domain.NewDetailsFromAnyEnc(val)
		if err != nil {
			return nil, false, fmt.Errorf("get old details: json to proto: %w", err)
		}
		newDetails, modified, err := proc(inputDetails)
		if err != nil {
			return nil, false, fmt.Errorf("run a modifier: %w", err)
		}
		if !modified {
			return nil, false, nil
		}
		if newDetails == nil {
			newDetails = domain.NewDetails()
		}
		// Ensure ID is set
		newDetails.SetString(bundle.RelationKeyId, id)

		jsonVal := newDetails.ToAnyEnc(arena)
		diff, err := pbtypes.DiffAnyEnc(val, jsonVal)
		if err != nil {
			return nil, false, fmt.Errorf("diff json: %w", err)
		}
		if len(diff) == 0 {
			return nil, false, nil
		}
		notifyDetails = newDetails
		return jsonVal, true, nil
	})
	var err error
	if upsert {
		_, err = s.objects.UpsertId(ctx, id, modifier)
	} else {
		_, err = s.objects.UpdateId(ctx, id, modifier)
		if errors.Is(err, anystore.ErrDocNotFound) {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("modify details: %w", err)
	}
	if notifyDetails != nil {
		s.sendUpdatesToSubscriptions(ctx, id, notifyDetails)
	}
	return nil
}

func (s *dsObjectStore) updateObjectLinks(ctx context.Context, id string, links []string) (added []string, removed []string, err error) {
	_, err = s.links.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		prev := anyEncArrayToStrings(val.GetArray(linkOutboundField))
		removed, added = slice.DifferenceRemovedAdded(prev, links)
		if len(added) == 0 && len(removed) == 0 {
			return val, false, nil
		}
		val.Set(linkOutboundField, stringsToJsonArray(arena, links))
		return val, len(added)+len(removed) > 0, nil
	}))
	return
}

func stringsToJsonArray(arena *anyenc.Arena, arr []string) *anyenc.Value {
	res := arena.NewArray()
	for i, v := range arr {
		res.SetArrayItem(i, arena.NewString(v))
	}
	return res
}

func anyEncArrayToStrings(arr []*anyenc.Value) []string {
	res := make([]string, 0, len(arr))
	for _, v := range arr {
		res = append(res, string(v.GetStringBytes()))
	}
	return res
}

func (s *dsObjectStore) updateObjectLinksDetailed(ctx context.Context, id string, outgoingLinks []OutgoingLink) (added []string, removed []string, err error) {
	_, err = s.links.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		// Get previous simple links for diff calculation
		prev := anyEncArrayToStrings(val.GetArray(linkOutboundField))

		// Create target ID list for diff (deduplicated for backward compatibility with simple links)
		current := make([]string, 0, len(outgoingLinks))
		seen := make(map[string]struct{})
		for _, link := range outgoingLinks {
			if _, ok := seen[link.TargetID]; !ok {
				current = append(current, link.TargetID)
				seen[link.TargetID] = struct{}{}
			}
		}

		removed, added = slice.DifferenceRemovedAdded(prev, current)
		detailedChanged := len(added)+len(removed) == 0 && isDetailedLinksChanged(val.GetArray(linkDetailedField), outgoingLinks)

		// Store simple links for backward compatibility
		val.Set(linkOutboundField, stringsToJsonArray(arena, current))

		// Store detailed link information
		detailedLinks := arena.NewArray()
		for i, link := range outgoingLinks {
			linkObj := arena.NewObject()
			linkObj.Set(linkTargetField, arena.NewString(link.TargetID))
			if link.BlockID != "" {
				linkObj.Set(linkBlockField, arena.NewString(link.BlockID))
			}
			if link.RelationKey != "" {
				linkObj.Set(linkRelationField, arena.NewString(link.RelationKey))
			}
			detailedLinks.SetArrayItem(i, linkObj)
		}
		val.Set(linkDetailedField, detailedLinks)

		return val, len(added)+len(removed) > 0 || detailedChanged, nil
	}))
	return
}

func isDetailedLinksChanged(prevArr []*anyenc.Value, current []OutgoingLink) bool {
	if len(prevArr) != len(current) {
		return true
	}
	for i, link := range current {
		prev := prevArr[i]
		if !bytes.Equal(prev.GetStringBytes(linkTargetField), []byte(link.TargetID)) {
			return true
		}
		if !bytes.Equal(prev.GetStringBytes(linkBlockField), []byte(link.BlockID)) {
			return true
		}
		if !bytes.Equal(prev.GetStringBytes(linkRelationField), []byte(link.RelationKey)) {
			return true
		}
	}
	return false
}
