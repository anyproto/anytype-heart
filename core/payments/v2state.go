package payments

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// getStatusV2Budget bounds a whole user-facing V2 GetStatus attempt: the wait
// for getStatusV2Limiter, dial, handshake and the RPC. It starts before the
// limiter wait.
var getStatusV2Budget = 15 * time.Second

// cacheCommitTimeout bounds the cache load and write done under the state
// lock; a write timeout is handled like any persistence failure, a load
// timeout leaves the mirror unloaded (retried on the next access)
var cacheCommitTimeout = 3 * time.Second

// v2Resource is one of the two independently fetched V2 resources
type v2Resource int

const (
	v2Status v2Resource = iota
	v2Products
	v2ResourceCount
)

func (r v2Resource) String() string {
	if r == v2Status {
		return "status"
	}
	return "products"
}

// fetchOrigin is who started a fetch (telemetry only)
type fetchOrigin int

const (
	originUser fetchOrigin = iota
	originBackground
	originForced
	originCount
)

func (o fetchOrigin) String() string {
	switch o {
	case originUser:
		return "user"
	case originBackground:
		return "background"
	default:
		return "forced"
	}
}

type v2ResourceState struct {
	// fetched: successfully fetched at least once (persisted)
	fetched bool
	// lastSuccessAt: wall time of the last successful fetch, zero if unknown
	lastSuccessAt time.Time
	// persistedSuccessAt: the last success time written to the cache. An
	// unchanged success is not written again while this is recent
	// (persistUnchangedAfter), so the 60s poll doesn't write every minute.
	persistedSuccessAt time.Time
	// lastErr: error of the latest accepted outcome, nil after a success
	lastErr error

	// fetch order: nextSeq is allocated right before the network call.
	// Successes and failures are ordered separately: a success newer than
	// the last applied success is applied even if a newer failure was
	// accepted (it then doesn't clear that failure); a failure must be newer
	// than both. acceptedSeq is the newest accepted outcome of either kind.
	nextSeq        uint64
	lastSuccessSeq uint64
	lastFailureSeq uint64
	acceptedSeq    uint64
	// verified: a network success was applied in this epoch. Data loaded
	// from the cache is served STALE until then (it may predate a success
	// whose write failed before a restart), and the loop fetches.
	verified bool
	// recoveryOwed: a caller was given a non-FRESH answer, so the next
	// successful commit publishes FRESH even if the data didn't change
	recoveryOwed bool
	// unpersisted: the in-memory data (authoritative) is newer than the cache
	// because its write failed; the next success is written even if
	// unchanged, and the background loop keeps fetching until it is
	unpersisted bool
	// revision orders the states of the resource: it moves on every
	// published success (data difference, recovery, first fetch) and on an
	// error-code transition (answered in responses only, no event). One
	// (epoch, revision) always labels one payload on every path.
	revision uint64
}

// v2State is the in-memory mirror of the V2 cache plus the per-resource
// freshness and ordering state. One mutex guards outcome acceptance, the
// cache write, response selection, recovery-debt registration and event
// publication, so these steps are atomic with respect to each other.
type v2State struct {
	mu     sync.Mutex
	loaded bool
	// loadFailed: a load failed, so answers may already have been given
	// from the empty mirror at the current revisions
	loadFailed bool
	epoch      uint64
	status     *model.MembershipV2Data
	products   []*model.MembershipV2Product
	expireTime time.Time
	res        [v2ResourceCount]v2ResourceState
}

func newV2State() *v2State {
	var b [8]byte
	_, _ = rand.Read(b[:])
	epoch := binary.LittleEndian.Uint64(b[:])
	if epoch == 0 {
		epoch = 1
	}
	return &v2State{epoch: epoch}
}

