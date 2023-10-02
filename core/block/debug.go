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
	r.Get("/objects", debug.JSONHandler(s.debugObjects))
}

type debugObject struct {
	ID      string
	Details json.RawMessage
	Store   *json.RawMessage
	Blocks  *blockbuilder.Block
}

func (s *Service) debugObjects(req *http.Request) ([]debugObject, error) {
	ids, err := s.objectStore.ListIds()
	if err != nil {
		return nil, fmt.Errorf("list ids: %w", err)
	}
	result := make([]debugObject, 0, len(ids))
	marshaller := jsonpb.Marshaler{}
	for _, id := range ids {
		err = Do(s, id, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			root := blockbuilder.BuildAST(st.Blocks())
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
			result = append(result, debugObject{
				ID:      id,
				Store:   storeRaw,
				Details: json.RawMessage(detailsRaw),
				Blocks:  root,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("can't get object %s: %w", id, err)
		}
	}
	return result, nil
}
