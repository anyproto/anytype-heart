package spaceobjects

import (
	"encoding/json"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/valyala/fastjson"
)

// SetActiveViews accepts map of active views by blocks, as objects can handle multiple dataview blocks
func (s *dsObjectStore) SetActiveViews(objectId string, views map[string]string) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	it, err := keyValueItem(arena, objectId, views)
	if err != nil {
		return err
	}
	_, err = s.activeViews.UpsertOne(s.componentCtx, it)
	return err
}

func (s *dsObjectStore) SetActiveView(objectId, blockId, viewId string) error {
	views, err := s.GetActiveViews(objectId)
	// if active views are not found in BD, or we could not parse them, then we need to rewrite them
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		return err
	}
	if views == nil {
		views = make(map[string]string, 1)
	}
	views[blockId] = viewId
	return s.SetActiveViews(objectId, views)
}

// GetActiveViews returns a map of activeViews by block ids
func (s *dsObjectStore) GetActiveViews(objectId string) (map[string]string, error) {
	doc, err := s.activeViews.FindId(s.componentCtx, objectId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active view: %w", err)
	}
	val := doc.Value().GetStringBytes("value")
	views := map[string]string{}
	err = json.Unmarshal(val, &views)
	return views, err
}

func keyValueItem(arena *fastjson.Arena, key string, value any) (*fastjson.Value, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	obj := arena.NewObject()
	obj.Set("id", arena.NewString(key))
	obj.Set("value", arena.NewStringBytes(raw))
	return obj, nil
}
