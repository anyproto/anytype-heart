package fileoffloader

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "core.files.fileoffloader"

var log = logging.Logger(CName).Desugar()

type Service interface {
	app.Component

	FileOffload(ctx context.Context, objectId string, includeNotPinned bool) (totalSize uint64, err error)
	FilesOffload(ctx context.Context, objectIds []string, includeNotPinned bool) (err error)
	FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error)
	FileOffloadRaw(ctx context.Context, id domain.FullFileId) (totalSize uint64, err error)
}

type service struct {
	objectStore objectstore.ObjectStore
	fileStore   filestore.FileStore
	dagService  ipld.DAGService
	commonFile  fileservice.FileService
	fileStorage filestorage.FileStorage
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.dagService = s.commonFile.DAGService()
	s.fileStorage = app.MustComponent[filestorage.FileStorage](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) FileOffload(ctx context.Context, objectId string, includeNotPinned bool) (totalSize uint64, err error) {
	details, err := s.objectStore.GetDetails(objectId)
	if err != nil {
		return 0, fmt.Errorf("get object details: %w", err)
	}
	return s.fileOffload(ctx, details.GetDetails(), includeNotPinned)
}

func (s *service) fileOffload(ctx context.Context, fileDetails *types.Struct, includeNotPinned bool) (uint64, error) {
	fileId := pbtypes.GetString(fileDetails, bundle.RelationKeyFileId.String())
	if fileId == "" {
		return 0, fmt.Errorf("empty file id")
	}
	backupStatus := filesyncstatus.Status(pbtypes.GetInt64(fileDetails, bundle.RelationKeyFileBackupStatus.String()))
	id := domain.FullFileId{
		SpaceId: pbtypes.GetString(fileDetails, bundle.RelationKeySpaceId.String()),
		FileId:  domain.FileId(fileId),
	}

	if !includeNotPinned && backupStatus != filesyncstatus.Synced {
		return 0, nil
	}

	return s.FileOffloadRaw(ctx, id)
}

func (s *service) FilesOffload(ctx context.Context, objectIds []string, includeNotPinned bool) (err error) {
	if len(objectIds) == 0 {
		return s.offloadAllFiles(ctx, includeNotPinned)
	}

	for _, objectId := range objectIds {
		_, err := s.FileOffload(ctx, objectId, includeNotPinned)
		if err != nil {
			log.Error("failed to offload file", zap.String("objectId", objectId), zap.Error(err))
			continue
		}
	}
	return nil
}

func (s *service) offloadAllFiles(ctx context.Context, includeNotPinned bool) (err error) {
	gc := s.fileStorage.NewLocalStoreGarbageCollector()

	if !includeNotPinned {
		records, err := s.objectStore.Query(database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyFileId.String(),
					Condition:   model.BlockContentDataviewFilter_NotEmpty,
				},
				{
					RelationKey: bundle.RelationKeyFileBackupStatus.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.Int64(int64(filesyncstatus.Synced)),
				},
			},
		})
		if err != nil {
			return fmt.Errorf("query not pinned files: %w", err)
		}

		for _, record := range records {
			fileId := domain.FullFileId{
				SpaceId: pbtypes.GetString(record.Details, bundle.RelationKeySpaceId.String()),
				FileId:  domain.FileId(pbtypes.GetString(record.Details, bundle.RelationKeyFileId.String())),
			}
			_, cids, err := s.getAllExistingFileBlocksCids(ctx, fileId)
			if err != nil {
				return fmt.Errorf("not pinned file: collect cids: %w", err)
			}
			gc.MarkAsUsing(cids)
		}
	}

	err = gc.CollectGarbage(ctx)
	return err
}

func (s *service) FileSpaceOffload(ctx context.Context, spaceId string, includeNotPinned bool) (filesOffloaded int, totalSize uint64, err error) {
	records, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("query file objects by spaceId: %w", err)
	}
	for _, record := range records {
		fileId := pbtypes.GetString(record.Details, bundle.RelationKeyFileId.String())
		size, err := s.offloadFileSafe(ctx, spaceId, fileId, record, includeNotPinned)
		if err != nil {
			log.Error("FileSpaceOffload: failed to offload file", zap.String("fileId", fileId), zap.Error(err))
			return 0, 0, err
		}
		if size > 0 {
			filesOffloaded++
			err = s.fileStore.DeleteFile(domain.FileId(fileId))
			if err != nil {
				return 0, 0, fmt.Errorf("failed to delete file from store: %w", err)
			}
		}
		totalSize += size
	}
	return filesOffloaded, totalSize, nil
}

func (s *service) offloadFileSafe(ctx context.Context,
	spaceId string,
	fileId string,
	record database.Record,
	includeNotPinned bool,
) (uint64, error) {
	existingObjects, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(fileId),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		return 0, err
	}
	if len(existingObjects) > 0 {
		return s.fileOffload(ctx, record.Details, false)
	}
	return s.fileOffload(ctx, record.Details, includeNotPinned)
}

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

func (s *service) FileOffloadRaw(ctx context.Context, id domain.FullFileId) (totalSize uint64, err error) {
	totalSize, cids, err := s.getAllExistingFileBlocksCids(ctx, id)
	if err != nil {
		return 0, err
	}

	dagService := s.dagServiceForSpace(id.SpaceId)
	for _, c := range cids {
		err = dagService.Remove(context.Background(), c)
		if err != nil {
			// no need to check for cid not exists
			return 0, err
		}
	}

	return totalSize, nil
}

func (s *service) getAllExistingFileBlocksCids(ctx context.Context, id domain.FullFileId) (totalSize uint64, cids []cid.Cid, err error) {
	var getCidsLinksRecursively func(c cid.Cid) (err error)
	dagService := s.dagServiceForSpace(id.SpaceId)

	var visitedMap = make(map[string]struct{})
	getCidsLinksRecursively = func(c cid.Cid) (err error) {
		if exists, err := s.hasCid(ctx, id.SpaceId, c); err != nil {
			return err
		} else if !exists {
			// double-check the blockstore, if we don't have the block - we have not yet downloaded it
			// otherwise format.GetLinks will do bitswap
			return nil
		}
		cids = append(cids, c)

		// here we can be sure that the block is loaded to the blockstore, so 1s should be more than enough
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		ctx = context.WithValue(ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
		n, err := dagService.Get(ctx, c)
		if err != nil {
			log.Error("GetAllExistingFileBlocksCids: failed to get links", zap.Error(err))
		}
		cancel()
		if n != nil {
			// use rawData because Size() includes size of inner links which may be not loaded
			totalSize += uint64(len(n.RawData()))
		}
		if n == nil || len(n.Links()) == 0 {
			return nil
		}
		for _, link := range n.Links() {
			if _, visited := visitedMap[link.Cid.String()]; visited {
				continue
			}
			visitedMap[link.Cid.String()] = struct{}{}
			err := getCidsLinksRecursively(link.Cid)
			if err != nil {
				return err
			}
		}

		return
	}

	c, err := cid.Parse(id.FileId.String())
	if err != nil {
		return 0, nil, err
	}

	err = getCidsLinksRecursively(c)

	return
}

func (s *service) hasCid(ctx context.Context, spaceID string, c cid.Cid) (bool, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
	return s.commonFile.HasCid(cctx, c)
}