// v2Result is the outcome of one attempt to fetch a resource
type v2Result struct {
	res v2Resource
	// seq is the attempt's fetch order; 0 means it left the limiter queue
	// before reaching the network, which is not a resource outcome
	seq      uint64
	status   *model.MembershipV2Data
	products []*model.MembershipV2Product
	err      error
	// callerGone: the caller canceled; like leaving the queue, not a
	// resource outcome (seq is 0)
	callerGone bool
}

// v2Response is what a caller gets for one resource
type v2Response struct {
	status   *model.MembershipV2Data
	products []*model.MembershipV2Product
	state    *model.MembershipV2FetchState
	// err is nil for FRESH and for a STALE fallback after a transient error
	err error
}

func emptyV2Status() *model.MembershipV2Data {
	return &model.MembershipV2Data{Products: []*model.MembershipV2PurchasedProduct{}}
}

func emptyV2Products() []*model.MembershipV2Product {
	return []*model.MembershipV2Product{}
}

// NoneFetchState is explicit "unknown" metadata for responses produced
// before any outcome exists (not logged in, V2 disabled, invalid request)
func NoneFetchState() *model.MembershipV2FetchState {
	return &model.MembershipV2FetchState{Freshness: model.MembershipV2_NONE}
}

// v2LoadLocked fills the mirror from the cache once. A read failure leaves it
// unloaded, and the next access retries. A late load never replaces a
// resource already fetched in memory (memory is authoritative). The load is
// bounded by cacheCommitTimeout, since it runs under the state lock.
func (s *service) v2LoadLocked() {
	st := s.v2
	if st.loaded {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheCommitTimeout)
	defer cancel()
	snap, err := s.cache.CacheV2Load(ctx)
	if err != nil {
		st.loadFailed = true
		log.Debug("membership v2: can not load cache", zap.Error(err))
		return
	}
	st.loaded = true
	if st.expireTime.IsZero() {
		st.expireTime = snap.ExpireTime
	}
	if rs := &st.res[v2Status]; !rs.fetched {
		st.status = snap.Status
		s.v2InstallLoadedLocked(rs, snap.StatusFetched, snap.StatusLastSuccessAt)
	}
	if rs := &st.res[v2Products]; !rs.fetched {
		st.products = snap.Products
		s.v2InstallLoadedLocked(rs, snap.ProductsFetched, snap.ProductsLastSuccessAt)
	}
}

// v2InstallLoadedLocked installs a resource's cached metadata. After a
// failed load, answers were given from the empty mirror (NONE) at the
// current revision: installed data gets a new revision, and a recovery is
// owed so the next success publishes.
func (s *service) v2InstallLoadedLocked(rs *v2ResourceState, fetched bool, lastSuccessAt time.Time) {
	rs.fetched = fetched
	rs.lastSuccessAt = lastSuccessAt
	rs.persistedSuccessAt = lastSuccessAt
	if fetched && s.v2.loadFailed {
		rs.revision++
		rs.recoveryOwed = true
	}
}

// v2AllocSeq allocates the fetch order of an attempt; call it right before
// the network call, after any limiter wait
func (s *service) v2AllocSeq(r v2Resource) uint64 {
	s.v2.mu.Lock()
	defer s.v2.mu.Unlock()
	s.v2.res[r].nextSeq++
	return s.v2.res[r].nextSeq
}

// v2NeedsFetch tells the background loop to go to the network although the
// shared expiry hasn't passed: an error or an owed recovery notification is
// pending for a resource, or it was never fetched
func (s *service) v2NeedsFetch() bool {
	s.v2.mu.Lock()
	defer s.v2.mu.Unlock()
	s.v2LoadLocked()
	if !s.v2.loaded || !s.v2.expireTime.After(time.Now()) {
		return true
	}
	for _, rs := range s.v2.res {
		if rs.lastErr != nil || rs.recoveryOwed || !rs.fetched || rs.unpersisted || !rs.verified {
			return true
		}
	}
	return false
}

