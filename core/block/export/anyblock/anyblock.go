// Package anyblock is the native AnyBlock JSON exporter: it writes a bundle
// (SPEC.md §2c) from a live space, wiring store, cache and writer around the
// shared composition (pkg/lib/anyblockjson/compose) on top of the extracted
// collection seam (core/block/export/collect).
//
// The pipeline is the designed collect → plan → emit → finish
// (EXPORTER_DESIGN.md §1.1): collection returns the complete doc set before
// anything is written; the plan fixes every path single-threaded from
// details alone (a pure per-id function, §1.3 — no collision machinery);
// emit runs width-bounded concurrent tasks that load one object each,
// decide omission through the package predicates, marshal, write, stream
// the blob for file objects, and close the object out of the cache; finish
// writes properties.json and index.json, re-read-verified (I1 at bundle
// scope).
//
// The RPC surface (a new model.ExportFormat value) is deliberately not
// here — design Q6, follow-up — so this package is driven through the Go
// API by tests and cmd tooling until the enum lands.
package anyblock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	sb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/export/collect"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/compose"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
)

var log = logging.Logger("anytype-mw-export-anyblock")

// Writer is where the bundle's files land. The legacy exporter's dir and
// zip writers satisfy it structurally; DirWriter (writer.go) is the
// package's own deterministic directory form.
//
// One determinism caveat belongs to the writer, not this package: a DIR
// tree from the same space state is byte-identical file for file (the
// determinism test pins it), but a ZIP archive additionally encodes entry
// ORDER, and emit is concurrent — archive-level byte identity would need an
// ordered writer, which nothing requires yet.
type Writer interface {
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) error
}

// Request describes one space's bundle export.
type Request struct {
	SpaceId string
	// Ids are the requested roots; empty = the whole space.
	Ids []string

	IncludeNested    bool
	IncludeFiles     bool
	IncludeArchived  bool
	IncludeBacklinks bool
	IncludeSpace     bool

	// SpaceName is index.json's fallback name, used only when the space's
	// own settings document states none (§2c).
	SpaceName string

	// BundleRoot is a path prefix inside the writer — empty for a
	// single-space export (the bundle root IS the writer root), and
	// "spaces/<spaceId>" per bundle when a caller exports several spaces
	// into one archive (design Q9: the wrapper is load-bearing — the same
	// id legitimately recurs across spaces, so flattening collides).
	// Manifest paths stay bundle-relative either way.
	BundleRoot string

	// StateFilters filter the state of objects collected as links, exactly
	// as the legacy exporter's LinksStateFilters do.
	StateFilters *state.Filters
}

// Exporter wires the pipeline's dependencies. Construct it with the same
// components the legacy export service holds; every field is required
// except none.
type Exporter struct {
	Collector collect.Collector
	// Picker is typed CachedObjectGetter, not ObjectGetter, ON PURPOSE:
	// close-after-write is the memory model (design §1.5/§1.6 — without it
	// the export retains throughput × cache TTL of loaded trees), so a
	// picker that cannot close is a compile error here, never a silent
	// degradation behind a failed type assertion.
	Picker      cache.CachedObjectGetter
	ObjectStore objectstore.ObjectStore
	SbtProvider typeprovider.SmartBlockTypeProvider
}

// emitWidthFor bounds the emit phase's concurrency, following the repo's
// prior art for exactly this problem — the reindex limiter
// (core/indexer/reindexlimiter.go): each task cold-builds one object into
// the space's object cache, work is storage-read-bound, so a small overlap
// keeps throughput while capping the peak; mobile gets half the slots.
// With close-after-write (below) the width IS the resident content set:
// at most this many export-loaded trees exist at any instant (design §1.6).
func emitWidthFor(goos string) int {
	if goos == "ios" || goos == "android" {
		return 2
	}
	return 4
}

