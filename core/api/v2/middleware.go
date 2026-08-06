package apiv2

// middleware.go holds the API v2 plumbing that lives at the HTTP layer:
// the C8 idempotency store/middleware, the C9 dry-run scaffold, and the C6
// error responder.

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	v2handler "github.com/anyproto/anytype-heart/core/api/v2/handler"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

const (
	// IdempotencyKeyHeader is the C8 request header.
	IdempotencyKeyHeader = "Idempotency-Key"
	// idempotencyMaxEntries bounds the in-process store (LRU eviction);
	// persistence across restart is not required for v2.0 (§8).
	idempotencyMaxEntries = 1024
	// idempotencyMaxBody caps stored replay bodies; larger responses are
	// not replayable and simply re-execute.
	idempotencyMaxBody = 1 << 20 // 1 MiB
	// MaxRequestBody bounds the request body the idempotency middleware
	// buffers before the handler runs (C3). Without it, io.ReadAll here is
	// unbounded and bypasses the per-handler size caps (e.g. /v2/validate),
	// so a keyed POST with a huge body could OOM the process. Sized to match
	// the largest handler cap (validate = 10 MiB).
	MaxRequestBody = 10 << 20 // 10 MiB
)

// storedResult is a replayable response.
type storedResult struct {
	bodyHash    string
	status      int
	contentType string
	body        []byte
}

// idempotencyStore is the in-process C8 store: (space, Idempotency-Key) →
// (body-hash, stored result), bounded LRU.
type idempotencyStore struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List               // front = most recent
	pending map[string]chan struct{} // in-flight reservations (M4)
	max     int
}

type idempotencyEntry struct {
	key    string
	result storedResult
}

func newIdempotencyStore(maxEntries int) *idempotencyStore {
	return &idempotencyStore{
		entries: map[string]*list.Element{},
		order:   list.New(),
		pending: map[string]chan struct{}{},
		max:     maxEntries,
	}
}

func idempotencyStoreKey(spaceId, key string) string {
	return spaceId + "\x00" + key
}

// begin claims (space, key) for execution or returns a completed result to
// replay (M4). It blocks while another request with the same key is in-flight,
// then re-checks — so concurrent retries never double-execute. When it returns
// owner=true the caller MUST call finish exactly once (use defer, so a handler
// panic still releases the reservation).
func (s *idempotencyStore) begin(spaceId, key string) (result storedResult, replay bool, owner bool) {
	storeKey := idempotencyStoreKey(spaceId, key)
	for {
		s.mu.Lock()
		if el, ok := s.entries[storeKey]; ok {
			s.order.MoveToFront(el)
			res := el.Value.(*idempotencyEntry).result
			s.mu.Unlock()
			return res, true, false
		}
		if ch, ok := s.pending[storeKey]; ok {
			s.mu.Unlock()
			<-ch // wait for the in-flight owner, then re-check
			continue
		}
		s.pending[storeKey] = make(chan struct{})
		s.mu.Unlock()
		return storedResult{}, false, true
	}
}

// finish releases the reservation taken by begin, storing result for replay
// when it is non-nil (only successful responses are cached; a nil result lets
// a subsequent retry re-execute).
func (s *idempotencyStore) finish(spaceId, key string, result *storedResult) {
	storeKey := idempotencyStoreKey(spaceId, key)
	s.mu.Lock()
	defer s.mu.Unlock()
	if result != nil {
		s.putLocked(storeKey, *result)
	}
	if ch, ok := s.pending[storeKey]; ok {
		close(ch)
		delete(s.pending, storeKey)
	}
}

// putLocked stores a result under storeKey, evicting the least recent entry
// beyond the bound. The caller holds s.mu.
func (s *idempotencyStore) putLocked(storeKey string, result storedResult) {
	if el, ok := s.entries[storeKey]; ok {
		el.Value.(*idempotencyEntry).result = result
		s.order.MoveToFront(el)
		return
	}
	s.entries[storeKey] = s.order.PushFront(&idempotencyEntry{key: storeKey, result: result})
	for s.order.Len() > s.max {
		last := s.order.Back()
		s.order.Remove(last)
		delete(s.entries, last.Value.(*idempotencyEntry).key)
	}
}

// get returns the stored result for (space, key), refreshing recency.
func (s *idempotencyStore) get(spaceId, key string) (storedResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	el, ok := s.entries[idempotencyStoreKey(spaceId, key)]
	if !ok {
		return storedResult{}, false
	}
	s.order.MoveToFront(el)
	return el.Value.(*idempotencyEntry).result, true
}

// put stores a result for (space, key) directly (used in tests; the request
// path goes through begin/finish).
func (s *idempotencyStore) put(spaceId, key string, result storedResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putLocked(idempotencyStoreKey(spaceId, key), result)
}

