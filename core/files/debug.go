package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/util/debug"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/status", debug.JSONHandler(s.debugFiles))
	r.Get("/queue", debug.JSONHandler(s.fileSync.DebugQueue))
	r.Get("/tree/{rootID}", debug.PlaintextHandler(s.printTree))
}

type fileDebugInfo struct {
	Hash       string
	SyncStatus int
	IsIndexed  bool
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

		var isIndexed bool
		details, err := s.objectStore.GetDetails(hash)
		if err != nil && !errors.Is(err, localstore.ErrNotFound) {
			return nil, fmt.Errorf("get status for %s: %s", hash, err)
		}
		if details != nil && !pbtypes.IsStructEmpty(details.Details) {
			isIndexed = true
		}

		result = append(result, &fileDebugInfo{
			Hash:       hash,
			SyncStatus: status,
			IsIndexed:  isIndexed,
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
	_, err = s.printNode(req.Context(), w, id, 0)
	return err
}

func (s *service) printNode(ctx context.Context, w io.Writer, id cid.Cid, level int) (uint64, error) {
	node, err := s.dagService.Get(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("get dag node %s: %w", id.String(), err)
	}
	size, err := node.Size()
	if err != nil {
		return 0, fmt.Errorf("get size for node %s: %w", id.String(), err)
	}

	prefix := strings.Repeat("|   ", level)
	_, err = fmt.Fprintf(w, "%s%s  totalSize=%d\n", prefix, id.String(), size)
	if err != nil {
		return 0, fmt.Errorf("print node %s: %w", id.String(), err)
	}
	var childrenSize uint64
	for _, link := range node.Links() {
		childSize, err := s.printNode(ctx, w, link.Cid, level+1)
		if err != nil {
			return 0, err
		}
		childrenSize += childSize
	}
	_, err = fmt.Fprintf(w, "%s%s %d\n", prefix, "node size", size-childrenSize)
	if err != nil {
		return 0, fmt.Errorf("print node %s: %w", id.String(), err)
	}
	return size, nil
}