func (s *service) v2FreshnessLocked(r v2Resource, now time.Time) model.MembershipV2Freshness {
	rs := s.v2.res[r]
	switch {
	case !rs.fetched:
		return model.MembershipV2_NONE
	case rs.lastErr != nil, !rs.verified, rs.lastSuccessAt.IsZero(), rs.lastSuccessAt.After(now),
		now.Sub(rs.lastSuccessAt) > cache.CacheV2Lifetime():
		// unknown age, a wall clock stepped back, or simply old
		return model.MembershipV2_STALE
	default:
		return model.MembershipV2_FRESH
	}
}

func (s *service) v2FetchStateLocked(r v2Resource, freshness model.MembershipV2Freshness, errForCode error) *model.MembershipV2FetchState {
	rs := s.v2.res[r]
	st := &model.MembershipV2FetchState{
		Freshness:        freshness,
		LastRefreshError: refreshErrorCode(errForCode),
		Revision: &model.MembershipV2Revision{
			Epoch:   s.v2.epoch,
			Counter: rs.revision,
		},
	}
	if rs.fetched && !rs.lastSuccessAt.IsZero() {
		st.LastSuccessfulFetchAt = rs.lastSuccessAt.Unix()
	}
	return st
}

// v2DataLocked returns the committed data of r, or the empty placeholder
func (s *service) v2DataLocked(r v2Resource, resp *v2Response) {
	if r == v2Status {
		resp.status = s.v2.status
		if resp.status == nil || !s.v2.res[r].fetched {
			resp.status = emptyV2Status()
		}
		return
	}
	resp.products = s.v2.products
	if resp.products == nil || !s.v2.res[r].fetched {
		resp.products = emptyV2Products()
	}
}

// v2CacheOnly serves a cache-only read: FRESH only if the last success is
// recent, STALE if older, unknown or the latest refresh failed, NONE if never
// fetched
func (s *service) v2CacheOnly(r v2Resource) *v2Response {
	s.v2.mu.Lock()
	defer s.v2.mu.Unlock()
	s.v2LoadLocked()
	freshness := s.v2FreshnessLocked(r, time.Now())
	resp := &v2Response{state: s.v2FetchStateLocked(r, freshness, s.v2.res[r].lastErr)}
	s.v2DataLocked(r, resp)
	if freshness != model.MembershipV2_FRESH {
		s.v2.res[r].recoveryOwed = true
	}
	s.stats.resource(r).cacheRead(freshness)
	return resp
}

// v2None is the explicit "unknown" answer (local-only mode)
func (s *service) v2None(r v2Resource) *v2Response {
	resp := &v2Response{state: NoneFetchState()}
	if r == v2Status {
		resp.status = emptyV2Status()
	} else {
		resp.products = emptyV2Products()
	}
	return resp
}

type v2Verdict int

const (
	// left the limiter queue: not a resource outcome
	verdictQueueExit v2Verdict = iota
	// a newer outcome was already accepted
	verdictSuperseded
	verdictFailure
	verdictSuccess
)

// v2Round is one commitV2 call in progress
type v2Round struct {
	results  []v2Result
	verdicts []v2Verdict
	// resChanged: differs by the P0 equality set (membershipV2DataEqual,
	// productsV2Equal); drives `changed` (forced-poll stop, limits, expiry)
	resChanged [v2ResourceCount]bool
	// resDiffers: differs by full proto equality; any difference is
	// persisted and published at a new revision, so clients that order by
	// revision never drop it
	resDiffers [v2ResourceCount]bool
	// resWritten: the resource is part of this round's cache write
	resWritten [v2ResourceCount]bool
	commit     cache.V2Commit
	persisted  bool
}

func (rd *v2Round) changed() bool {
	return rd.resChanged[v2Status] || rd.resChanged[v2Products]
}

