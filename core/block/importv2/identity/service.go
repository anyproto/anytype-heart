// Package identity owns all id semantics of an import run: minting tree ids
// for enumerated objects (pass 1), dedup-matching against existing objects,
// deriving keyed objects (relations, types, options) on demand, and resolving
// file-object ids through futures. Its resident state — the sourceKey→id
// index and the create-payload store — is the only run state allowed to scale
// with object count.
package identity

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

// TreePayloadCreator is the space subset used to mint/derive tree ids.
// Satisfied by clientspace.Space.
type TreePayloadCreator interface {
	CreateTreePayload(ctx context.Context, params payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error)
	DeriveTreePayload(ctx context.Context, params payloadcreator.PayloadDerivationParams) (treestorage.TreeStorageCreatePayload, error)
}

// Store is the space-index subset used for dedup queries. Satisfied by
// spaceindex.Store.
type Store interface {
	QueryObjectIds(q database.Query) (ids []string, total int, err error)
	Query(q database.Query) (records []database.Record, err error)
	QueryRaw(filters *database.Filters, limit int, offset int) (records []database.Record, err error)
}

// Assignment is the identity decision for one streamed object.
type Assignment struct {
	Id string
	// IsExisting means the object matched an existing one (or was already
	// assigned earlier this run): no tree creation, update path applies.
	IsExisting bool
	// Payload is the tree-create payload when a new tree must be created
	// (zero when IsExisting).
	Payload treestorage.TreeStorageCreatePayload
	// InternalKey is the state key for keyed (derived) objects; empty for
	// minted-class objects.
	InternalKey string
}

type entryMode int

const (
	entryMinted entryMode = iota
	entryMatched
	entryFile
)

type entry struct {
	id     string
	mode   entryMode
	future *fileFuture
	// claimed marks pass-1 claims; assigned marks their pass-2 arrival.
	// The gap between the two is the completeness-reconciliation input.
	claimed  bool
	assigned bool
	// reclaimable marks a crawl-resume rehydration seed: exactly one claim
	// for the key is absorbed as a reuse of the recorded identity (see
	// RehydratedEntry.Reclaimable). Cleared by that claim.
	reclaimable bool
}

// ClaimLedgerRecord is one pass-1 decision handed to the durable ledger:
// the minted id is write-ahead intent, and the serialized payload is what a
// later materialize-restart needs so it mints nothing.
type ClaimLedgerRecord struct {
	SourceKey    string
	ObjectId     string
	Matched      bool
	PayloadRoot  []byte // RawTreeChangeWithId proto; nil for matched claims
	PayloadHeads []string
}

// ClaimLedger is the durable write-through seam for claims, implemented (via
// a thin adapter wrapper) by runstore.Store.
type ClaimLedger interface {
	RecordClaims(ctx context.Context, claims []ClaimLedgerRecord) error
}

// claimBatchSize batches ledger writes (08-13 §5.2: measured 3-4x over
// per-claim commits). The batch's loss-harmlessness argument ("no side
// effects exist at claim time, the resumed pass simply re-mints") holds for
// PASS-1 claims only — they all flush before pass 2 appends anything. It
// EXPIRED for pass-2 late claims when DM-3 made the spool a durable
// artifact a resume replays: a spool row whose claim was lost fails the
// resumed pass 3, so the engine's spool sink flushes each late claim
// through before appending the claimed object (review P0-D).
const claimBatchSize = 500

// Option configures a Service.
type Option func(*Service)

// WithClaimLedger attaches the durable claim ledger.
func WithClaimLedger(ledger ClaimLedger) Option {
	return func(s *Service) { s.ledger = ledger }
}

// Service implements the identity index for one run. Claim, Assign and
// AssignDerived are called from the engine's single dispatch goroutine;
// Resolve, ResolveFile and CompleteFile are safe for concurrent worker use.
type Service struct {
	space          TreePayloadCreator
	store          Store
	updateExisting bool
	now            time.Time
	ledger         ClaimLedger

	mu       sync.RWMutex
	entries  map[string]*entry
	payloads map[string]treestorage.TreeStorageCreatePayload
	// derived memoizes uniqueKey → assignment so a repeated definition
	// converges to one object per run.
	derived map[string]Assignment
	// pending buffers claim records between ledger flushes, under its own
	// lock (claims arrive on one goroutine per pass by convention, but the
	// convention is unenforced and -race cannot see a convention).
	pendingMu sync.Mutex
	pending   []ClaimLedgerRecord
}

