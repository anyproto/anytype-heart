package objectstore

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *dsObjectStore) DebugRouter(r chi.Router) {
	r.Get("/details/{id}", debug.JSONHandler(s.debugDetails))
}

func (s *dsObjectStore) debugDetails(req *http.Request) (*model.ObjectDetails, error) {
	id := chi.URLParam(req, "id")
	return s.GetDetails("TODO", id)
}
