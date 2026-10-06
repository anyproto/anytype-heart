package v2service

// search_stream.go is the service half of the search stream
// (POST /v2/spaces/{space_id}/search/stream): it admits a client (space and
// grant first, then the request, compiled exactly as POST …/search compiles
// it, then the stream cap) onto a live search of the space
// (apicore.ObjectSearchService) and renders its changes into the v2
// envelope with the search row renderer.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"go.uber.org/zap"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

// maxConcurrentSearchStreams caps open search streams process-wide,
// independently of the chat stream caps: an agent holding many chat streams
// cannot starve discovery, and the reverse.
const maxConcurrentSearchStreams = 16

// StreamFrame is one rendered Server-Sent Event: the event name and its JSON
// body.
type StreamFrame struct {
	Type string
	Data []byte
}

// SearchStream is one open search stream: the live search plus the stream
// slot. Snapshot and Drain render with the search row renderer; neither is
// safe for concurrent use. Close is safe to call more than once.
type SearchStream struct {
	sub      apicore.ObjectSearchSubscription
	render   *searchStreamRenderer
	snapshot []StreamFrame

	release   func()
	closeOnce sync.Once
}

// Snapshot is the opening snapshot: an object_added frame per matching
// object, then snapshot_complete.
func (s *SearchStream) Snapshot() []StreamFrame { return s.snapshot }

// Ready is signalled whenever live changes are queued.
func (s *SearchStream) Ready() <-chan struct{} { return s.sub.Ready() }

// Drain renders and returns every queued live change, oldest first.
func (s *SearchStream) Drain() []StreamFrame {
	changes := s.sub.Drain()
	if len(changes) == 0 {
		return nil
	}
	return s.render.frames(changes, nil)
}

// Close ends the live search and returns the stream slot.
func (s *SearchStream) Close() {
	s.closeOnce.Do(func() {
		s.sub.Close()
		s.release()
	})
}

// OpenSearchStream opens the search stream. The space and the key's grant
// are checked, and the request compiled, before anything subscribes; every
// refusal is a C6 error, returned before the stream's first byte.
func (s *Service) OpenSearchStream(ctx context.Context, spaceId string, req v2model.SearchRequest) (*SearchStream, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, err
	}
	if req.Query != "" {
		return nil, v2model.ValidationFailed("the search stream takes no full-text query",
			v2model.Issue{Path: "/query", Message: "a live search follows filters only; full-text matching is not tracked live"}.
				Hintf("drop query and narrow with filter, or run the full-text search once with %s", v2model.RefSearchSpace(spaceId)))
	}
	plan, err := s.planSpaceSearch(ctx, spaceId, req)
	if err != nil {
		return nil, err
	}
	if s.objectSearch == nil {
		return nil, fmt.Errorf("open search stream in %s: live search is not configured", spaceId)
	}
	render, err := s.newSearchStreamRenderer(spaceId, req.Fields)
	if err != nil {
		return nil, err
	}
	release, ok := s.searchStreams.acquire(maxConcurrentSearchStreams)
	if !ok {
		return nil, v2model.NewError(http.StatusTooManyRequests, v2model.CodeTooManyStreams,
			fmt.Sprintf("this process already holds %d open search streams", maxConcurrentSearchStreams),
			v2model.Issue{
				Message: "the cap is on streams held at once, not on how fast they are opened, so retrying the same request cannot succeed",
				Hint:    "close a search stream you no longer read before opening another",
			})
	}
	// the slot and, once opened, the search are released here unless the
	// stream took them: a panic below must not burn the slot for the life of
	// the process, and the handler's deferred Close does not exist yet
	var sub apicore.ObjectSearchSubscription
	handedOff := false
	defer func() {
		if handedOff {
			return
		}
		if sub != nil {
			sub.Close()
		}
		release()
	}()

	sub, err = s.objectSearch.OpenObjectSearch(ctx, apicore.ObjectSearchOpen{
		SpaceId: spaceId,
		Filters: plan.filters,
		Sorts:   plan.sorts,
		Keys:    render.storeKeys(),
	})
	if err != nil {
		return nil, fmt.Errorf("open object search in %s: %w", spaceId, err)
	}
	stream := &SearchStream{sub: sub, render: render, release: release}
	stream.snapshot = render.frames(sub.Snapshot(), plan.warnings)
	handedOff = true
	return stream, nil
}

