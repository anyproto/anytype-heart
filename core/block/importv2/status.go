package importv2

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"strings"
)

// The vocabulary of the progress surface, shared by the engine, the converters and the adapter's emitter so
// the producer side has ONE set of names. Everything here is advisory
// telemetry: it never affects control flow, carries no determinism
// requirement, and the golden harness ignores it.

// Phase is the coarse stage indicator. It is deliberately coarse: §15.3
// forbids a blended overall percentage (pass 2 runs at ~1.5 items/s against
// Notion's pacer, pass 3 at persist speed), so the phase — not one bar — is
// what tells a client which counters it is looking at.
type Phase int

const (
	// PhaseScanning is pass 1: the identity crawl. Totals are indeterminate
	// here — a cursor-chained /search does not know its own count until the
	// chain ends.
	PhaseScanning Phase = iota
	// PhaseAnalyzing is the converter-side structure-plan step between
	// scanning and fetching. It is the stall the plan step creates and
	// nothing ever reported: 10-20 s of silence with the LLM planner.
	PhaseAnalyzing
	// PhaseFetching is pass 2: crawl, convert, spool. Nothing enters the
	// space, so cancelling here undoes nothing.
	PhaseFetching
	// PhaseCreating is pass 3: materialize the spool into the space.
	PhaseCreating
	// PhaseFinalizing is the root collection, the report page, the widget.
	PhaseFinalizing
)

// String is the legacy progress message for the phase — the same free-text
// strings the pre-redesign Reporter.Phase call sites passed, so the legacy
// process scalar's message is unchanged by the seam redesign.
func (p Phase) String() string {
	switch p {
	case PhaseScanning:
		return "Scanning source"
	case PhaseAnalyzing:
		return "Analyzing structure"
	case PhaseFetching:
		return "Fetching content"
	case PhaseCreating:
		return "Creating objects"
	case PhaseFinalizing:
		return "Finalizing"
	default:
		return "Importing"
	}
}

// SpoolSpillPrefix names the files pass 2 drains its downloads into, inside
// the run's spill dir. It lives here because TWO packages depend on the
// naming: the engine writes them, and the status surface sums their sizes
// for a dormant run's bytesDone. The persister spills uploads into
// the same dir under a different prefix — those bytes are the same content
// read back rather than new transfer — so this prefix is exactly what keeps
// them out of the count.
const SpoolSpillPrefix = "spool-"

// SpillBytes sums the pass-2 download spill in dir: `bytesDone`,
// defined as BYTES ON DISK rather than bytes ever transferred. That is the
// only definition both halves of the surface can hold — a dormant dir can
// count nothing else, a resumed crawl inherits its predecessor's downloads,
// and a failed download whose partial file was discarded is work that must
// be redone, not progress.
//
// Only the spool sink's own files count: the persister spills uploads into
// the same dir under a different prefix, and those bytes are the same
// content read back, not new transfer. An unreadable dir sums to zero,
// which the schema already means as unknown — this is telemetry and must
// never fail a caller.
func SpillBytes(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), SpoolSpillPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}

// Kind separates the two counted classes. They are separate BY REQUIREMENT:
// 500 small files and one 2 GB file behave nothing alike, and the
// legacy blended scalar — one `Step(1)` for a page and for a file — is
// exactly what made a per-kind statistic impossible to publish honestly.
//
// KindPage counts MINTED content objects: the things a pass-1 claim exists
// for. Derived-class definitions (relations, types, options) are engine
// bookkeeping, not the user's pages, and are counted by neither kind — that
// is what keeps done <= total true in both directions, since the fetching
// denominator is the claim count and the materializing one is the spool's
// minted census.
type Kind int

const (
	KindPage Kind = iota
	KindFile
)

// DisplayText carries user content — a page title — to the wire's
// `currentItem`: displayable, NEVER loggable.
//
// The rule is enforced by the type rather than by review: String is the
// md5 hash, following the logging-hygiene rule this codebase already
// applies to user text (v1's notion importer hashes titles before they
// reach a log line or an error message — core/block/import/notion/api/
// block/link.go hashText). So a stray %s, %v, log.With or fmt.Errorf on a
// DisplayText yields a stable, non-reversible token; the plaintext is
// reachable only through the explicit Display accessor, which has exactly
// one caller — the pb message builder.
type DisplayText string

// String renders the hash. It exists so that logging a DisplayText by
// accident cannot leak a page title; use Display for the wire.
func (t DisplayText) String() string { return t.Hash() }

// Hash is the loggable form: empty for empty text, md5 hex otherwise.
func (t DisplayText) Hash() string {
	if t == "" {
		return ""
	}
	sum := md5.Sum([]byte(t))
	return hex.EncodeToString(sum[:])
}

// Display is the wire form — the only way to the plaintext. Every call site
// must be one that renders to a user, never one that records.
func (t DisplayText) Display() string { return string(t) }