// Result is what one export can say about itself — nothing a caller might
// act on is dropped into a log line alone.
type Result struct {
	// Succeed counts the documents accounted for: written, or omitted into
	// the bundle files.
	Succeed int
	// DocErrors counts documents that failed to emit at all (load, marshal
	// or write) — logged and skipped, the legacy exporter's own per-doc
	// discipline, but COUNTED, so a caller can tell a clean backup from a
	// holed one.
	DocErrors int
	// BlobErrors counts file objects whose DOCUMENT was written but whose
	// bytes could not be streamed (node offline, blocks missing). The
	// document still travels — metadata is strictly more than the nothing a
	// failed doc leaves — and the manifest simply omits the binding, which
	// the bundle tooling then surfaces (a file document unbound by a
	// present map is a warning, §2c). A FAT bundle with BlobErrors > 0 is
	// not the faithful byte carrier it promises to be; the caller decides
	// whether that fails the operation.
	BlobErrors int
}

// Export writes one space's bundle through wr: every collected document at
// its planned path, blobs beside their file documents, then
// properties.json and index.json at the bundle root.
func (e *Exporter) Export(ctx context.Context, req Request, wr Writer) (res Result, err error) {
	docs, err := e.Collector.Collect(ctx, collect.Request{
		SpaceId:          req.SpaceId,
		Ids:              req.Ids,
		Closure:          collect.ClosureDerived,
		IncludeNested:    req.IncludeNested,
		IncludeFiles:     req.IncludeFiles,
		IncludeArchived:  req.IncludeArchived,
		IncludeBacklinks: req.IncludeBacklinks,
		IncludeSpace:     req.IncludeSpace,
		StateFilters:     req.StateFilters,
	})
	if err != nil {
		return res, fmt.Errorf("collect docs for export: %w", err)
	}

	// plan: details only, single-threaded, before the first emit task
	// (design §1.1). Excluded rows are dropped here by the same rule every
	// emitter applies.
	metas := make([]compose.DocMeta, 0, len(docs))
	emitIds := make([]string, 0, len(docs))
	for id, doc := range docs {
		if collect.Excluded(doc.Details) {
			continue
		}
		sbType, sbtErr := e.SbtProvider.Type(req.SpaceId, id)
		if sbtErr != nil {
			log.With("objectId", id).Errorf("failed to get smartblock type: %v", sbtErr)
			continue
		}
		metas = append(metas, compose.DocMeta{
			Id:       id,
			SbType:   sbType.ToProto(),
			FileExt:  doc.Details.GetString(bundle.RelationKeyFileExt),
			FileMime: doc.Details.GetString(bundle.RelationKeyFileMimeType),
		})
		emitIds = append(emitIds, id)
	}
	plan, err := compose.BuildPlan(req.SpaceId, metas)
	if err != nil {
		return res, fmt.Errorf("build path plan: %w", err)
	}

	// the composer gets a DEDICATED resolver set: storeresolver.Resolvers
	// is not safe for concurrent use, and the composer consults its options
	// only under its own mutex — sharing an instance with a worker would
	// race (compose.NewComposer's contract).
	composer := compose.NewComposer(storeresolver.New(e.ObjectStore.SpaceIndex(req.SpaceId)).Options(), req.SpaceName)

	// emit: width-bounded workers, each with its own resolver set. The
	// output cannot depend on scheduling: every path was fixed by the plan,
	// the composer's aggregates are commutative, and finish sorts (§1.5).
	var succeedAsync, docErrs, blobErrs int64
	width := emitWidthFor(runtime.GOOS)
	ids := make(chan string)
	var wg sync.WaitGroup
	for range width {
		wg.Add(1)
		go func() {
			defer wg.Done()
			opts := storeresolver.New(e.ObjectStore.SpaceIndex(req.SpaceId)).Options()
			for id := range ids {
				blobFailed, werr := e.emitDoc(ctx, req, docs, plan, composer, opts, wr, id)
				if blobFailed {
					atomic.AddInt64(&blobErrs, 1)
				}
				if werr != nil {
					log.With("objectID", id).Warnf("can't export doc: %v", werr)
					atomic.AddInt64(&docErrs, 1)
				} else {
					atomic.AddInt64(&succeedAsync, 1)
				}
			}
		}()
	}
	for _, id := range emitIds {
		ids <- id
	}
	close(ids)
	wg.Wait()
	res = Result{Succeed: int(succeedAsync), DocErrors: int(docErrs), BlobErrors: int(blobErrs)}
	if res.BlobErrors > 0 {
		log.Errorf("export %s: %d file blob(s) could not be streamed; their documents travel without bytes and the manifest omits the bindings", req.SpaceId, res.BlobErrors)
	}

	// finish: the two bundle files, re-read-verified by the composer (I1
	// at bundle scope). Nil bytes = nothing was written, nothing to state.
	index, properties, _, err := composer.Finish()
	if err != nil {
		return res, fmt.Errorf("compose bundle files: %w", err)
	}
	if properties != nil {
		if err := wr.WriteFile(path.Join(req.BundleRoot, anyblockjson.PropertiesFileName), bytes.NewReader(properties), 0); err != nil {
			return res, fmt.Errorf("write property dictionary: %w", err)
		}
	}
	if index != nil {
		if err := wr.WriteFile(path.Join(req.BundleRoot, anyblockjson.IndexFileName), bytes.NewReader(index), 0); err != nil {
			return res, fmt.Errorf("write index: %w", err)
		}
	}
	return res, nil
}

