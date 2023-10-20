package objectstore

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *dsObjectStore) DebugRouter(r chi.Router) {
	r.Get("/keys", debug.PlaintextHandler(s.debugListKeys))
	r.Get("/keys/*", debug.PlaintextHandler(s.debugListKeysByPrefix))
	r.Get("/details/{id}", debug.JSONHandler(s.debugDetails))
}

func (s *dsObjectStore) debugDetails(req *http.Request) (*model.ObjectDetails, error) {
	id := chi.URLParam(req, "id")
	return s.GetDetails(id)
}

func (s *dsObjectStore) debugListKeys(w io.Writer, req *http.Request) error {
	return iterateKeysByPrefix(s.db, nil, func(key []byte) {
		w.Write(key)
		w.Write([]byte("\n"))
	})
}

func (s *dsObjectStore) debugListKeysByPrefix(w io.Writer, req *http.Request) error {
	prefix := chi.URLParam(req, "*")
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return iterateKeysByPrefix(s.db, []byte(prefix), func(key []byte) {
		w.Write(key)
		w.Write([]byte("\n"))
	})
}
