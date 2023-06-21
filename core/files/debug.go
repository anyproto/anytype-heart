package files

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
)

func (s *service) runDebugServer() {
	if port, ok := os.LookupEnv("ANYDEBUG"); ok && port != "" {
		go func() {
			r := chi.NewRouter()
			r.Route("/debug/files", func(r chi.Router) {
				r.Get("/syncstatus", jsonHandler(s.debugFiles))
				r.Get("/queue", jsonHandler(s.fileSync.DebugQueue))
			})
			err := http.ListenAndServe(port, r)
			if err != nil {
				log.Errorf("debug server: %s", err)
			}
		}()
	}
}

func jsonHandler[T any](handlerFunc func(req *http.Request) (T, error)) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")

		data, err := handlerFunc(req)
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			err := json.NewEncoder(rw).Encode(errorResponse{Error: err.Error()})
			if err != nil {
				log.Errorf("encode debug response for path %s error: %s", req.URL.Path, err)
			}
			return
		}
		err = json.NewEncoder(rw).Encode(data)
		if err != nil {
			log.Errorf("encode debug response for path %s error: %s", req.URL.Path, err)
		}
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

type fileDebugInfo struct {
	Hash       string
	SyncStatus int
}

func (s *service) debugFiles(_ *http.Request) ([]*fileDebugInfo, error) {
	hashes, err := s.fileStore.ListTargets()
	if err != nil {
		return nil, fmt.Errorf("list targets: %s", err)
	}
	result := make([]*fileDebugInfo, 0, len(hashes))
	for _, hash := range hashes {
		status, err := s.fileStore.GetSyncStatus(hash)
		if err == localstore.ErrNotFound {
			status = -1
			err = nil
		}
		if err != nil {
			return nil, fmt.Errorf("get status for %s: %s", hash, err)
		}
		result = append(result, &fileDebugInfo{
			Hash:       hash,
			SyncStatus: status,
		})
	}
	return result, nil
}
