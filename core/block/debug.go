package block

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *Service) DebugRouter(r chi.Router) {
	r.Get("/objects", debug.JSONHandler(s.debugListObjects))
	r.Get("/objects/{id}", debug.JSONHandler(s.debugGetObject))
}

type debugObject struct {
	ID      string
	Details json.RawMessage
	Store   *json.RawMessage `json:"Store,omitempty"`
	Blocks  *blockbuilder.Block

	Error string `json:"Error,omitempty"`
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
