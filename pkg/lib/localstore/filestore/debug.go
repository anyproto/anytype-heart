package filestore

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *dsFileStore) DebugRouter(r chi.Router) {
	r.Get("/list", debug.JSONHandler(s.debugList))
	r.Get("/list_by_target/{targetID}", debug.JSONHandler(s.debugListByTarget))
}

func (s *dsFileStore) debugList(_ *http.Request) ([]*storage.FileInfo, error) {
	return sanitizeFileInfos(s.ListAllFileVariants())
}

func (s *dsFileStore) debugListByTarget(req *http.Request) ([]*storage.FileInfo, error) {
	id := chi.URLParam(req, "targetID")
	return sanitizeFileInfos(s.ListFileVariants(domain.FileId(id)))
}

func sanitizeFileInfos(infos []*storage.FileInfo, err error) ([]*storage.FileInfo, error) {
	if err != nil {
		return nil, err
	}
	out := make([]*storage.FileInfo, len(infos))
	for i, info := range infos {
		out[i] = sanitizeFileInfoForDebug(info)
	}
	return out, nil
}

func sanitizeFileInfoForDebug(info *storage.FileInfo) *storage.FileInfo {
	out := proto.Clone(info).(*storage.FileInfo)
	// out.Key = "<ENCRYPTION KEY>"
	// out.Name = "<SENSITIVE DATA>" + filepath.Ext(out.Name)
	return out
}