// commitV2 applies the results of one fetch round. Under one lock it
// (1) accepts each outcome only if its fetchSeq is newer than the last
// accepted one, (2) persists the accepted successes in one field-scoped
// cache write and only then updates the state and publishes events,
// (3) chooses each caller's response and, for a non-FRESH one, registers the
// recovery notification it is owed.
//
// Events are broadcast under the lock, so they leave in revision order. The
// desktop sender only enqueues; the mobile native event handler is called
// synchronously and must not re-enter a membership RPC from it (deadlock).
//
// callerCtx is the caller's own context (its cancellation is not a node
// failure); forCaller=false (background loop) skips response selection.
// renewExpiry renews the shared expiry, only if every result is an accepted
// success (a paired refresh) and something changed.
func (s *service) commitV2(callerCtx context.Context, results []v2Result, forCaller, renewExpiry bool) (resp [v2ResourceCount]*v2Response, changed bool) {
	s.v2.mu.Lock()
	defer s.v2.mu.Unlock()
	s.v2LoadLocked()

	rd := &v2Round{
		results:   results,
		verdicts:  make([]v2Verdict, len(results)),
		commit:    cache.V2Commit{SuccessAt: time.Now()},
		persisted: true,
	}
	allSuccess := s.v2AcceptLocked(rd)
	rd.commit.RenewExpiry = renewExpiry && allSuccess && (rd.resChanged[v2Status] || rd.resChanged[v2Products])
	s.v2PersistLocked(rd)
	s.v2ApplyLocked(rd)
	changed = rd.changed()
	if changed {
		s.notifyLimits()
	}

	if !forCaller {
		return resp, changed
	}
	for i, r := range results {
		out := s.v2SelectLocked(callerCtx, rd, i)
		if out.state.Freshness != model.MembershipV2_FRESH {
			s.v2.res[r.res].recoveryOwed = true
		}
		s.stats.resource(r.res).answered(out.state.Freshness, out.err)
		resp[r.res] = out
	}
	return resp, changed
}

// persistUnchangedAfter: an unchanged success is written to the cache only
// when the persisted success time is at least this old, so a restart still
// finds a recent success without a write on every 60s poll
func persistUnchangedAfter() time.Duration {
	return cache.CacheV2Lifetime() / 2
}

// v2AcceptLocked orders the outcomes by fetchSeq and stages the accepted
// successes for one field-scoped commit. It reports whether every result is
// an accepted success.
func (s *service) v2AcceptLocked(rd *v2Round) (allSuccess bool) {
	allSuccess = len(rd.results) > 0
	now := rd.commit.SuccessAt
	for i, r := range rd.results {
		rs := &s.v2.res[r.res]
		rstats := s.stats.resource(r.res)
		switch {
		case r.seq == 0:
			rd.verdicts[i] = verdictQueueExit
			if r.callerGone {
				rstats.callerGone.Inc()
			} else {
				rstats.queueExits.Inc()
			}
		case r.err != nil && r.seq > max(rs.lastSuccessSeq, rs.lastFailureSeq):
			rs.lastFailureSeq = r.seq
			rs.acceptedSeq = max(rs.acceptedSeq, r.seq)
			rd.verdicts[i] = verdictFailure
			rstats.failed(r.err)
		case r.err == nil && r.seq > rs.lastSuccessSeq:
			rs.lastSuccessSeq = r.seq
			rs.acceptedSeq = max(rs.acceptedSeq, r.seq)
			rd.verdicts[i] = verdictSuccess
			rstats.successes.Inc()
		default:
			rd.verdicts[i] = verdictSuperseded
			rstats.superseded.Inc()
		}
		if rd.verdicts[i] != verdictSuccess {
			allSuccess = false
			continue
		}
		if r.res == v2Status {
			rd.resChanged[r.res] = !membershipV2DataEqual(s.v2.status, r.status)
			rd.resDiffers[r.res] = !rs.fetched || !s.v2.status.Equal(r.status)
		} else {
			rd.resChanged[r.res] = !productsV2Equal(s.v2.products, r.products)
			rd.resDiffers[r.res] = !rs.fetched || rd.resChanged[r.res]
		}
		persistedAge := now.Sub(rs.persistedSuccessAt)
		// the skip lets the disk success time lag by up to the window (plus a
		// poll), so after a restart the data turns STALE that much earlier
		if !rd.resDiffers[r.res] && rs.lastErr == nil && !rs.recoveryOwed && !rs.unpersisted && !rs.persistedSuccessAt.IsZero() &&
			persistedAge >= 0 && persistedAge < persistUnchangedAfter() {
			continue
		}
		rd.resWritten[r.res] = true
		if r.res == v2Status {
			rd.commit.SetStatus, rd.commit.Status = true, r.status
		} else {
			rd.commit.SetProducts, rd.commit.Products = true, r.products
		}
	}
	return allSuccess
}

