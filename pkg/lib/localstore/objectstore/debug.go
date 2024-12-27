package objectstore

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *dsObjectStore) DebugRouter(r chi.Router) {
	r.Get("/details/{id}", debug.JSONHandler(s.debugDetails))
}

func (s *dsObjectStore) debugDetails(req *http.Request) (*domain.Details, error) {
	// id := chi.URLParam(req, "id")
	return nil, fmt.Errorf("not implemented")
	// return s.GetDetails("TODO", id)
}
