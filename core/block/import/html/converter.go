package html

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const numberOfStages = 2 // 1 cycle to get snapshots and 1 cycle to create objects
const (
	Name               = "Html"
	rootCollectionName = "HTML Import"
)

var log = logging.Logger("import-html")

type HTML struct {
	collectionService *collection.Service
	tempDirProvider   core.TempDirProvider
}

func New(collectionService *collection.Service, tempDirProvider core.TempDirProvider) converter.Converter {
	return &HTML{
		collectionService: collectionService,
		tempDirProvider:   tempDirProvider,
	}
}

func (h *HTML) Name() string {
	return Name
}

func (h *HTML) GetParams(req *pb.RpcObjectImportRequest) []string {
	if p := req.GetHtmlParams(); p != nil {
		return p.Path
	}

	return nil
}

func (h *HTML) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, importID string) (*converter.Response, *converter.ConvertError) {
	path := h.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	cErr := converter.NewError()
	snapshots, targetObjects, cancelError := h.getSnapshots(req, progress, path, cErr, importID)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if h.shouldReturnError(req, cErr, path) {
		return nil, cErr
	}

	rootCollection := converter.NewRootCollection(h.collectionService)
	rootCol, err := rootCollection.MakeRootCollection(rootCollectionName, targetObjects)
	if err != nil {
		cErr.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, cErr
		}
	}
	if rootCol != nil {
		snapshots = append(snapshots, rootCol)
	}

	progress.SetTotal(int64(numberOfStages * len(snapshots)))
	if cErr.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}, nil
	}

	return &converter.Response{
		Snapshots: snapshots,
	}, cErr
}

func (h *HTML) shouldReturnError(req *pb.RpcObjectImportRequest, cErr *converter.ConvertError, path []string) bool {
	return (!cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) ||
		(cErr.IsNoObjectToImportError(len(path)))
}

func (h *HTML) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, path []string, cErr *converter.ConvertError, importID string) ([]*converter.Snapshot, []string, *converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			return nil, nil, converter.NewCancelError(err)
		}
		sn, to, err := h.handleImportPath(p, req.GetMode(), importID)
		if err != nil {
			cErr.Add(err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil
			}
			continue
		}
		snapshots = append(snapshots, sn...)
		targetObjects = append(targetObjects, to...)
	}
	return snapshots, targetObjects, nil
}

func (h *HTML) handleImportPath(path string, mode pb.RpcObjectImportRequestMode, importID string) ([]*converter.Snapshot, []string, error) {
	importSource := source.GetSource(path)
	if importSource == nil {
		return nil, nil, fmt.Errorf("failed to identify source: %s", path)
	}
	defer importSource.Close()
	supportedExtensions := []string{".html"}
	imageFormats := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	videoFormats := []string{".mp4", ".m4v", ".mov"}
	audioFormats := []string{".mp3", ".ogg", ".wav", ".m4a", ".flac"}
	pdf := []string{".pdf"}

	supportedExtensions = append(supportedExtensions, videoFormats...)
	supportedExtensions = append(supportedExtensions, imageFormats...)
	supportedExtensions = append(supportedExtensions, audioFormats...)
	supportedExtensions = append(supportedExtensions, pdf...)
	readers, err := importSource.GetFileReaders(path, supportedExtensions, nil)
	if err != nil {
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, err
		}
	}
	if source.CountFilesWithGivenExtension(readers, ".html") == 0 {
		return nil, nil, converter.ErrNoObjectsToImport
	}
	snapshots := make([]*converter.Snapshot, 0, len(readers))
	targetObjects := make([]string, 0, len(readers))
	for name, rc := range readers {
		if filepath.Ext(name) != ".html" {
			continue
		}
		blocks, err := h.getBlocksForSnapshot(rc, readers, path)
		if err != nil {
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, err
			}
			continue
		}
		sn, id := h.getSnapshot(blocks, name, importID)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
	}
	return snapshots, targetObjects, nil
}

func (h *HTML) getBlocksForSnapshot(rc io.ReadCloser, files map[string]io.ReadCloser, path string) ([]*model.Block, error) {
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	blocks, _, err := anymark.HTMLToBlocks(b)
	for _, block := range blocks {
		if block.GetFile() != nil {
			if newFileName, _, err := converter.ProvideFileName(block.GetFile().GetName(), files, path, h.tempDirProvider); err == nil {
				block.GetFile().Name = newFileName
			} else {
				log.Errorf("failed to update file block with new file name: %v", oserror.TransformError(err))
			}
		}
		if block.GetText() != nil && block.GetText().Marks != nil && len(block.GetText().Marks.Marks) > 0 {
			h.updateFilesInLinks(block, files, path)
		}
	}
	return blocks, nil
}

func (h *HTML) updateFilesInLinks(block *model.Block, files map[string]io.ReadCloser, path string) {
	marks := block.GetText().GetMarks().GetMarks()
	for _, mark := range marks {
		if mark.Type == model.BlockContentTextMark_Link {
			var (
				err             error
				newFileName     string
				createFileBlock bool
			)
			if newFileName, createFileBlock, err = converter.ProvideFileName(mark.Param, files, path, h.tempDirProvider); err == nil {
				mark.Param = newFileName
				if createFileBlock {
					anymark.ConvertTextToFile(block)
					break
				}
				continue
			}
			log.Errorf("failed to update link block with new file name: %v", oserror.TransformError(err))
		}
	}
}

func (h *HTML) getSnapshot(blocks []*model.Block, p string, importID string) (*converter.Snapshot, string) {
	sn := &model.SmartBlockSnapshotBase{
		Blocks:      blocks,
		Details:     h.provideDetails(p, importID),
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
	}

	snapshot := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: p,
		Snapshot: &pb.ChangeSnapshot{Data: sn},
		SbType:   smartblock.SmartBlockTypePage,
	}
	return snapshot, snapshot.Id
}

func (h *HTML) provideDetails(p string, importID string) *types.Struct {
	details := converter.GetCommonDetails(p, "", "", model.ObjectType_basic)
	details.Fields[bundle.RelationKeyImportID.String()] = pbtypes.String(importID)
	return details
}