// v2PersistLocked writes the staged successes, bounded by
// cacheCommitTimeout
func (s *service) v2PersistLocked(rd *v2Round) {
	if !rd.commit.SetStatus && !rd.commit.SetProducts {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheCommitTimeout)
	defer cancel()
	if err := s.cache.CacheV2Commit(ctx, rd.commit); err != nil {
		rd.persisted = false
		s.stats.persistFailures.Inc()
		log.Warn("membership v2: can not persist fetched data, serving it from memory", zap.Error(err))
	}
}

// v2ApplyLocked updates the in-memory state after the write and publishes
// the events: on any data difference, a recovery after an error or an owed
// STALE answer, or the first successful fetch. A failed write doesn't stop
// it: memory is authoritative, and the resource is marked unpersisted.
func (s *service) v2ApplyLocked(rd *v2Round) {
	st := s.v2
	for i, r := range rd.results {
		rs := &st.res[r.res]
		switch rd.verdicts[i] {
		case verdictSuccess:
			s.v2ApplySuccessLocked(rd, r)
		case verdictFailure:
			if rs.lastErr == nil {
				log.Warn("membership v2: refresh failed", zap.Stringer("resource", r.res), zap.Error(r.err))
			} else {
				log.Debug("membership v2: refresh still failing", zap.Stringer("resource", r.res), zap.Error(r.err))
			}
			if rs.lastErr == nil || refreshErrorCode(rs.lastErr) != refreshErrorCode(r.err) {
				rs.revision++
			}
			rs.lastErr = r.err
		}
	}
}

func (s *service) v2ApplySuccessLocked(rd *v2Round, r v2Result) {
	st := s.v2
	rs := &st.res[r.res]
	prevErr, prevFetched, owed := rs.lastErr, rs.fetched, rs.recoveryOwed
	if r.res == v2Status {
		st.status = r.status
		if st.status == nil {
			st.status = emptyV2Status()
		}
	} else {
		st.products = r.products
		if st.products == nil {
			st.products = emptyV2Products()
		}
	}
	rs.fetched = true
	rs.verified = true
	rs.lastSuccessAt = rd.commit.SuccessAt.UTC()
	if rd.resWritten[r.res] {
		if rd.persisted {
			rs.persistedSuccessAt = rs.lastSuccessAt
			rs.unpersisted = false
		} else {
			rs.unpersisted = true
		}
	}
	// a newer failure was already accepted: the data is applied, the
	// failure stays (served STALE), and the recovery is still owed
	stillFailing := r.seq < rs.lastFailureSeq
	if !stillFailing {
		rs.lastErr = nil
	}
	if rd.commit.RenewExpiry {
		st.expireTime = time.Now().Add(cache.CacheV2Lifetime())
	}
	if prevErr != nil && r.seq > rs.lastFailureSeq {
		log.Info("membership v2: refresh recovered", zap.Stringer("resource", r.res))
	}
	if stillFailing {
		// no FRESH event while failing (no STALE events either): responses
		// carry the new data at a new revision
		if rd.resDiffers[r.res] || !prevFetched {
			rs.revision++
		}
		rs.recoveryOwed = true
		return
	}
	if rd.resDiffers[r.res] || prevErr != nil || owed || !prevFetched {
		rs.revision++
		rs.recoveryOwed = false
		s.publishV2Locked(r.res)
		if !rd.resDiffers[r.res] {
			s.stats.resource(r.res).recoveryEvents.Inc()
		}
	}
}