// emitDoc is one emit task: load, decide omission, marshal, write, stream
// the blob, observe — then close the object out of the cache. A blob
// stream failure does NOT fail the document: the document is already
// written and carries strictly more than the nothing a failed doc leaves,
// so the failure is reported through blobFailed (and the manifest omits
// the binding) rather than by undoing the doc.
func (e *Exporter) emitDoc(ctx context.Context, req Request, docs collect.Docs, plan *compose.Plan,
	composer *compose.Composer, opts anyblockjson.Options, wr Writer, id string) (blobFailed bool, _ error) {

	err := cache.Do(e.Picker, id, func(b sb.SmartBlock) error {
		st := b.NewState()
		if st.CombinedDetails().GetBool(bundle.RelationKeyIsDeleted) {
			return nil
		}
		st = st.Copy().Filter(stateFilters(req, docs, id))
		if isCollection(st) {
			collectionFilterMissing(st, docs)
		}

		sbType := b.Type().ToProto()
		// the same snapshot shape the pb converter feeds the wire
		// (core/converter/pbc), which is also the shape the corpus sweep
		// verified the codec against — 38k documents, 34 known failures
		base := &model.SmartBlockSnapshotBase{
			Blocks:      st.BlocksToSave(),
			Details:     st.CombinedDetails().ToProto(),
			ObjectTypes: domain.MarshalTypeKeys(st.ObjectTypeKeys()),
			Collections: st.Store(),
			Key:         st.UniqueKeyInternal(),
			FileInfo:    st.GetFileInfo().ToModel(),
		}

		// omission is decided HERE, on the loaded snapshot — the predicates
		// take the base, so they cannot run at plan time (design §1.1). An
		// omitted document's facts were lifted into the composer; a planned
		// name going unused does not disturb determinism, since omission is
		// itself a deterministic function of state. Issues mean the lift
		// failed to account for something — a bug worth logging loudly, but
		// the export carries on (the fail-closed predicates keep the
		// document in every doubtful case, so an issue here is belt over
		// braces).
		omitted, issues := composer.Observe(sbType, base)
		for _, is := range issues {
			log.With("objectID", id).Errorf("bundle composition %s: %s", is.Category, is.Detail)
		}
		if omitted {
			return nil
		}

		data, err := anyblockjson.Marshal(sbType, base, opts)
		if err != nil {
			return fmt.Errorf("marshal document: %w", err)
		}
		docPath, ok := plan.DocPath(id)
		if !ok {
			return fmt.Errorf("no planned path for %s", id)
		}
		lastModifiedDate := st.LocalDetails().GetInt64(bundle.RelationKeyLastModifiedDate)
		if err := wr.WriteFile(path.Join(req.BundleRoot, docPath), bytes.NewReader(data), lastModifiedDate); err != nil {
			return fmt.Errorf("write document: %w", err)
		}
		if err := composer.ObserveWritten(sbType, base, data, docPath); err != nil {
			return fmt.Errorf("observe written document: %w", err)
		}

		// the blob, adjacent to its document (§1.4): streamed, never
		// buffered, and bound by the manifest `files` map — the document
		// itself carries no path, and `source` keeps meaning what its
		// relation says it means (the legacy clobber does not carry over)
		if req.IncludeFiles && b.Type() == smartblock.SmartBlockTypeFileObject {
			blobPath, ok := plan.BlobPath(id)
			if !ok {
				return fmt.Errorf("no planned blob path for %s", id)
			}
			fullBlobPath := path.Join(req.BundleRoot, blobPath)
			if err := e.saveBlob(ctx, wr, b, fullBlobPath); err != nil {
				blobFailed = true
				log.With("objectID", id).Warnf("file blob not streamed, document travels without bytes: %v", err)
				// a PARTIAL blob is worse than none — truncated bytes a
				// reader may trust — so a writer that can un-write gets the
				// chance. A zip writer cannot (entries are streamed); there
				// the partial stays, unbound by the manifest, and the
				// tooling's orphan check flags it.
				if remover, ok := wr.(interface{ RemoveFile(string) error }); ok {
					if rerr := remover.RemoveFile(fullBlobPath); rerr != nil {
						log.With("objectID", id).Warnf("partial blob not removed: %v", rerr)
					}
				}
				return nil
			}
			composer.ObserveFileBlob(id, blobPath)
		}
		return nil
	})

	// Close after write — active, immediate, TTL-independent (design §1.5):
	// an object closes iff nobody else has it open, so the resident content
	// set stays ≈ the emit width instead of throughput × cache TTL, which
	// is the whole memory model (design §1.6). Log-only on failure, the
	// fulltext indexer's own discipline (core/indexer/fulltext.go:348) —
	// the passive TTL collects whatever this call could not.
	//
	// RELEASE GATE (design Q11): this path reaches the filed ocache
	// TryRemove-on-loading bug GO-7333 — nil-deref/hang/race when another
	// caller concurrently re-loads the same entry. The fix is any-sync PR
	// https://github.com/anyproto/any-sync/pull/769 (open, green CI): the
	// any-sync bump carrying it must land before this exporter ships. Our
	// own entry is loaded, not loading (cache.Do returned above), so the
	// window arms only on a concurrent re-load — the identical race the
	// fulltext indexer has soaked in production on every indexed object.
	if _, cerr := e.Picker.TryRemoveFromCache(ctx, id); cerr != nil {
		log.With("objectID", id).Warnf("object cache remove: %v", cerr)
	}
	return blobFailed, err
}

