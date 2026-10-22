package identity

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

// fileFuture is the pending id of a file object whose final identity comes
// from its upload.
type fileFuture struct {
	done chan struct{}

	mu  sync.Mutex
	id  string
	err error
}

func newFileFuture() *fileFuture {
	return &fileFuture{done: make(chan struct{})}
}

func (f *fileFuture) complete(id string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.done:
		return // already completed
	default:
	}
	f.id, f.err = id, err
	close(f.done)
}

func (f *fileFuture) resolvedId() string {
	select {
	case <-f.done:
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.id
	default:
		return ""
	}
}

// RegisterFile creates the (unresolved) index entry for a file object the
// moment it is emitted, in stream order — this is what makes later waits
// deadlock-free. Registering the same key twice is a no-op.
func (s *Service) RegisterFile(sourceKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[sourceKey]; ok {
		return
	}
	s.entries[sourceKey] = &entry{mode: entryFile, future: newFileFuture()}
}

// CompleteFile resolves a file future with the uploaded object id (or the
// upload failure, which propagates to every waiting reference).
func (s *Service) CompleteFile(sourceKey, id string, err error) {
	s.mu.RLock()
	e, ok := s.entries[sourceKey]
	s.mu.RUnlock()
	if !ok || e.mode != entryFile {
		return
	}
	e.future.complete(id, err)
	if err == nil {
		s.mu.Lock()
		e.id = id
		s.mu.Unlock()
	}
}

// ResolveRef resolves any reference by source key: immediate for minted,
// matched and derived entries, future-waiting for file entries. Returns
// found=false for unknown keys; a non-nil error means the wait was cancelled
// or the file's upload failed.
func (s *Service) ResolveRef(ctx context.Context, sourceKey string) (id string, found bool, err error) {
	s.mu.RLock()
	e, ok := s.entries[sourceKey]
	s.mu.RUnlock()
	if !ok {
		return "", false, nil
	}
	if e.mode != entryFile {
		return e.id, true, nil
	}
	id, err = s.ResolveFile(ctx, sourceKey)
	if err != nil {
		return "", true, err
	}
	return id, true, nil
}

// ResolveFile waits (ctx-aware) for a file object's final id. A reference to
// a never-registered file key is a converter contract violation
// (use-before-definition) and returns an invariant issue instead of hanging.
func (s *Service) ResolveFile(ctx context.Context, sourceKey string) (string, error) {
	s.mu.RLock()
	e, ok := s.entries[sourceKey]
	s.mu.RUnlock()
	if !ok {
		return "", importv2.Issue{
			Severity:  importv2.SeverityObjectError,
			Code:      importv2.IssueInvariant,
			SourceKey: sourceKey,
			Message:   "file referenced before its definition was emitted",
		}
	}
	if e.mode != entryFile {
		// A non-file entry under a file key: resolve directly.
		return e.id, nil
	}
	select {
	case <-e.future.done:
		e.future.mu.Lock()
		defer e.future.mu.Unlock()
		if e.future.err != nil {
			return "", fmt.Errorf("file %q upload: %w", sourceKey, e.future.err)
		}
		return e.future.id, nil
	case <-ctx.Done():
		return "", fmt.Errorf("wait for file %q: %w", sourceKey, ctx.Err())
	}
}