// v2SelectLocked chooses the response for results[i]
func (s *service) v2SelectLocked(callerCtx context.Context, rd *v2Round, i int) *v2Response {
	r := rd.results[i]
	rs := s.v2.res[r.res]
	out := &v2Response{}
	switch {
	case rd.verdicts[i] == verdictSuccess:
		out.status, out.products = r.status, r.products
		if out.status == nil && r.res == v2Status {
			out.status = emptyV2Status()
		}
		if out.products == nil && r.res == v2Products {
			out.products = emptyV2Products()
		}
		if rs.lastErr != nil {
			// a newer failure is still the latest outcome
			out.state = s.v2FetchStateLocked(r.res, model.MembershipV2_STALE, rs.lastErr)
		} else {
			out.state = s.v2FetchStateLocked(r.res, model.MembershipV2_FRESH, nil)
		}
	case rd.verdicts[i] == verdictSuperseded && rs.lastErr == nil && rs.fetched:
		// a newer outcome succeeded: the caller gets that snapshot
		s.v2DataLocked(r.res, out)
		out.state = s.v2FetchStateLocked(r.res, s.v2FreshnessLocked(r.res, time.Now()), nil)
	case rd.verdicts[i] == verdictSuperseded && r.err == nil:
		// own fetch succeeded but a newer one failed: the committed snapshot
		// as STALE with that failure's code, never another caller's error
		s.v2DataLocked(r.res, out)
		freshness := model.MembershipV2_STALE
		if !rs.fetched {
			freshness = model.MembershipV2_NONE
		}
		out.state = s.v2FetchStateLocked(r.res, freshness, rs.lastErr)
	default:
		// own failure, leaving the queue, the caller giving up, or superseded
		// by a newer failure: the fallback snapshot
		judged := r.err
		if judged == nil {
			judged = rs.lastErr
		}
		s.v2DataLocked(r.res, out)
		switch {
		case rs.fetched && (judged == nil || isTransientForCaller(callerCtx, judged)):
			out.state = s.v2FetchStateLocked(r.res, model.MembershipV2_STALE, judged)
		case rs.fetched:
			out.state = s.v2FetchStateLocked(r.res, model.MembershipV2_STALE, judged)
			out.err = judged
		default:
			out.state = s.v2FetchStateLocked(r.res, model.MembershipV2_NONE, judged)
			out.err = judged
		}
	}
	return out
}

// publishV2Locked sends the update event of r with FRESH metadata
func (s *service) publishV2Locked(r v2Resource) {
	state := s.v2FetchStateLocked(r, model.MembershipV2_FRESH, nil)
	s.stats.resource(r).eventsPublished.Inc()
	if r == v2Status {
		s.sendMembershipV2UpdateEvent(s.v2.status, state)
		return
	}
	s.sendMembershipV2ProductsUpdateEvent(s.v2.products, state)
}

func (s *service) sendMembershipV2UpdateEvent(membership *model.MembershipV2Data, state *model.MembershipV2FetchState) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipV2Update{
		MembershipV2Update: &pb.EventMembershipV2Update{
			Data:       membership,
			FetchState: state,
		},
	}))
}

func (s *service) sendMembershipV2ProductsUpdateEvent(products []*model.MembershipV2Product, state *model.MembershipV2FetchState) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfMembershipV2ProductsUpdate{
		MembershipV2ProductsUpdate: &pb.EventMembershipV2ProductsUpdate{
			Products:   products,
			FetchState: state,
		},
	}))
}

