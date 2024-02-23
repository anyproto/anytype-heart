package block

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/go-chi/chi/v5"
	"github.com/gogo/protobuf/jsonpb"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) DebugRouter(r chi.Router) {
	r.Get("/objects", debug.JSONHandler(s.debugListObjects))
	r.Get("/tree/{id}", debug.JSONHandler(s.debugTree))
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
	ids, _, err := s.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceId),
				Condition:   model.BlockContentDataviewFilter_Equal,
			},
		},
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
	ids, err := s.objectStore.ListIds()
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
	err := Do(s, id, func(sb smartblock.SmartBlock) error {
		ot := sb.Tree()
		return ot.IterateRoot(source.UnmarshalChange, func(change *objecttree.Change) bool {
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
	err := Do(s, id, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		root := blockbuilder.BuildAST(st.Blocks())
		marshaller := jsonpb.Marshaler{}
		detailsRaw, err := marshaller.MarshalToString(st.CombinedDetails())
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
