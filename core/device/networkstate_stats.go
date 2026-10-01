package device

/*
AI generated

Name: Network state debug stats
Scope: part of the networkState component

## Responsibility
- Retained counters and last-value fields for the connectivity recovery and
  wake (freeze) detection, exposed through debugstat (ProvideStat) so they
  survive renderer reloads; detailed per-wake values are in the WARN logs
*/

import (
	"fmt"

	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/core/domain"
)

// recoveryStats counts how the recovery mechanism is exercised. Every update
// is a single atomic op on an event-driven path (signals arrive at human
// timescales — network switches, wakes, RPCs), and the JSON snapshot is built
// only when the debug stat endpoint asks, so this adds no steady-state
// overhead. Per-wake detail (clock gaps, sources, decisions) is in the WARN
// logs, not here.
type recoveryStats struct {
	networkReports atomic.Int64
	// networkReportsDuplicate counts no-op reports (same type and id). The
	// ratio to networkReports shows the client's call pattern: near-100%
	// duplicates is a healthy client reporting on every OS path callback as
	// documented; 0 raw reports means the client isn't wired up at all.
	networkReportsDuplicate atomic.Int64
	foregroundEvents        atomic.Int64
	backgroundEvents        atomic.Int64
	// duplicate lifecycle reports (Foreground->Foreground, Background->
	// Background); duplicate foregrounds with a gap are a recovery trigger
	foregroundDuplicate atomic.Int64
	backgroundDuplicate atomic.Int64
	// transition recoveries skipped because a recent wake recovery covered
	// this background interval
	foregroundSuppressed atomic.Int64
	signalsCoalesced     atomic.Int64

	// executed recoveries per trigger, indexed like allTriggers
	executedByTrigger [len(allTriggers)]atomic.Int64

	trailingRuns    atomic.Int64
	trailingSkipped atomic.Int64

	recoveries          atomic.Int64
	recoveriesOffline   atomic.Int64
	lastRecoveryUnix    atomic.Int64
	lastRecoveryReason  atomic.String
	lastRecoveryTrigger atomic.String
	lastRecoveryWakeGen atomic.Int64

	refreshTimeouts   atomic.Int64
	closeWaitTimeouts atomic.Int64

	flushErrors         atomic.Int64
	flushTimeouts       atomic.Int64
	flushSkippedClosed  atomic.Int64
	lastFlushError      atomic.String
	lastFlushDurationMs atomic.Int64
}

func triggerCounts(c *[len(allTriggers)]atomic.Int64) map[string]int64 {
	m := make(map[string]int64, len(allTriggers))
	for i, t := range allTriggers {
		m[t] = c[i].Load()
	}
	return m
}

type networkStateStat struct {
	NetworkType      string `json:"networkType"`
	NetworkId        string `json:"networkId,omitempty"`
	ReportedByClient bool   `json:"reportedByClient"`
	LinkDown         bool   `json:"linkDown"`
	MonitorSnapshot  string `json:"monitorSnapshot"`
	MonitorGen       int64  `json:"monitorGeneration"`
	Offline          bool   `json:"offline"`
	Mobile           bool   `json:"mobile"`

	DeviceState             string `json:"deviceState"`
	PrevDeviceState         string `json:"prevDeviceState,omitempty"`
	Closing                 bool   `json:"closing"`
	NetworkReports          int64  `json:"networkReports"`
	NetworkReportsDuplicate int64  `json:"networkReportsDuplicate"`
	ForegroundEvents        int64  `json:"foregroundEvents"`
	BackgroundEvents        int64  `json:"backgroundEvents"`
	ForegroundDuplicate     int64  `json:"foregroundDuplicate"`
	BackgroundDuplicate     int64  `json:"backgroundDuplicate"`
	ForegroundSuppressed    int64  `json:"foregroundSuppressed"`
	SignalsCoalesced        int64  `json:"signalsCoalesced"`

	// wake generations: observed -> queued -> running -> completed
	WakeGenObserved     int64  `json:"wakeGenObserved"`
	WakeGenQueued       int64  `json:"wakeGenQueued"`
	WakeGenRunning      int64  `json:"wakeGenRunning"`
	WakeGenCompleted    int64  `json:"wakeGenCompleted"`
	LastWakeUnix        int64  `json:"lastWakeUnix,omitempty"`
	LastWakeFrozenForMs int64  `json:"lastWakeFrozenForMs,omitempty"`
	LastWakeSource      string `json:"lastWakeSource,omitempty"`
	LastWakeOutcome     string `json:"lastWakeOutcome,omitempty"`

	RecoveryQueued      bool             `json:"recoveryQueued"`
	RecoveryRunning     bool             `json:"recoveryRunning"`
	ExecutedByTrigger   map[string]int64 `json:"executedByTrigger"`
	TrailingRuns        int64            `json:"trailingRuns"`
	TrailingSkipped     int64            `json:"trailingSkipped"`
	Recoveries          int64            `json:"recoveries"`
	RecoveriesOffline   int64            `json:"recoveriesOffline"`
	LastRecoveryUnix    int64            `json:"lastRecoveryUnix,omitempty"`
	LastRecoveryReason  string           `json:"lastRecoveryReason,omitempty"`
	LastRecoveryTrigger string           `json:"lastRecoveryTrigger,omitempty"`
	LastRecoveryWakeGen int64            `json:"lastRecoveryWakeGen,omitempty"`

	RefreshTimeouts   int64 `json:"refreshTimeouts"`
	CloseWaitTimeouts int64 `json:"closeWaitTimeouts"`

	FlushErrors         int64  `json:"flushErrors"`
	FlushTimeouts       int64  `json:"flushTimeouts"`
	FlushSkippedClosed  int64  `json:"flushSkippedClosed"`
	LastFlushDurationMs int64  `json:"lastFlushDurationMs,omitempty"`
	LastFlushError      string `json:"lastFlushError,omitempty"`
}