// fetchV2StatusAttempt is one bounded GetStatus attempt. The budget starts
// before the limiter wait; leaving the queue (cancellation or budget) returns
// seq=0. fetchSeq is allocated after the limiter is acquired and the context
// checked, right before the network call.
func (s *service) fetchV2StatusAttempt(ctx context.Context, origin fetchOrigin) v2Result {
	rstats := s.stats.resource(v2Status)
	rstats.attempts[origin].Inc()
	start := time.Now()
	budgetCtx, cancel := context.WithTimeoutCause(ctx, getStatusV2Budget, errCallBudget)
	defer cancel()

	select {
	case <-budgetCtx.Done():
		rstats.observe(time.Since(start), time.Since(start))
		return v2Result{res: v2Status, err: budgetCtx.Err(), callerGone: callerCanceled(ctx)}
	case s.getStatusV2Limiter <- struct{}{}:
	}
	queued := time.Since(start)
	if err := budgetCtx.Err(); err != nil {
		<-s.getStatusV2Limiter
		rstats.observe(queued, queued)
		return v2Result{res: v2Status, err: err, callerGone: callerCanceled(ctx)}
	}
	seq := s.v2AllocSeq(v2Status)
	rstats.inFlight.Inc()
	data, err := s.fetchV2Membership(budgetCtx)
	rstats.inFlight.Dec()
	<-s.getStatusV2Limiter
	rstats.observe(queued, time.Since(start))
	return s.v2AttemptResult(ctx, v2Result{res: v2Status, seq: seq, status: data, err: err})
}

// v2AttemptResult normalizes a network attempt's error with the caller's
// context known: the caller giving up is not an outcome (seq 0), and a
// cancellation from below is a lost connection
func (s *service) v2AttemptResult(callerCtx context.Context, r v2Result) v2Result {
	if r.err == nil {
		return r
	}
	r.err, r.callerGone = normalizeFetchErr(callerCtx, r.err)
	if r.callerGone {
		r.seq = 0
		return r
	}
	if errors.Is(r.err, context.DeadlineExceeded) {
		s.stats.resource(r.res).timeouts.Inc()
	}
	return r
}

// fetchV2ProductsAttempt is one bounded GetProducts attempt (no limiter)
func (s *service) fetchV2ProductsAttempt(ctx context.Context, origin fetchOrigin) v2Result {
	rstats := s.stats.resource(v2Products)
	rstats.attempts[origin].Inc()
	start := time.Now()
	seq := s.v2AllocSeq(v2Products)
	rstats.inFlight.Inc()
	products, err := s.fetchV2Products(ctx)
	rstats.inFlight.Dec()
	rstats.observe(0, time.Since(start))
	return s.v2AttemptResult(ctx, v2Result{res: v2Products, seq: seq, products: products, err: err})
}

// refreshV2 is the background/forced poll: both resources, one commit
func (s *service) refreshV2(ctx context.Context, force bool) (changed bool, err error) {
	// skip running loop if we are in local-only mode
	if s.cfg.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		return false, nil
	}
	if !force && !s.v2NeedsFetch() {
		return false, nil
	}
	origin := originBackground
	if force {
		origin = originForced
	}
	results := []v2Result{
		s.fetchV2ProductsAttempt(ctx, origin),
		s.fetchV2StatusAttempt(ctx, origin),
	}
	_, changed = s.commitV2(ctx, results, false, true)
	var errs []error
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
		}
	}
	return changed, errors.Join(errs...)
}

// notifyLimits asks the coordinator status and file node usage updaters to
// refresh. Both notifications are non-blocking: payments never runs or waits
// on that work.
func (s *service) notifyLimits() {
	s.stats.limitsNotifications.Inc()
	s.multiplayerLimitsUpdater.UpdateCoordinatorStatus()
	s.fileLimitsUpdater.RequestNodeUsageUpdate()
}
