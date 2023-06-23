package syncstatus

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/file_watchers", debug.JSONHandler(s.listFileWatchers))
}

type fileWatcherDebugInfo struct {
	ID        string
	IsLimited bool
}

func (s *service) listFileWatchers(_ *http.Request) ([]*fileWatcherDebugInfo, error) {
	files := s.fileWatcher.list()
	result := make([]*fileWatcherDebugInfo, 0, len(files))
	for _, file := range files {
		result = append(result, &fileWatcherDebugInfo{
			ID:        file.fileID,
			IsLimited: file.isUploadLimited,
		})
	}
	return result, nil
}
