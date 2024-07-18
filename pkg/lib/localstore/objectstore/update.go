package objectstore

import (
	"context"
	"fmt"

	"github.com/anyproto/any-store/query"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *dsObjectStore) UpdateObjectDetails(ctx context.Context, id string, details *types.Struct) error {
	if details == nil {
		return nil
	}
	if details.Fields == nil {
		return fmt.Errorf("details fields are nil")
	}
	if len(details.Fields) == 0 {
		return fmt.Errorf("empty details")
	}
	// Ensure ID is set
	details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)

	arena := s.arenaPool.Get()
	jsonVal := pbtypes.ProtoToJson(arena, details)
	var isModified bool
	_, err := s.objects.UpsertId(ctx, id, query.ModifyFunc(func(arena *fastjson.Arena, val *fastjson.Value) (*fastjson.Value, bool, error) {
		diff, err := pbtypes.DiffJson(val, jsonVal)
		if err != nil {
			return nil, false, fmt.Errorf("diff json: %w", err)
		}
		if len(diff) == 0 {
			return nil, false, nil
		}
		isModified = true
		return jsonVal, true, nil
	}))
	if isModified {
		s.sendUpdatesToSubscriptions(id, details)
	}
	arena.Reset()
	s.arenaPool.Put(arena)
	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}
	return nil
}

func (s *dsObjectStore) UpdateObjectLinks(id string, links []string) error {
	added, removed, err := s.updateObjectLinks(s.componentCtx, id, links)
	if err != nil {
		return err
	}
	s.RLock()
	defer s.RUnlock()
	if s.onLinksUpdateCallback != nil && len(added)+len(removed) > 0 {
		s.onLinksUpdateCallback(LinksUpdateInfo{
			LinksFromId: id,
			Added:       added,
			Removed:     removed,
		})
	}
	return nil
}

func (s *dsObjectStore) UpdateObjectSnippet(id string, snippet string) error {
	return badgerhelper.SetValue(s.db, pagesSnippetBase.ChildString(id).Bytes(), snippet)
}

func (s *dsObjectStore) UpdatePendingLocalDetails(id string, proc func(details *types.Struct) (*types.Struct, error)) error {
	return s.updateTxn(func(txn *badger.Txn) error {
		key := pendingDetailsBase.ChildString(id).Bytes()

		objDetails, err := s.getPendingLocalDetails(txn, key)
		if err != nil && !badgerhelper.IsNotFound(err) {
			return fmt.Errorf("get pending details: %w", err)
		}

		oldDetails := objDetails.GetDetails()
		if oldDetails == nil || oldDetails.Fields == nil {
			oldDetails = &types.Struct{Fields: map[string]*types.Value{}}
		}
		newDetails, err := proc(oldDetails)
		if err != nil {
			return fmt.Errorf("run a modifier: %w", err)
		}
		if newDetails == nil {
			err = txn.Delete(key)
			if err != nil {
				return err
			}
			return nil
		}

		if newDetails.Fields == nil {
			newDetails.Fields = map[string]*types.Value{}
		}
		newDetails.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
		err = badgerhelper.SetValueTxn(txn, key, &model.ObjectDetails{Details: newDetails})
		if err != nil {
			return fmt.Errorf("put pending details: %w", err)
		}
		return nil
	})
}

// ModifyObjectDetails updates existing details in store using modification function `proc`
// `proc` should return ErrDetailsNotChanged in case old details are empty or no changes were made
func (s *dsObjectStore) ModifyObjectDetails(id string, proc func(details *types.Struct) (*types.Struct, bool, error)) error {
	if proc == nil {
		return nil
	}
	arena := s.arenaPool.Get()
	_, err := s.objects.UpsertId(s.componentCtx, id, query.ModifyFunc(func(arena *fastjson.Arena, val *fastjson.Value) (*fastjson.Value, bool, error) {
		inputDetails, err := pbtypes.JsonToProto(val)
		if err != nil {
			return nil, false, fmt.Errorf("get old details: json to proto: %w", err)
		}
		inputDetails = pbtypes.EnsureStructInited(inputDetails)
		newDetails, modified, err := proc(inputDetails)
		if err != nil {
			return nil, false, fmt.Errorf("run a modifier: %w", err)
		}
		if !modified {
			return nil, false, nil
		}
		newDetails = pbtypes.EnsureStructInited(newDetails)
		// Ensure ID is set
		newDetails.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)

		jsonVal := pbtypes.ProtoToJson(arena, newDetails)
		diff, err := pbtypes.DiffJson(val, jsonVal)
		if err != nil {
			return nil, false, fmt.Errorf("diff json: %w", err)
		}
		if len(diff) == 0 {
			return nil, false, nil
		}
		s.sendUpdatesToSubscriptions(id, newDetails)
		return jsonVal, true, nil
	}))
	arena.Reset()
	s.arenaPool.Put(arena)
	if err != nil {
		return fmt.Errorf("upsert details: %w", err)
	}
	return nil
}

func (s *dsObjectStore) extractDetailsByKey(txn *badger.Txn, key []byte) (*model.ObjectDetails, error) {
	it, err := txn.Get(key)
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	return s.unmarshalDetailsFromItem(it)
}

func (s *dsObjectStore) getPendingLocalDetails(txn *badger.Txn, key []byte) (*model.ObjectDetails, error) {
	return badgerhelper.GetValueTxn(txn, key, func(raw []byte) (*model.ObjectDetails, error) {
		var res model.ObjectDetails
		err := proto.Unmarshal(raw, &res)
		return &res, err
	})
}

func (s *dsObjectStore) updateObjectLinks(ctx context.Context, id string, links []string) (added []string, removed []string, err error) {
	_, err = s.links.UpsertId(ctx, id, query.ModifyFunc(func(arena *fastjson.Arena, val *fastjson.Value) (*fastjson.Value, bool, error) {
		prev := pbtypes.JsonArrayToStrings(val.GetArray(linkOutboundField))
		added, removed = slice.DifferenceRemovedAdded(prev, links)
		val.Set(linkOutboundField, pbtypes.StringsToJsonArray(arena, links))
		return val, len(added)+len(removed) > 0, nil
	}))
	return
}

func (s *dsObjectStore) sendUpdatesToSubscriptions(id string, details *types.Struct) {
	detCopy := pbtypes.CopyStruct(details, false)
	detCopy.Fields[database.RecordIDField] = pbtypes.ToValue(id)
	s.RLock()
	defer s.RUnlock()
	if s.onChangeCallback != nil {
		s.onChangeCallback(database.Record{
			Details: detCopy,
		})
	}
	for _, sub := range s.subscriptions {
		_ = sub.PublishAsync(id, detCopy)
	}
}