func NewService(space TreePayloadCreator, store Store, updateExisting bool, now time.Time, opts ...Option) *Service {
	s := &Service{
		space:          space,
		store:          store,
		updateExisting: updateExisting,
		now:            now,
		entries:        map[string]*entry{},
		payloads:       map[string]treestorage.TreeStorageCreatePayload{},
		derived:        map[string]Assignment{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Claim mints or dedup-matches one pass-1 identity claim. On a crawl-resumed
// run, a claim for a reclaimable rehydrated key is absorbed as a reuse of the
// recorded decision: no re-mint (the seed argument — a new payload's random
// seed would yield a different id while the spooled references point at the
// recorded one), no dedup re-query, no ledger re-record.
func (s *Service) Claim(ctx context.Context, c importv2.IdentityClaim) error {
	if c.SourceKey == "" {
		return fmt.Errorf("claim: empty source key")
	}
	s.mu.Lock()
	if e, dup := s.entries[c.SourceKey]; dup {
		if e.reclaimable {
			e.reclaimable = false // one-shot: the next claim is a bug again
			s.mu.Unlock()
			return nil
		}
		s.mu.Unlock()
		return fmt.Errorf("claim %q: duplicate source key", c.SourceKey)
	}
	s.mu.Unlock()

	// Minted (page-class) objects keep the flag's gate: whether a re-import
	// overwrites the user's pages is the user's call.
	id, err := s.matchExisting(c, false)
	if err != nil {
		return fmt.Errorf("claim %q: match existing: %w", c.SourceKey, err)
	}
	mode := entryMatched
	if id == "" {
		payload, err := s.space.CreateTreePayload(ctx, payloadcreator.PayloadCreationParams{
			Time:           s.now,
			SmartblockType: c.SbType,
		})
		if err != nil {
			return fmt.Errorf("claim %q: create tree payload: %w", c.SourceKey, err)
		}
		id = payload.RootRawChange.Id
		mode = entryMinted
		s.mu.Lock()
		s.payloads[id] = payload
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.entries[c.SourceKey] = &entry{id: id, mode: mode, claimed: true}
	s.mu.Unlock()
	return s.ledgerClaim(ctx, c.SourceKey, id, mode == entryMatched)
}

// ledgerClaim buffers one claim for the durable ledger, flushing full
// batches. The payload serializes at claim time (the root change is the
// id's own proof — a restart must reuse it, never re-mint).
func (s *Service) ledgerClaim(ctx context.Context, sourceKey, id string, matched bool) error {
	if s.ledger == nil {
		return nil
	}
	record := ClaimLedgerRecord{SourceKey: sourceKey, ObjectId: id, Matched: matched}
	if !matched {
		s.mu.RLock()
		payload, ok := s.payloads[id]
		s.mu.RUnlock()
		if ok {
			// RootRawChange is {RawChange []byte, Id string} and the Id IS
			// the objectId — the raw bytes alone reconstruct the payload.
			record.PayloadRoot = payload.RootRawChange.GetRawChange()
			record.PayloadHeads = payload.Heads
		}
	}
	s.pendingMu.Lock()
	s.pending = append(s.pending, record)
	full := len(s.pending) >= claimBatchSize
	s.pendingMu.Unlock()
	if full {
		return s.FlushClaims(ctx)
	}
	return nil
}

// FlushClaims writes the buffered claim batch through the ledger. The
// engine calls it at the end of pass 1 (and the sink's late claims ride the
// next flush or the pass-2 end).
func (s *Service) FlushClaims(ctx context.Context) error {
	if s.ledger == nil {
		return nil
	}
	// The lock is held across take, write and trim: the transaction is
	// what needs protecting, not the field (a released lock between take
	// and trim let overlapping flushes double-deliver and then panic on
	// the second trim). The ledger write is a local, bounded db write;
	// Claim contends only for its duration. E3 holds: the batch is
	// retained until the ledger accepts it.
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	if err := s.ledger.RecordClaims(ctx, s.pending); err != nil {
		return fmt.Errorf("record claims: %w", err)
	}
	s.pending = nil
	return nil
}

// Assign returns the pass-1 decision for a claimed object arriving in pass 2,
// transferring ownership of its create payload to the caller.
func (s *Service) Assign(sourceKey string) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[sourceKey]
	if !ok {
		return Assignment{}, fmt.Errorf("assign %q: object was not claimed in pass 1", sourceKey)
	}
	if e.mode == entryMatched {
		e.assigned = true
		return Assignment{Id: e.id, IsExisting: true}, nil
	}
	payload, ok := s.payloads[e.id]
	if !ok {
		return Assignment{}, fmt.Errorf("assign %q: create payload already taken", sourceKey)
	}
	delete(s.payloads, e.id)
	e.assigned = true
	return Assignment{Id: e.id, Payload: payload}, nil
}

// UnassignedClaims returns pass-1 claims that never arrived in pass 2 — the
// completeness-reconciliation input. Order is unspecified; the
// engine sorts for determinism.
func (s *Service) UnassignedClaims() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var keys []string
	for key, e := range s.entries {
		if e.claimed && !e.assigned {
			keys = append(keys, key)
		}
	}
	return keys
}

// AssignDerived resolves a derivable-class object (relation, type, option,
// profile) arriving in pass 2: dedup-match against existing objects, else
// derive a new keyed tree. The object's source key is registered in the index
// so later references resolve.
func (s *Service) AssignDerived(ctx context.Context, o *importv2.Object) (Assignment, error) {
	uniqueKey, err := s.uniqueKeyOf(o)
	if err != nil {
		return Assignment{}, fmt.Errorf("assign derived %q: %w", o.SourceKey, err)
	}

	s.mu.RLock()
	memo, seen := s.derived[uniqueKey.Marshal()]
	s.mu.RUnlock()
	if seen {
		s.register(o.SourceKey, memo.Id, entryMatched)
		memo.IsExisting = true
		memo.Payload = treestorage.TreeStorageCreatePayload{}
		return memo, nil
	}

	assignment, err := s.assignDerivedUncached(ctx, o, uniqueKey)
	if err != nil {
		return Assignment{}, err
	}
	s.mu.Lock()
	s.derived[uniqueKey.Marshal()] = Assignment{Id: assignment.Id, InternalKey: assignment.InternalKey}
	s.mu.Unlock()
	s.register(o.SourceKey, assignment.Id, entryMatched)
	return assignment, nil
}

func (s *Service) assignDerivedUncached(ctx context.Context, o *importv2.Object, uniqueKey domain.UniqueKey) (Assignment, error) {
	id, err := s.matchExistingDerived(o)
	if err != nil {
		return Assignment{}, fmt.Errorf("assign derived %q: match existing: %w", o.SourceKey, err)
	}
	if id != "" {
		internalKey, err := s.internalKeyOf(id)
		if err != nil {
			return Assignment{}, fmt.Errorf("assign derived %q: internal key of %q: %w", o.SourceKey, id, err)
		}
		return Assignment{Id: id, IsExisting: true, InternalKey: internalKey}, nil
	}

	// A previously deleted keyed object must not be resurrected under the
	// same key: mint a fresh key instead (v1 behavior).
	if s.isDeleted(uniqueKey.Marshal()) {
		freshKey := bson.NewObjectId().Hex()
		uniqueKey, err = domain.NewUniqueKey(o.SbType, freshKey)
		if err != nil {
			return Assignment{}, fmt.Errorf("assign derived %q: fresh unique key: %w", o.SourceKey, err)
		}
	}
	payload, err := s.space.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{Key: uniqueKey})
	if err != nil {
		return Assignment{}, fmt.Errorf("assign derived %q: derive tree payload: %w", o.SourceKey, err)
	}
	return Assignment{
		Id:          payload.RootRawChange.Id,
		Payload:     payload,
		InternalKey: uniqueKey.InternalKey(),
	}, nil
}

func (s *Service) uniqueKeyOf(o *importv2.Object) (domain.UniqueKey, error) {
	raw := o.Payload.Details.GetString(bundle.RelationKeyUniqueKey)
	if uniqueKey, err := domain.UnmarshalUniqueKey(raw); err == nil {
		return uniqueKey, nil
	}
	uniqueKey, err := domain.NewUniqueKey(o.SbType, o.Payload.Key)
	if err != nil {
		return nil, fmt.Errorf("unique key from %s and %q: %w", o.SbType, o.Payload.Key, err)
	}
	return uniqueKey, nil
}

func (s *Service) register(sourceKey, id string, mode entryMode) {
	s.mu.Lock()
	s.entries[sourceKey] = &entry{id: id, mode: mode}
	s.mu.Unlock()
}

// Resolve looks up a final id by source key. For file keys it returns the id
// only once the upload has resolved it (use ResolveFile to wait).
func (s *Service) Resolve(sourceKey string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[sourceKey]
	if !ok {
		return "", false
	}
	if e.mode == entryFile {
		id := e.future.resolvedId()
		return id, id != ""
	}
	return e.id, true
}

// Ids returns final ids for the given source keys, skipping unresolved ones.
func (s *Service) Ids(sourceKeys []string) []string {
	ids := make([]string, 0, len(sourceKeys))
	for _, key := range sourceKeys {
		if id, ok := s.Resolve(key); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
