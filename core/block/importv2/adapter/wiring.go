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
	objectcreator "github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

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

// progressReporter down-projects the engine's rich progress onto the wire
// scalar (total/done + message).
type progressReporter struct {
	progress process.Progress
	total    atomic.Int64
}

func (r *progressReporter) Phase(name string) {
	r.progress.SetProgressMessage(name)
}

func (r *progressReporter) AddTotal(delta int64) {
	r.progress.SetTotalPreservingRatio(r.total.Add(delta))
}

func (r *progressReporter) Step(delta int64) {
	r.progress.AddDone(delta)
}
