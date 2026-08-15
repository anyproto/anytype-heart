package adapter

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	objectcreator "github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// existsChecker probes the space index for an id (file dedup classification).
type existsChecker struct {
	store spaceindex.Store
}

func (c *existsChecker) Exists(id string) bool {
	ids, _, err := c.store.QueryObjectIds(database.Query{Filters: []database.FilterRequest{{
		Condition:   model.BlockContentDataviewFilter_Equal,
		RelationKey: bundle.RelationKeyId,
		Value:       domain.String(id),
	}}})
	return err == nil && len(ids) > 0
}

// uploaderAdapter satisfies persist.FileUploader over the two real services.
type uploaderAdapter struct {
	blockService      *block.Service
	fileObjectService fileobject.Service
}

func (u *uploaderAdapter) UploadFile(ctx context.Context, spaceId string, req block.FileUploadRequest) (string, model.BlockContentFileType, *domain.Details, error) {
	return u.blockService.UploadFile(ctx, spaceId, req)
}

func (u *uploaderAdapter) CreateFromImport(fileId domain.FullFileId, origin objectorigin.ObjectOrigin, details *domain.Details) (string, error) {
	return u.fileObjectService.CreateFromImport(fileId, origin, details)
}

// installerAdapter narrows InstallBundledObjects onto the persist seam.
type installerAdapter struct {
	installer objectcreator.Service
	space     clientspace.Space
}

func (i *installerAdapter) InstallBundledObjects(ctx context.Context, ids []string) error {
	_, _, err := i.installer.InstallBundledObjects(ctx, i.space, ids)
	if err != nil {
		return fmt.Errorf("install bundled objects: %w", err)
	}
	return nil
}

// collectionFactory builds collection objects through the collection
// service's state template (pure state construction, no store writes).
type collectionFactory struct {
	service *collection.Service
	addDate bool
}

func (f *collectionFactory) MakeCollection(name string, memberSourceKeys []string) (*importv2.Object, error) {
	if f.addDate {
		name = fmt.Sprintf("%s %s", name, time.Now().Format("2006-01-02 15:04"))
	}
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	_, _, st, err := f.service.CreateCollection(details, []*model.InternalFlag{
		{Value: model.InternalFlag_collectionDontIndexLinks},
	})
	if err != nil {
		return nil, fmt.Errorf("create collection state: %w", err)
	}
	merged := st.CombinedDetails().Merge(details)
	merged.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_collection))
	st.UpdateStoreSlice(template.CollectionStoreKey, memberSourceKeys)
	return &importv2.Object{
		SourceKey: "collection:" + name,
		SbType:    coresb.SmartBlockTypePage,
		Payload: &importv2.Snapshot{
			Blocks:      st.Blocks(),
			Details:     merged,
			ObjectTypes: []string{bundle.TypeKeyCollection.String()},
			Collections: st.Store(),
		},
	}, nil
}

// teeReporter fans the seam out to every consumer of a run's progress: the
// legacy process scalar and the §15 statistic emitter. There is exactly one
// construction site (engineDeps), so a consumer cannot be wired into the
// fresh-run path and forgotten in the two resume ones.
type teeReporter []engine.Reporter

func (t teeReporter) Phase(p importv2.Phase) {
	for _, r := range t {
		r.Phase(p)
	}
}

func (t teeReporter) Discovered(kind importv2.Kind, delta int64) {
	for _, r := range t {
		r.Discovered(kind, delta)
	}
}

func (t teeReporter) Completed(kind importv2.Kind, delta int64) {
	for _, r := range t {
		r.Completed(kind, delta)
	}
}

func (t teeReporter) Bytes(delta int64) {
	for _, r := range t {
		r.Bytes(delta)
	}
}

func (t teeReporter) Created(count int64) {
	for _, r := range t {
		r.Created(count)
	}
}

func (t teeReporter) Item(item importv2.DisplayText) {
	for _, r := range t {
		r.Item(item)
	}
}

// progressReporter down-projects the engine's per-kind, per-phase counters
// onto the legacy wire scalar (one total, one done, one message). The legacy
// surface stays untouched by the §15 work — it is the compatibility path —
// so this projection reproduces exactly what the pre-§15 seam produced,
// with one repair the split made free: the denominator re-bases onto the
// spool census when materialization starts, for a fresh run and a resumed
// one alike (the resume path used to do that by hand in resumerun.go, the
// classic rule-in-one-sibling shape).
type progressReporter struct {
	progress process.Progress
	total    atomic.Int64
	// scanned gates the FIRST publish of the total. Pass 1 now discovers
	// claims one at a time (the SCANNING count-up), and
	// SetTotalPreservingRatio is not idempotent once done is non-zero — on a
	// multi-path markdown request, where path 2 starts with path 1's done
	// already counted, publishing per claim would run its ratio arithmetic
	// thousands of times. So pass 1's count reaches the scalar once, at the
	// pass-1/pass-2 boundary, exactly as the single AddTotal(count) did.
	scanned atomic.Bool
	// materializing gates the DONE counter onto pass 3. The legacy scalar
	// has one bar: pass 2's spooling counted into it would fill it once and
	// then pass 3 would fill it again.
	materializing atomic.Bool
}

func (r *progressReporter) Phase(p importv2.Phase) {
	r.progress.SetProgressMessage(p.String())
	if p == importv2.PhaseCreating {
		r.materializing.Store(true)
		r.total.Store(0) // re-based by the census Discovered calls that follow
		return
	}
	if p > importv2.PhaseScanning && r.scanned.CompareAndSwap(false, true) {
		r.progress.SetTotalPreservingRatio(r.total.Load())
	}
}

func (r *progressReporter) Discovered(kind importv2.Kind, delta int64) {
	total := r.total.Add(delta)
	if r.scanned.Load() {
		// Late claims during pass 2 and the pass-3 census publish
		// immediately, as AddTotal always did; pass 1 accumulates silently.
		r.progress.SetTotalPreservingRatio(total)
	}
}

func (r *progressReporter) Completed(kind importv2.Kind, delta int64) {
	if !r.materializing.Load() {
		return
	}
	r.progress.AddDone(delta)
}

// The legacy scalar carries none of these: bytes and the created level are
// new with §15, and currentItem is user content that has no place in a
// process message.
func (r *progressReporter) Bytes(int64)               {}
func (r *progressReporter) Created(int64)             {}
func (r *progressReporter) Item(importv2.DisplayText) {}
