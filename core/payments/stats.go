package payments

/*
AI generated

Name: Payments debug stats
Scope: part of the payments component

## Responsibility
- Retained counters for membership V2 fetch attempts, their outcomes
  (FRESH/STALE/NONE/error), ordering drops, recovery events and forced-poll
  admissions, exposed through debugstat (ProvideStat) so they survive renderer
  reloads. Counters carry denominators (attempts), so rates can be computed.
*/

import (
	"time"
	"unicode/utf8"

	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// resourceStats counts one resource's network attempts and their outcomes.
// Every attempt ends as exactly one of: success, failure (accepted),
// superseded, queue exit or caller gone, so
// sum(attempts) = successes + failures + superseded + queueExits + callerGone.
// answered* count what user calls were told; cacheRead* count cache-only reads
// (no network attempt).
type resourceStats struct {
	attempts [originCount]atomic.Int64

	successes atomic.Int64
	// failures: accepted failed outcomes, including ones answered as STALE
	failures          atomic.Int64
	transientFailures atomic.Int64
	// network calls that hit the budget or a deadline
	timeouts atomic.Int64
	// outcomes dropped: an older success after a newer success, or a failure
	// older than the newest accepted outcome
	superseded atomic.Int64
	// attempts that left the limiter queue on the budget (or the caller's
	// own deadline) without reaching the network
	queueExits atomic.Int64
	// attempts ended by the caller's own cancellation
	callerGone atomic.Int64

	answeredFresh atomic.Int64
	answeredStale atomic.Int64
	answeredNone  atomic.Int64
	answeredError atomic.Int64

	cacheReadFresh atomic.Int64
	cacheReadStale atomic.Int64
	cacheReadNone  atomic.Int64

	eventsPublished atomic.Int64
	// events published for a success without a data difference, after an
	// error or a STALE answer
	recoveryEvents atomic.Int64

	inFlight    atomic.Int64
	lastQueueMs atomic.Int64
	lastTotalMs atomic.Int64
	maxTotalMs  atomic.Int64
	lastOutcome atomic.String
	lastError   atomic.String
}

func (r *resourceStats) observe(queued, total time.Duration) {
	r.lastQueueMs.Store(queued.Milliseconds())
	ms := total.Milliseconds()
	r.lastTotalMs.Store(ms)
	for {
		cur := r.maxTotalMs.Load()
		if ms <= cur || r.maxTotalMs.CompareAndSwap(cur, ms) {
			return
		}
	}
}

// failed records an accepted failed outcome
func (r *resourceStats) failed(err error) {
	r.failures.Inc()
	if IsTransientError(err) {
		r.transientFailures.Inc()
	}
	r.lastError.Store(truncateErr(err.Error()))
}

// maxLastErrorLen bounds the retained and reported error text
const maxLastErrorLen = 256

// truncateErr cuts msg to at most maxLastErrorLen bytes on a rune boundary
func truncateErr(msg string) string {
	if len(msg) <= maxLastErrorLen {
		return msg
	}
	cut := maxLastErrorLen
	for cut > 0 && !utf8.RuneStart(msg[cut]) {
		cut--
	}
	return msg[:cut]
}

// answered records what a user call was told
func (r *resourceStats) answered(freshness model.MembershipV2Freshness, err error) {
	switch {
	case err != nil:
		r.answeredError.Inc()
		r.lastOutcome.Store("error")
	case freshness == model.MembershipV2_FRESH:
		r.answeredFresh.Inc()
		r.lastOutcome.Store("fresh")
	case freshness == model.MembershipV2_STALE:
		r.answeredStale.Inc()
		r.lastOutcome.Store("stale")
	default:
		r.answeredNone.Inc()
		r.lastOutcome.Store("none")
	}
}

func (r *resourceStats) cacheRead(freshness model.MembershipV2Freshness) {
	switch freshness {
	case model.MembershipV2_FRESH:
		r.cacheReadFresh.Inc()
	case model.MembershipV2_STALE:
		r.cacheReadStale.Inc()
	default:
		r.cacheReadNone.Inc()
	}
}

type paymentsStats struct {
	resources           [v2ResourceCount]resourceStats
	persistFailures     atomic.Int64
	limitsNotifications atomic.Int64
	// forced refresh requests from MembershipV2GetStatus.forceRefreshSec
	manualRequests atomic.Int64
}

func (p *paymentsStats) resource(r v2Resource) *resourceStats {
	return &p.resources[r]
}

type resourceStat struct {
	Freshness         string           `json:"freshness"`
	Fetched           bool             `json:"fetched"`
	LastSuccessUnix   int64            `json:"lastSuccessUnix,omitempty"`
	LastErr           string           `json:"lastErr,omitempty"`
	Revision          uint64           `json:"revision"`
	RecoveryOwed      bool             `json:"recoveryOwed"`
	Attempts          map[string]int64 `json:"attempts"`
	Successes         int64            `json:"successes"`
	Failures          int64            `json:"failures"`
	TransientFailures int64            `json:"transientFailures"`
	Timeouts          int64            `json:"timeouts"`
	Superseded        int64            `json:"superseded"`
	QueueExits        int64            `json:"queueExits"`
	CallerGone        int64            `json:"callerGone"`
	AnsweredFresh     int64            `json:"answeredFresh"`
	AnsweredStale     int64            `json:"answeredStale"`
	AnsweredNone      int64            `json:"answeredNone"`
	AnsweredError     int64            `json:"answeredError"`
	CacheReadFresh    int64            `json:"cacheReadFresh"`
	CacheReadStale    int64            `json:"cacheReadStale"`
	CacheReadNone     int64            `json:"cacheReadNone"`
	EventsPublished   int64            `json:"eventsPublished"`
	RecoveryEvents    int64            `json:"recoveryEvents"`
	InFlight          int64            `json:"inFlight"`
	LastQueueMs       int64            `json:"lastQueueMs"`
	LastTotalMs       int64            `json:"lastTotalMs"`
	MaxTotalMs        int64            `json:"maxTotalMs"`
	LastOutcome       string           `json:"lastOutcome,omitempty"`
	LastError         string           `json:"lastError,omitempty"`
}

type forceStat struct {
	Admissions      map[string]int64 `json:"admissions"`
	Extended        map[string]int64 `json:"extended"`
	Expired         map[string]int64 `json:"expired"`
	ManualRejected  int64            `json:"manualRejected"`
	ForcedFetches   int64            `json:"forcedFetches"`
	StoppedOnChange int64            `json:"stoppedOnChange"`
	Active          map[string]bool  `json:"active"`
}

type paymentsStat struct {
	V2Enabled           bool          `json:"v2Enabled"`
	Epoch               uint64        `json:"epoch,omitempty"`
	Status              *resourceStat `json:"status,omitempty"`
	Products            *resourceStat `json:"products,omitempty"`
	PersistFailures     int64         `json:"persistFailures"`
	LimitsNotifications int64         `json:"limitsNotifications"`
	ManualRequests      int64         `json:"manualRequests"`
	Force               *forceStat    `json:"force,omitempty"`
}

func (rc *refreshController) stat() *forceStat {
	if rc == nil {
		return nil
	}
	st := &forceStat{
		Admissions:      map[string]int64{},
		Extended:        map[string]int64{},
		Expired:         map[string]int64{},
		Active:          map[string]bool{},
		ManualRejected:  rc.stats.manualRejected.Load(),
		ForcedFetches:   rc.stats.forcedFetches.Load(),
		StoppedOnChange: rc.stats.stoppedOnChange.Load(),
	}
	rc.mu.Lock()
	for o := forceOrigin(0); o < forceOriginCount; o++ {
		st.Active[o.String()] = rc.intents[o].active
	}
	rc.mu.Unlock()
	for o := forceOrigin(0); o < forceOriginCount; o++ {
		st.Admissions[o.String()] = rc.stats.admissions[o].Load()
		st.Extended[o.String()] = rc.stats.extended[o].Load()
		st.Expired[o.String()] = rc.stats.expired[o].Load()
	}
	return st
}

func (s *service) ProvideStat() any {
	st := paymentsStat{
		V2Enabled:           s.cfg != nil && s.cfg.EnableMembershipV2,
		PersistFailures:     s.stats.persistFailures.Load(),
		LimitsNotifications: s.stats.limitsNotifications.Load(),
		ManualRequests:      s.stats.manualRequests.Load(),
	}
	s.v2.mu.Lock()
	st.Epoch = s.v2.epoch
	now := time.Now()
	var out [v2ResourceCount]*resourceStat
	for r := v2Resource(0); r < v2ResourceCount; r++ {
		rs := s.v2.res[r]
		c := s.stats.resource(r)
		rst := &resourceStat{
			Freshness:         s.v2FreshnessLocked(r, now).String(),
			Fetched:           rs.fetched,
			Revision:          rs.revision,
			RecoveryOwed:      rs.recoveryOwed,
			Attempts:          map[string]int64{},
			Successes:         c.successes.Load(),
			Failures:          c.failures.Load(),
			TransientFailures: c.transientFailures.Load(),
			Timeouts:          c.timeouts.Load(),
			Superseded:        c.superseded.Load(),
			QueueExits:        c.queueExits.Load(),
			CallerGone:        c.callerGone.Load(),
			AnsweredFresh:     c.answeredFresh.Load(),
			AnsweredStale:     c.answeredStale.Load(),
			AnsweredNone:      c.answeredNone.Load(),
			AnsweredError:     c.answeredError.Load(),
			CacheReadFresh:    c.cacheReadFresh.Load(),
			CacheReadStale:    c.cacheReadStale.Load(),
			CacheReadNone:     c.cacheReadNone.Load(),
			EventsPublished:   c.eventsPublished.Load(),
			RecoveryEvents:    c.recoveryEvents.Load(),
			InFlight:          c.inFlight.Load(),
			LastQueueMs:       c.lastQueueMs.Load(),
			LastTotalMs:       c.lastTotalMs.Load(),
			MaxTotalMs:        c.maxTotalMs.Load(),
			LastOutcome:       c.lastOutcome.Load(),
			LastError:         c.lastError.Load(),
		}
		if !rs.lastSuccessAt.IsZero() {
			rst.LastSuccessUnix = rs.lastSuccessAt.Unix()
		}
		if rs.lastErr != nil {
			rst.LastErr = truncateErr(rs.lastErr.Error())
		}
		for o := fetchOrigin(0); o < originCount; o++ {
			rst.Attempts[o.String()] = c.attempts[o].Load()
		}
		out[r] = rst
	}
	s.v2.mu.Unlock()
	st.Status, st.Products = out[v2Status], out[v2Products]
	if s.refreshCtrlV2 != nil {
		st.Force = s.refreshCtrlV2.stat()
	} else if s.refreshCtrl != nil {
		st.Force = s.refreshCtrl.stat()
	}
	return st
}

func (s *service) StatId() string { return CName }

func (s *service) StatType() string { return CName }