// planSpaceSearch compiles a space search request: the one validation and
// canonicalisation path of POST …/search and the search stream. The space
// was checked by the caller.
func (s *Service) planSpaceSearch(ctx context.Context, spaceId string, req v2model.SearchRequest) (*searchPlan, error) {
	if err := validateSearchShape(req); err != nil {
		return nil, err
	}
	return s.buildSearchPlan(spaceId, req, true, errKeysFor(ctx))
}

// searchStreamRenderer renders live search changes as search rows. A stream
// lives long, so the row builder is rebuilt for every batch: a type or an
// option created after the open renders like any other.
type searchStreamRenderer struct {
	s       *Service
	spaceId string
	fields  []string
	builder *objectRowBuilder
	// fresh is set while the builder is the one the open made, so the
	// opening snapshot does not build it twice
	fresh bool
}

func (s *Service) newSearchStreamRenderer(spaceId string, fields []string) (*searchStreamRenderer, error) {
	builder, err := s.newSearchRowBuilder(spaceId, fields)
	if err != nil {
		return nil, err
	}
	return &searchStreamRenderer{s: s, spaceId: spaceId, fields: fields, builder: builder, fresh: true}, nil
}

// storeKeys are the details the rows read: each requested field under the
// stored key it canonicalizes to (and its own spelling, which the builder
// tries first). The live search carries the base row's keys itself.
func (r *searchStreamRenderer) storeKeys() []string {
	var keys []string
	for _, field := range r.fields {
		keys = appendMissing(keys, field)
		if backing, ok := r.builder.aliases[field]; ok {
			keys = appendMissing(keys, string(backing))
		}
	}
	return keys
}

// frames renders one batch; warnings ride its snapshot_complete. A change
// that cannot be rendered is logged and dropped: the stream never ends on
// its own.
func (r *searchStreamRenderer) frames(changes []apicore.ObjectSearchChange, warnings []v2model.Issue) []StreamFrame {
	if r.fresh {
		r.fresh = false
	} else if builder, err := r.s.newSearchRowBuilder(r.spaceId, r.fields); err == nil {
		r.builder = builder
	} else {
		log.Warn("v2 search stream: rebuild the row builder, keeping the previous one",
			zap.String("spaceId", r.spaceId), zap.Error(err))
	}
	out := make([]StreamFrame, 0, len(changes))
	for _, change := range changes {
		event := v2model.SearchStreamEvent{SpaceId: r.spaceId}
		switch change.Type {
		case apicore.ObjectSearchAdded, apicore.ObjectSearchUpdated:
			event.Type = v2model.SearchStreamEventObjectAdded
			if change.Type == apicore.ObjectSearchUpdated {
				event.Type = v2model.SearchStreamEventObjectUpdated
			}
			row := r.builder.row(database.Record{Details: change.Details})
			event.Object = &row
		case apicore.ObjectSearchRemoved:
			event.Type = v2model.SearchStreamEventObjectRemoved
			event.ObjectId = change.Id
		case apicore.ObjectSearchSnapshotComplete:
			event.Type = v2model.SearchStreamEventSnapshotComplete
			event.Warnings = warnings
		default:
			log.Warn("v2 search stream: unknown change type, dropping it", zap.Uint8("type", uint8(change.Type)))
			continue
		}
		data, err := json.Marshal(event)
		if err != nil {
			log.Warn("v2 search stream: marshal event, dropping it",
				zap.String("type", event.Type), zap.String("objectId", change.Id), zap.Error(err))
			continue
		}
		out = append(out, StreamFrame{Type: event.Type, Data: data})
	}
	return out
}
