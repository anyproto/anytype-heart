// Package debugreporter exposes a small typed surface — Reporter — used by
// middleware subsystems (the metrics interceptor's long-method detector, the
// memory-growth watcher, critical error sites such as database corruption) to
// capture a debug artifact and broadcast an Event.Debug.ProfileCreated
// notification in one call.
//
// The interface lives in its own package, free of dependencies on metrics,
// event, pb or the profiler implementation, so any caller can import it
// without risking cycles. The concrete implementation is provided by the
// core/debug/profiler component and wired at bootstrap.
package debugreporter

// Kind picks the shape of the on-disk artifact produced alongside the event.
type Kind int

const (
	// KindNone emits only the event — no on-disk artifact is written. Use
	// for signals whose context fully fits in the `extra` map (database
	// corruption, invariant violations, etc.).
	KindNone Kind = iota
	// KindGoroutines writes a small snapshot zip with goroutines.txt +
	// info.json. Use when the actionable signal is "what is everyone
	// doing right now" (long-method detection, deadlock suspicion)
	// without the memory cost of a heap profile.
	KindGoroutines
	// KindHeap writes a snapshot zip with heap.pb.gz + goroutines.txt +
	// info.json (+ stat.json when available). Use for memory investigations
	// where the heap snapshot is load-bearing.
	KindHeap
	// KindTimedFull writes a richer archive: CPU profile, execution trace,
	// heap start/end and goroutine start/end, captured over
	// Capture.DurationSeconds. Intended for interactive support requests
	// via DebugRunProfiler; not yet produced by internal callers.
	KindTimedFull
)

// Capture is the "how" argument to Report. The zero value (Kind == KindNone)
// requests event-only delivery, which is the correct default for callers
// that already have enough context to describe the problem.
type Capture struct {
	Kind            Kind
	DurationSeconds int // honoured only when Kind == KindTimedFull
}

// Reporter is implemented by the profiler component. Callers pass a reason
// label (stable, short, grep-friendly — e.g. "MEMORY_GROWTH",
// "LONG_RPC", "DB_CORRUPTION"), an arbitrary map of extras that gets
// marshaled to JSON and forwarded verbatim to the event / info.json
// reasonDesc field, and a Capture describing whether and how to take a
// profile.
//
// Implementations must tolerate being called from any goroutine, including
// hot paths (the implementation may internally dispatch the capture). nil or
// empty extras are permitted and render as an omitted reasonDesc.
type Reporter interface {
	Report(reason string, extra map[string]any, capture Capture)
}
