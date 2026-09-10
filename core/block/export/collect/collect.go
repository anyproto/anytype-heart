// Package collect declares the format-agnostic collection seam of the
// exporter: the dependency closure over an export request, complete before
// anything is written.
//
// Collection decides WHICH objects an export carries and holds details only —
// content loads later, per document, inside each writer's emit task. The
// closure MODE used to travel through the legacy exporter as a bare
// `isProtobuf bool`; here it is an explicit Closure, so a new format states
// which closure it wants instead of impersonating protobuf. The
// implementation lives with the legacy exporter (core/block/export), which
// exposes it through Collector; the native AnyBlock JSON writer consumes the
// interface and nothing behind it.
package collect

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// Closure names which dependency closure a collection runs.
type Closure int

const (
	// ClosureContent is the md-style closure: the requested objects, their
	// linked files, and — behind IncludeNested — the objects their content
	// links to. Nothing derived is pulled in.
	ClosureContent Closure = iota
	// ClosureDerived is the collect-everything-derived closure the snapshot
	// formats want: types, relations, relation options, templates, dataview
	// dependencies and recommended relations ride along, so the export
	// stands alone.
	ClosureDerived
)

// Request describes one collection run.
type Request struct {
	SpaceId string
	// Ids are the requested roots; empty means every exportable object of
	// the space.
	Ids     []string
	Closure Closure

	IncludeNested    bool
	IncludeFiles     bool
	IncludeArchived  bool
	IncludeBacklinks bool
	IncludeSpace     bool

	// StateFilters filter the state of objects that entered the set as
	// links (Doc.IsLink); the requested roots always render unfiltered.
	StateFilters *state.Filters
}

// Doc is one collected object: its details, and whether it entered the set
// as a link rather than as a requested root.
type Doc struct {
	Details *domain.Details
	IsLink  bool
}

// Docs is a collection result, keyed by object id.
type Docs map[string]*Doc

// TransformToDetailsMap re-wraps the collected details for converters that
// take a plain id → details map. Same pointers, not copies: the collection
// stays the single resident copy of the details (design §1.6).
func (d Docs) TransformToDetailsMap() map[string]*domain.Details {
	details := make(map[string]*domain.Details, len(d))
	for id, doc := range d {
		details[id] = doc.Details
	}
	return details
}

// Collector runs the requested closure and returns the complete set before
// anything is written.
type Collector interface {
	Collect(ctx context.Context, req Request) (Docs, error)
}

// Excluded reports a collected row no format should emit: empty or id-only
// details, an id-plus-backlinks tombstone, or a legacy raw file id. The
// collection may still hold such rows (they arrive through store queries);
// every emitter skips them by this one rule.
func Excluded(details *domain.Details) bool {
	if details == nil {
		return true
	}
	n := details.Len()
	// Empty details or containing only id
	if n <= 1 {
		return true
	}
	// Details only with id + backlinks should be discarded
	if n == 2 && details.Has(bundle.RelationKeyBacklinks) {
		return true
	}

	id := details.GetString(bundle.RelationKeyId)
	if domain.IsFileId(id) {
		return true
	}

	return false
}