// bodyRecorder captures the response for replay while streaming it through.
type bodyRecorder struct {
	gin.ResponseWriter
	buf      bytes.Buffer
	overflow bool
}

func (r *bodyRecorder) Write(p []byte) (int, error) {
	if r.buf.Len()+len(p) > idempotencyMaxBody {
		r.overflow = true
	} else {
		r.buf.Write(p)
	}
	return r.ResponseWriter.Write(p)
}

// ensureIdempotency implements C8 on mutation routes (POST, PATCH, PUT —
// and DELETE, the Phase-6 widening, carried by EVERY registered v2 DELETE:
// the chat message, type and property deletes alike, so C8 reads "every v2
// mutation" with no per-route exceptions): replay with the same key and
// body returns the stored result; the same key with a different body → 409
// idempotency_conflict. Requests without the header pass through. PATCH is
// where a blind agent retry does the most damage — a retried successful
// insertBlocks duplicates blocks, a retried deleteBlock 404s misleadingly —
// so the middleware covers it like POST (v0.3.5).
func ensureIdempotency(store *idempotencyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(IdempotencyKeyHeader)
		switch c.Request.Method {
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		default:
			c.Next()
			return
		}
		if key == "" {
			c.Next()
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, MaxRequestBody+1))
		if err != nil {
			respondV2Error(c, v2model.ValidationFailed("read request body: "+err.Error()))
			return
		}
		if len(body) > MaxRequestBody {
			respondV2Error(c, v2model.RequestTooLarge(fmt.Sprintf("request body exceeds the %d-byte limit", MaxRequestBody)))
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		// The hash identifies the whole request, not just its body:
		//   - method and path, because PATCH carries the target object in the
		//     PATH — two byte-identical edits to different objects under one
		//     reused key would otherwise replay the first object's success
		//     with its etag, silently leaving the second object unedited
		//     (no error an agent could repair from);
		//   - the query string, because a ?dry_run=true request and its later
		//     real twin share a body but must never replay for each other
		//     (C8/C9).
		hasher := sha256.New()
		for _, part := range []string{c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery} {
			hasher.Write([]byte(part))
			hasher.Write([]byte{0})
		}
		hasher.Write(body)
		bodyHash := hex.EncodeToString(hasher.Sum(nil))
		spaceId := c.Param("space_id")

		stored, replay, owner := store.begin(spaceId, key)
		if replay {
			if stored.bodyHash != bodyHash {
				respondV2Error(c, v2model.NewError(http.StatusConflict, v2model.CodeIdempotencyConflict,
					"Idempotency-Key was already used with a different request body — use a fresh key per distinct request"))
				return
			}
			c.Header("Idempotency-Replayed", "true")
			c.Data(stored.status, stored.contentType, stored.body)
			c.Abort()
			return
		}
		_ = owner // begin returns owner==true here; finish is deferred below

		// Release the reservation on the way out — via defer so a handler panic
		// (recovered upstream by gin.Recovery) still frees waiters (M4). result
		// stays nil unless the handler succeeded, so a failed/ panicked request
		// is not cached and a retry re-executes.
		var result *storedResult
		defer func() { store.finish(spaceId, key, result) }()

		recorder := &bodyRecorder{ResponseWriter: c.Writer}
		c.Writer = recorder
		c.Next()

		// only successful results replay; failures may be retried fresh
		status := recorder.Status()
		if status >= 200 && status < 300 && !recorder.overflow {
			result = &storedResult{
				bodyHash:    bodyHash,
				status:      status,
				contentType: recorder.Header().Get("Content-Type"),
				body:        append([]byte(nil), recorder.buf.Bytes()...),
			}
		}
	}
}

// dryRunKey is the context key ensureDryRun sets.
const dryRunKey = "dry_run"

// ensureDryRun parses the C9 ?dry_run=true flag into the request context.
// Mutation handlers (Phase 2+) read it via IsDryRun; until they land this
// is the no-op scaffold the spec asks for.
func ensureDryRun() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Query("dry_run") {
		case "", "false":
			c.Set(dryRunKey, false)
		case "true":
			c.Set(dryRunKey, true)
		default:
			respondV2Error(c, v2model.ValidationFailed("invalid dry_run value",
				v2model.Issue{Path: "dry_run", Message: "allowed values: true, false"}))
			return
		}
		c.Next()
	}
}

// IsDryRun reports whether the request asked for a dry run (C9).
func IsDryRun(c *gin.Context) bool {
	return c.GetBool(dryRunKey)
}

// respondV2Error writes a C6 error envelope and aborts the request.
func respondV2Error(c *gin.Context, err error) {
	v2handler.RespondV2Error(c, err)
}
