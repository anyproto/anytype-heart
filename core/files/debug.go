package files

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
)

func (s *service) runDebugServer() {
	if port, ok := os.LookupEnv("ANYDEBUG"); ok && port != "" {
		go func() {
			err := http.ListenAndServe(port, s)
			if err != nil {
				log.Errorf("debug server: %s", err)
			}
		}()
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *service) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "application/json")

	data, err := s.serveHTTP(req)
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

func (s *service) serveHTTP(req *http.Request) (interface{}, error) {
	switch req.URL.Path {
	case "/debug/files/syncstatus":
		return s.debugFiles()
	case "/debug/files/queue":
		return s.fileSync.DebugQueue()
	default:
		return nil, fmt.Errorf("unknown path %s", req.URL.Path)
	}
}

type fileDebugInfo struct {
	Hash       string
	SyncStatus int
}

func (s *service) debugFiles() ([]*fileDebugInfo, error) {
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
