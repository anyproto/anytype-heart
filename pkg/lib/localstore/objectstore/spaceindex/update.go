package spaceindex

import (
	"context"
	"errors"
	"fmt"

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
	if isModified {
		s.sendUpdatesToSubscriptions(id, details)
	}

	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}

	return nil
}

func (s *dsObjectStore) SubscribeForAll(callback func(rec database.Record)) {
	s.lock.Lock()
	s.onChangeCallback = callback
	s.lock.Unlock()
}

func (s *dsObjectStore) sendUpdatesToSubscriptions(id string, details *domain.Details) {
	detCopy := details.Copy()
	detCopy.SetString(bundle.RelationKeyId, id)
	s.lock.RLock()
	defer s.lock.RUnlock()
	if s.onChangeCallback != nil {
		s.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for _, sub := range s.subscriptions {
		_ = sub.PublishAsync(id, detCopy)
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
	// Extract simple links for compatibility
	links := make([]string, 0, len(outgoingLinks))
	linkMap := make(map[string][]OutgoingLink)
	for _, link := range outgoingLinks {
		links = append(links, link.TargetID)
		linkMap[link.TargetID] = append(linkMap[link.TargetID], link)
	}

	added, removed, err := s.updateObjectLinksDetailed(ctx, id, outgoingLinks)
	if err != nil {
		return err
	}

	s.subManager.updateObjectLinks(domain.FullID{SpaceID: s.SpaceId(), ObjectID: id}, added, removed)

	return nil
}

func (s *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *domain.Details) (*domain.Details, error)) error {
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

	var inputDetails *domain.Details
	doc, err := s.pendingDetails.FindId(txn.Context(), id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		inputDetails = domain.NewDetails()
	} else if err != nil {
		return fmt.Errorf("find details: %w", err)
	} else {
		inputDetails, err = domain.NewDetailsFromAnyEnc(doc.Value())
		if err != nil {
			return fmt.Errorf("json to proto: %w", err)
		}
	}

	newDetails, err := proc(inputDetails)
	if err != nil {
		return fmt.Errorf("run a modifier: %w", err)
	}
	if newDetails == nil {
		err = s.pendingDetails.DeleteId(txn.Context(), id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return fmt.Errorf("delete details: %w", err)
		}
		return txn.Commit()
	}
	newDetails.SetString(bundle.RelationKeyId, id)
	jsonVal := newDetails.ToAnyEnc(arena)
	err = s.pendingDetails.UpsertOne(txn.Context(), jsonVal)
	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("commit txn: %w", err)
	}

	return nil
}

// ModifyObjectDetails updates existing details in store using modification function `proc`
// `proc` should return ErrDetailsNotChanged in case old details are empty or no changes were made
func (s *dsObjectStore) ModifyObjectDetails(id string, proc func(details *domain.Details) (*domain.Details, bool, error)) error {
	if proc == nil {
		return nil
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	_, err := s.objects.UpsertId(s.componentCtx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
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
		s.sendUpdatesToSubscriptions(id, newDetails)
		return jsonVal, true, nil
	}))

	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}
	return nil
}

func (s *dsObjectStore) updateObjectLinks(ctx context.Context, id string, links []string) (added []string, removed []string, err error) {
	_, err = s.links.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		prev := anyEncArrayToStrings(val.GetArray(linkOutboundField))
		removed, added = slice.DifferenceRemovedAdded(prev, links)
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

		// Create target ID list for diff
		current := make([]string, 0, len(outgoingLinks))
		for _, link := range outgoingLinks {
			current = append(current, link.TargetID)
		}

		removed, added = slice.DifferenceRemovedAdded(prev, current)

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

		return val, len(added)+len(removed) > 0, nil
	}))
	return
}