// saveBlob streams one file object's bytes to the writer — the legacy
// saveFile minus the two things this format deletes: the namer (the blob
// path is the plan's pure function of the id) and the Source-detail clobber
// (the manifest binds instead, §1.4).
func (e *Exporter) saveBlob(ctx context.Context, wr Writer, b sb.SmartBlock, blobPath string) error {
	fileObject, ok := b.(fileobject.FileObject)
	if !ok {
		return fmt.Errorf("object is not a file object")
	}
	file, err := fileObject.GetFile()
	if err != nil {
		return fmt.Errorf("get file: %w", err)
	}
	if strings.HasPrefix(file.MimeType(), "image") {
		image, err := fileObject.GetImage()
		if err != nil {
			return fmt.Errorf("get image: %w", err)
		}
		file, err = image.GetOriginalFile()
		if err != nil {
			return fmt.Errorf("get original file: %w", err)
		}
	}
	rd, err := file.Reader(ctx)
	if err != nil {
		return fmt.Errorf("open file reader: %w", err)
	}
	if err := wr.WriteFile(blobPath, rd, file.LastModifiedDate()); err != nil {
		return fmt.Errorf("write file blob: %w", err)
	}
	return nil
}

// stateFilters mirrors the legacy exporter's rule: only objects that
// entered the collection as LINKS render filtered.
func stateFilters(req Request, docs collect.Docs, id string) *state.Filters {
	if doc, ok := docs[id]; ok && doc.IsLink {
		return req.StateFilters
	}
	return nil
}

// collectionFilterMissing drops collection members the export does not
// carry, exactly as the legacy exporter does: a collection referencing an
// object outside the bundle would dangle on import.
func collectionFilterMissing(st *state.State, docs collect.Docs) {
	collectionIds := st.GetStoreSlice(template.CollectionStoreKey)
	existingIds := make([]string, 0, len(collectionIds))
	for _, item := range collectionIds {
		if _, exists := docs[item]; exists {
			existingIds = append(existingIds, item)
		}
	}
	if len(existingIds) != len(collectionIds) {
		st.UpdateStoreSlice(template.CollectionStoreKey, existingIds)
	}
}

func isCollection(st state.Doc) bool {
	return st.CombinedDetails().GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_collection)
}