func deviceStateName(s domain.CompState, reported bool) string {
	if !reported {
		return "unreported"
	}
	switch s {
	case domain.CompStateAppWentForeground:
		return "foreground"
	case domain.CompStateAppWentBackground:
		return "background"
	default:
		return fmt.Sprintf("state%d", s)
	}
}

func (n *networkState) ProvideStat() any {
	n.networkMu.Lock()
	state, id, reported := n.networkState, n.networkId, n.networkStateReported
	n.networkMu.Unlock()

	n.recoveryMu.Lock()
	w := n.wake
	deviceState := deviceStateName(n.lastDeviceState, n.deviceStateReported)
	prevState, closing := n.prevDeviceState, n.closing
	queued, running := n.queuedJob != nil || n.recoveryPending, n.recoveryBusy
	n.recoveryMu.Unlock()

	s := &n.stats
	st := networkStateStat{
		NetworkType:      state.String(),
		NetworkId:        id,
		ReportedByClient: reported,
		LinkDown:         n.linkDown.Load(),
		MonitorSnapshot:  n.monitorSnapshot.Load(),
		MonitorGen:       n.monitorGen.Load(),
		Offline:          n.IsOffline(),
		Mobile:           n.mobile,

		DeviceState:             deviceState,
		PrevDeviceState:         prevState,
		Closing:                 closing,
		NetworkReports:          s.networkReports.Load(),
		NetworkReportsDuplicate: s.networkReportsDuplicate.Load(),
		ForegroundEvents:        s.foregroundEvents.Load(),
		BackgroundEvents:        s.backgroundEvents.Load(),
		ForegroundDuplicate:     s.foregroundDuplicate.Load(),
		BackgroundDuplicate:     s.backgroundDuplicate.Load(),
		ForegroundSuppressed:    s.foregroundSuppressed.Load(),
		SignalsCoalesced:        s.signalsCoalesced.Load(),

		WakeGenObserved:  w.observed,
		WakeGenQueued:    w.queued,
		WakeGenRunning:   w.running,
		WakeGenCompleted: w.completed,
		LastWakeSource:   w.lastSource,
		LastWakeOutcome:  w.outcome(n.mobile),

		RecoveryQueued:      queued,
		RecoveryRunning:     running,
		ExecutedByTrigger:   triggerCounts(&s.executedByTrigger),
		TrailingRuns:        s.trailingRuns.Load(),
		TrailingSkipped:     s.trailingSkipped.Load(),
		Recoveries:          s.recoveries.Load(),
		RecoveriesOffline:   s.recoveriesOffline.Load(),
		LastRecoveryUnix:    s.lastRecoveryUnix.Load(),
		LastRecoveryReason:  s.lastRecoveryReason.Load(),
		LastRecoveryTrigger: s.lastRecoveryTrigger.Load(),
		LastRecoveryWakeGen: s.lastRecoveryWakeGen.Load(),

		RefreshTimeouts:   s.refreshTimeouts.Load(),
		CloseWaitTimeouts: s.closeWaitTimeouts.Load(),

		FlushErrors:         s.flushErrors.Load(),
		FlushTimeouts:       s.flushTimeouts.Load(),
		FlushSkippedClosed:  s.flushSkippedClosed.Load(),
		LastFlushDurationMs: s.lastFlushDurationMs.Load(),
		LastFlushError:      s.lastFlushError.Load(),
	}
	if w.observed > 0 {
		st.LastWakeUnix = w.lastAtWall.Unix()
		st.LastWakeFrozenForMs = w.lastFrozenFor.Milliseconds()
	}
	return st
}

func (n *networkState) StatId() string { return CName }

func (n *networkState) StatType() string { return CName }
