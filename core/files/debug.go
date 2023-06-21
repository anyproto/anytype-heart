package files

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/util/debug"
)

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/syncstatus", debug.JSONHandler(s.debugFiles))
	r.Get("/queue", debug.JSONHandler(s.fileSync.DebugQueue))
	r.Get("/tree/{rootID}", debug.PlaintextHandler(s.printTree))
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

func (s *service) printTree(w io.Writer, req *http.Request) error {
	rawID := chi.URLParam(req, "rootID")
	id, err := cid.Parse(rawID)
	if err != nil {
		return fmt.Errorf("parse cid %s: %w", rawID, err)
	}
	return s.printNode(req.Context(), w, id, 0)
}

func (s *service) printNode(ctx context.Context, w io.Writer, id cid.Cid, level int) error {
	node, err := s.dagService.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get dag node %s: %w", id.String(), err)
	}
	size, err := node.Size()
	if err != nil {
		return fmt.Errorf("get size for node %s: %w", id.String(), err)
	}
	_, err = fmt.Fprintln(w, strings.Repeat("  ", level), id.String(), size)
	if err != nil {
		return fmt.Errorf("print node %s: %w", id.String(), err)
	}
	for _, link := range node.Links() {
		err = s.printNode(ctx, w, link.Cid, level+1)
		if err != nil {
			return err
		}
	}
	return nil
}
