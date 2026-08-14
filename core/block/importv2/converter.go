package importv2

import (
	"context"

	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Converter turns one source into a stream of objects. A converter instance
// is constructed per run and must not be shared between runs.
//
// Contract (enforced by engine assertions and tests):
//
//  1. One object at a time: Sink.Object blocks for backpressure; after it
//     returns the converter must drop its reference to the object.
//  2. Definitions before use: an object that defines identity others
//     reference — a relation, a type, an option, a file object — is emitted
//     before the first object referencing it.
//  3. No silent drops: every unsupported or lossy construct produces a
//     placeholder block or a Sink.Issue with a structured code, or both.
//  4. No store access: a converter sees only its source, the network (for
//     web-based formats) and the sink.
//  5. Deterministic: the same source yields the same objects in the same
//     order (stable iteration, no random ids or colors in emitted payloads).
type Converter interface {
	Name() string

	// EnumerateIdentities is pass 1: cheaply yield one IdentityClaim per
	// minted-class object this converter will emit, without reading heavy
	// content (block trees, file bytes). Derivable-class objects (relations,
	// types, options) and file objects are not claimed here.
	EnumerateIdentities(ctx context.Context, yield func(IdentityClaim) error) error

	// Convert is pass 2: stream every object. It must honor ctx promptly,
	// including mid-object (network fetches, downloads). The returned
	// RootSpec describes the run's root collection/widget; it is valid only
	// when the returned error is nil.
	Convert(ctx context.Context, sink Sink) (RootSpec, error)
}

// IdentityClaim announces one minted-class object during pass 1 so the
// identity service can dedup-match it against existing objects or mint a new
// tree id for it before any content streams.
type IdentityClaim struct {
	SourceKey string
	SbType    coresb.SmartBlockType

	// Dedup keys in match priority order; empty values are skipped.
	OldAnytypeID   string
	UniqueKey      string
	SourceFilePath string
}

// Sink is the converter's only output. Implementations are provided by the
// engine and are safe for use from a single converter goroutine; Issue and
// Progress are additionally safe for concurrent use (converter-internal
// worker pools).
type Sink interface {
	// Object hands one object to the engine, blocking for backpressure.
	// A non-nil error (context cancellation or a fatal engine state) means
	// the converter must stop and return.
	Object(ctx context.Context, o *Object) error

	// Issue records a warning or per-object error. It never blocks and never
	// aborts by itself; the engine applies the single mode predicate.
	Issue(i Issue)

	// Progress adds fine-grained progress ticks beyond the engine's
	// per-object accounting (e.g. per search page during a crawl).
	Progress(delta int64)

	// Claim registers a late identity claim for an entity discovered only
	// during pass 2 (second-chance discovery, §16 item 3 — e.g. a Notion
	// child page the eventually-consistent /search index omitted). The claim
	// must precede the emission of any object referencing the claimed key,
	// and the converter must eventually emit the claimed object (or report
	// an issue for it) — the claims-reconciliation invariant applies.
	Claim(ctx context.Context, claim IdentityClaim) error
}

// ResumableConverter is implemented by converters that can cheaply skip
// re-converting objects a previous incarnation already recorded (the 08-13
// §6.3 seam, serving DM-3's pass-2 crawl resume: the spool is the skip set).
// Skip is engine-provided, safe for concurrent use, and purely an
// optimization — the engine enforces recorded-row dedup at the sink
// regardless, so a converter that ignores the seam is merely slower, never
// incorrect. For Notion each skipped page saves the ~2 requests that make an
// interrupted crawl expensive to redo.
type ResumableConverter interface {
	Converter
	SetSkip(skip func(sourceKey string) bool)
}

// CollectionFactory builds a collection object whose membership references
// other stream objects by source key (the resolver maps them to final ids).
// Implementations are pure state builders — no store access.
type CollectionFactory interface {
	MakeCollection(name string, memberSourceKeys []string) (*Object, error)
}

// RootSpec describes the root collection and widget for a run. Zero value
// means: no root collection, no widget.
type RootSpec struct {
	// CollectionName names the root collection wrapping the run's root
	// candidates ("Markdown Import", "Notion Import"). Empty => no
	// collection is created.
	CollectionName string

	// RootObjectKey, when CollectionName is empty, points the widget at one
	// existing streamed object instead (markdown's single-directory-page
	// case). SourceKey form.
	RootObjectKey string

	WidgetLayout model.BlockContentWidgetLayout
}
