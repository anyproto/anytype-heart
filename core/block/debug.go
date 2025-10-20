package block

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/go-chi/chi/v5"
	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *Service) DebugRouter(r chi.Router) {
	r.Get("/objects", debug.JSONHandler(s.debugListObjects))
	r.Get("/tree/{id}", debug.JSONHandler(s.debugTree))
	r.Get("/tree_in_space/{spaceId}/{id}", debug.JSONHandler(s.debugTreeInSpace))
	r.Get("/objects_per_space/{spaceId}", debug.JSONHandler(s.debugListObjectsPerSpace))
	r.Get("/objects/{id}", debug.JSONHandler(s.debugGetObject))
}

type debugTree struct {
	Id      string
	Changes []debugChange
}

type debugChange struct {
	Change    json.RawMessage
	Identity  string
	Timestamp string
}

type debugObject struct {
	ID      string
	Details json.RawMessage
	Store   *json.RawMessage `json:"Store,omitempty"`
	Blocks  *blockbuilder.Block

	Error string `json:"Error,omitempty"`
}

func (s *Service) debugListObjectsPerSpace(req *http.Request) ([]debugObject, error) {
	spaceId := chi.URLParam(req, "spaceId")
	ids, _, err := s.objectStore.SpaceIndex(spaceId).QueryObjectIds(database.Query{
		Filters: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("list ids: %w", err)
	}
	result := make([]debugObject, 0, len(ids))
	for _, id := range ids {
		obj, err := s.getDebugObject(id)
		if err != nil {
			obj = debugObject{
				ID:    id,
				Error: err.Error(),
			}
		}
		result = append(result, obj)
	}
	return result, nil
}

func (s *Service) debugListObjects(req *http.Request) ([]debugObject, error) {
	ids, err := s.objectStore.ListIdsCrossSpace()
	if err != nil {
		return nil, fmt.Errorf("list ids: %w", err)
	}
	result := make([]debugObject, 0, len(ids))
	for _, id := range ids {
		obj, err := s.getDebugObject(id)
		if err != nil {
			obj = debugObject{
				ID:    id,
				Error: err.Error(),
			}
		}
		result = append(result, obj)
	}
	return result, nil
}

func (s *Service) debugGetObject(req *http.Request) (debugObject, error) {
	id := chi.URLParam(req, "id")
	return s.getDebugObject(id)
}

func (s *Service) debugTree(req *http.Request) (debugTree, error) {
	id := chi.URLParam(req, "id")

	result := debugTree{
		Id: id,
	}
	err := cache.Do(s, id, func(sb smartblock.SmartBlock) error {
		ot, err := sb.Space().TreeBuilder().BuildHistoryTree(req.Context(), sb.Id(), objecttreebuilder.HistoryTreeOpts{})
		if err != nil {
			return err
		}
		return ot.IterateRoot(sourceimpl.UnmarshalChange, func(change *objecttree.Change) bool {
			change.Next = nil
			change.Previous = nil
			raw, err := json.Marshal(change)
			if err != nil {
				log.Error("debug tree: marshal change", zap.Error(err))
				return false
			}
			ts := time.Unix(change.Timestamp, 0)
			ch := debugChange{
				Change:    raw,
				Timestamp: ts.Format(time.RFC3339),
			}
			if change.Identity != nil {
				ch.Identity = change.Identity.Account()
			}
			result.Changes = append(result.Changes, ch)
			return true
		})
	})
	return result, err
}

// TODO Refactor
func (s *Service) debugTreeInSpace(req *http.Request) (debugTree, error) {
	spaceId := chi.URLParam(req, "spaceId")
	id := chi.URLParam(req, "id")

	result := debugTree{
		Id: id,
	}

	spc, err := s.spaceService.Get(context.Background(), spaceId)
	if err != nil {
		return result, fmt.Errorf("get space: %w", err)
	}

	err = spc.Do(id, func(sb smartblock.SmartBlock) error {
		ot := sb.Tree()
		return ot.IterateRoot(sourceimpl.UnmarshalChange, func(change *objecttree.Change) bool {
			change.Next = nil
			raw, err := json.Marshal(change)
			if err != nil {
				log.Error("debug tree: marshal change", zap.Error(err))
				return false
			}
			ts := time.Unix(change.Timestamp, 0)
			ch := debugChange{
				Change:    raw,
				Timestamp: ts.Format(time.RFC3339),
			}
			if change.Identity != nil {
				ch.Identity = change.Identity.Account()
			}
			result.Changes = append(result.Changes, ch)
			return true
		})
	})
	return result, err
}

func (s *Service) getDebugObject(id string) (debugObject, error) {
	var obj debugObject
	err := cache.Do(s, id, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		root := blockbuilder.BuildAST(st.Blocks())
		marshaller := jsonpb.Marshaler{}
		detailsRaw, err := marshaller.MarshalToString(st.CombinedDetails().ToProto())
		if err != nil {
			return fmt.Errorf("marshal details: %w", err)
		}

		var storeRaw *json.RawMessage
		if store := st.Store(); store != nil {
			raw, err := marshaller.MarshalToString(st.Store())
			if err != nil {
				return fmt.Errorf("marshal store: %w", err)
			}
			rawMessage := json.RawMessage(raw)
			storeRaw = &rawMessage
		}
		obj = debugObject{
			ID:      id,
			Store:   storeRaw,
			Details: json.RawMessage(detailsRaw),
			Blocks:  root,
		}
		return nil
	})
	return obj, err
}
