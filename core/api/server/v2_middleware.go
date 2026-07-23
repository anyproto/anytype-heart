package server

// v2_middleware.go holds the API v2 plumbing that lives at the HTTP layer:
// the C8 idempotency store/middleware, the C9 dry-run scaffold, and the C6
// error responder.

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/handler"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
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
	order   *list.List // front = most recent
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
		max:     maxEntries,
	}
}

func idempotencyStoreKey(spaceId, key string) string {
	return spaceId + "\x00" + key
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

// put stores a result for (space, key), evicting the least recent entry
// beyond the bound.
func (s *idempotencyStore) put(spaceId, key string, result storedResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	storeKey := idempotencyStoreKey(spaceId, key)
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

// ensureIdempotency implements C8 on POST routes: replay with the same key
// and body returns the stored result; the same key with a different body →
// 409 idempotency_conflict. Requests without the header pass through.
func ensureIdempotency(store *idempotencyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(IdempotencyKeyHeader)
		if key == "" || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			respondV2Error(c, apimodel.V2ValidationFailed("read request body: "+err.Error()))
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		hash := sha256.Sum256(body)
		bodyHash := hex.EncodeToString(hash[:])
		spaceId := c.Param("space_id")

		if stored, ok := store.get(spaceId, key); ok {
			if stored.bodyHash != bodyHash {
				respondV2Error(c, apimodel.NewV2Error(http.StatusConflict, apimodel.V2CodeIdempotencyConflict,
					"Idempotency-Key was already used with a different request body — use a fresh key per distinct request"))
				return
			}
			c.Header("Idempotency-Replayed", "true")
			c.Data(stored.status, stored.contentType, stored.body)
			c.Abort()
			return
		}

		recorder := &bodyRecorder{ResponseWriter: c.Writer}
		c.Writer = recorder
		c.Next()

		// only successful results replay; failures may be retried fresh
		status := recorder.Status()
		if status >= 200 && status < 300 && !recorder.overflow {
			store.put(spaceId, key, storedResult{
				bodyHash:    bodyHash,
				status:      status,
				contentType: recorder.Header().Get("Content-Type"),
				body:        append([]byte(nil), recorder.buf.Bytes()...),
			})
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
			respondV2Error(c, apimodel.V2ValidationFailed("invalid dry_run value",
				apimodel.V2Issue{Path: "dry_run", Message: "allowed values: true, false"}))
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
	handler.RespondV2Error(c, err)
}
