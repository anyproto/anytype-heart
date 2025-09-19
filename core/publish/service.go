package publish

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/anytype-publish-server/publishclient"
	"github.com/anyproto/anytype-publish-server/publishclient/publishapi"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	jsonM = jsonpb.Marshaler{Indent: "  "}
)

const CName = "common.core.publishservice"

var log = logger.NewNamed(CName)

var ErrLimitExceeded = errors.New("limit exceeded")
var ErrUrlAlreadyTaken = errors.New("url is already taken by another page")

type PublishResult struct {
	Url string
}

type PublishingUberSnapshotMeta struct {
	SpaceId    string `json:"spaceId,omitempty"`
	RootPageId string `json:"rootPageId,omitempty"`
	InviteLink string `json:"inviteLink,omitempty"`
}

type Version struct {
	Heads     []string `json:"heads"`
	JoinSpace bool     `json:"joinSpace"`
}

// Contains all publishing .pb files
// and publishing meta info
type PublishingUberSnapshot struct {
	Meta PublishingUberSnapshotMeta `json:"meta,omitempty"`

	// A map of "dir/filename.pb -> jsonpb snapshot"
	PbFiles map[string]string `json:"pbFiles,omitempty"`
}

type Service interface {
	app.ComponentRunnable
	Publish(ctx context.Context, spaceId, pageObjId, uri string, joinSpace bool) (res PublishResult, err error)
	Unpublish(ctx context.Context, spaceId, pageObjId string) error
	PublishList(ctx context.Context, id string) ([]*pb.RpcPublishingPublishState, error)
	ResolveUri(ctx context.Context, uri string) (*pb.RpcPublishingPublishState, error)
	GetStatus(ctx context.Context, spaceId string, objectId string) (*pb.RpcPublishingPublishState, error)
}

type service struct {
	spaceService         space.Service
	exportService        export.Export
	publishClientService publishclient.Client
	identityService      identity.Service
	inviteService        inviteservice.InviteService
	objectStore          objectstore.ObjectStore
	tempDirService       core.TempDirProvider
	limitsConfig         config.PublishLimitsConfig
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.spaceService = app.MustComponent[space.Service](a)
	s.exportService = app.MustComponent[export.Export](a)
	s.publishClientService = app.MustComponent[publishclient.Client](a)
	s.identityService = app.MustComponent[identity.Service](a)
	s.inviteService = app.MustComponent[inviteservice.InviteService](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.tempDirService = app.MustComponent[core.TempDirProvider](a)
	s.limitsConfig = app.MustComponent[*config.Config](a).GetPublishLimits()
	return nil
}

func (s *service) Run(_ context.Context) error {
	return nil
}

func (s *service) Close(_ context.Context) error {
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func uniqName() string {
	return time.Now().Format("Anytype.WebPublish.20060102.150405.99")
}

func (s *service) exportToDir(ctx context.Context, spaceId, pageId string, includeSpaceInfo bool) (dirEntries []fs.DirEntry, exportPath string, err error) {
	tempDir := s.tempDirService.TempDir()
	exportPath, _, err = s.exportService.Export(ctx, pb.RpcObjectListExportRequest{
		SpaceId:          spaceId,
		Format:           model.Export_Protobuf,
		IncludeFiles:     true,
		IsJson:           false,
		Zip:              false,
		Path:             tempDir,
		ObjectIds:        []string{pageId},
		NoProgress:       true,
		IncludeNested:    true,
		IncludeBacklinks: true,
		IncludeSpace:     includeSpaceInfo,
		LinksStateFilters: &pb.RpcObjectListExportStateFilters{
			RelationsWhiteList: relationsWhiteListToPbModel(),
			RemoveBlocks:       true,
		},
	})
	if err != nil {
		return
	}

	dirEntries, err = os.ReadDir(exportPath)
	if err != nil {
		return
	}
	return
}

func (s *service) publishToPublishServer(ctx context.Context, spaceId, pageId, uri, globalName string, joinSpace bool) (err error) {
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	includeInviteLinkAndSpaceInfo := joinSpace && !spc.IsPersonal()
	dirEntries, exportPath, err := s.exportToDir(ctx, spaceId, pageId, includeInviteLinkAndSpaceInfo)
	if err != nil {
		return err
	}
	defer os.RemoveAll(exportPath)

	limit, err := s.getPublishLimit(globalName)
	if err != nil {
		return err
	}

	tempPublishDir := filepath.Join(s.tempDirService.TempDir(), uniqName())
	defer os.RemoveAll(tempPublishDir)

	if err := os.MkdirAll(tempPublishDir, 0777); err != nil {
		return err
	}

	uberSnapshot, totalSize, err := s.processExportedData(dirEntries, exportPath, tempPublishDir, limit, spaceId, pageId)
	if err != nil {
		return err
	}

	err = s.applyInviteLink(ctx, spaceId, &uberSnapshot, includeInviteLinkAndSpaceInfo)
	if err != nil {
		return err
	}
	if err := s.createIndexFile(tempPublishDir, uberSnapshot, totalSize, limit); err != nil {
		return err
	}

	version, err := s.evaluateDocumentVersion(ctx, spc, pageId, joinSpace)
	if err != nil {
		return err
	}

	if localPublishDir := os.Getenv("ANYTYPE_LOCAL_PUBLISH_DIR"); localPublishDir != "" {
		err := os.CopyFS(localPublishDir, os.DirFS(tempPublishDir))
		if err != nil {
			log.Error("publishing to local dir error", zap.Error(err))
			return err
		}
	} else {
		if err := s.publishToServer(ctx, spaceId, pageId, uri, version, tempPublishDir); err != nil {
			return err
		}
	}

	return nil
}

func (s *service) applyInviteLink(ctx context.Context, spaceId string, snapshot *PublishingUberSnapshot, includeInviteLink bool) error {
	if !includeInviteLink {
		return nil
	}
	inviteInfo, err := s.inviteService.GetCurrent(ctx, spaceId)
	if err != nil && errors.Is(err, inviteservice.ErrInviteNotExists) {
		return nil
	}
	if err != nil {
		return err
	}
	snapshot.Meta.InviteLink = fmt.Sprintf(s.limitsConfig.InviteLinkUrlTemplate, inviteInfo.InviteFileCid, inviteInfo.InviteFileKey)
	return nil
}

func (s *service) processExportedData(dirEntries []fs.DirEntry, exportPath, tempPublishDir string, limit int64, spaceId, pageId string) (PublishingUberSnapshot, int64, error) {
	uberSnapshot := PublishingUberSnapshot{
		Meta: PublishingUberSnapshotMeta{
			SpaceId:    spaceId,
			RootPageId: pageId,
		},
		PbFiles: make(map[string]string),
	}

	var totalSize int64
	for _, entry := range dirEntries {
		if entry.IsDir() {
			if size, err := s.processDirectory(entry, exportPath, tempPublishDir, &uberSnapshot, limit); err != nil {
				return PublishingUberSnapshot{}, 0, err
			} else {
				totalSize += size
			}
		} else {
			return PublishingUberSnapshot{}, 0, fmt.Errorf("unexpected file on export root level: %s", entry.Name())
		}
	}

	return uberSnapshot, totalSize, nil
}

func (s *service) processDirectory(entry fs.DirEntry, exportPath, tempPublishDir string, uberSnapshot *PublishingUberSnapshot, limit int64) (int64, error) {
	dirName := entry.Name()
	if dirName == export.Files {
		return s.processFilesDirectory(exportPath, tempPublishDir, limit)
	}

	dirFiles, err := os.ReadDir(filepath.Join(exportPath, dirName))
	if err != nil {
		return 0, err
	}

	for _, file := range dirFiles {
		if err := s.processSnapshotFile(exportPath, dirName, file, uberSnapshot); err != nil {
			return 0, err
		}
	}

	return 0, nil
}

func (s *service) processFilesDirectory(exportPath, tempPublishDir string, limit int64) (int64, error) {
	var size int64
	originalPath := filepath.Join(exportPath, export.Files)
	err := filepath.Walk(originalPath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			size += info.Size()
			if size > limit {
				return ErrLimitExceeded
			}
		}
		return nil
	})
	if err != nil {
		return size, err
	}
	fileDir := filepath.Join(tempPublishDir, export.Files)
	if err := os.CopyFS(fileDir, os.DirFS(originalPath)); err != nil {
		return size, err
	}
	return size, nil
}

func (s *service) processSnapshotFile(exportPath, dirName string, file fs.DirEntry, uberSnapshot *PublishingUberSnapshot) error {
	withDirName := filepath.Join(dirName, file.Name())
	snapshotData, err := os.ReadFile(filepath.Join(exportPath, withDirName))
	if err != nil {
		return err
	}

	snapshot := pb.SnapshotWithType{}
	if err := proto.Unmarshal(snapshotData, &snapshot); err != nil {
		return err
	}

	details := snapshot.GetSnapshot().GetData().GetDetails()
	if source := pbtypes.GetString(details, bundle.RelationKeySource.String()); source != "" {
		source = filepath.ToSlash(source)
		details.Fields[bundle.RelationKeySource.String()] = pbtypes.String(source)
	}
	jsonData, err := jsonM.MarshalToString(&snapshot)
	if err != nil {
		return err
	}
	fileNameKey := fmt.Sprintf("%s/%s", dirName, file.Name())
	uberSnapshot.PbFiles[fileNameKey] = jsonData
	return nil
}

func (s *service) createIndexFile(tempPublishDir string, uberSnapshot PublishingUberSnapshot, totalSize int64, limit int64) error {
	jsonData, err := json.Marshal(&uberSnapshot)
	if err != nil {
		return err
	}

	outputFile := filepath.Join(tempPublishDir, s.limitsConfig.IndexFileName)
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := gzip.NewWriter(file)
	if _, err := writer.Write(jsonData); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	totalSize += stat.Size()
	if totalSize > limit {
		return ErrLimitExceeded
	}

	return nil
}

func (s *service) publishToServer(ctx context.Context, spaceId, pageId, uri, version, tempPublishDir string) error {
	publishReq := &publishapi.PublishRequest{
		SpaceId:  spaceId,
		ObjectId: pageId,
		Uri:      uri,
		Version:  version,
	}

	uploadUrl, err := s.publishClientService.Publish(ctx, publishReq)
	if err != nil {
		if errors.Is(err, publishapi.ErrUriNotUnique) {
			return ErrUrlAlreadyTaken
		}

		return err
	}

	if err := s.publishClientService.UploadDir(ctx, uploadUrl, tempPublishDir); err != nil {
		return err
	}

	return nil
}

func (s *service) evaluateDocumentVersion(ctx context.Context, spc clientspace.Space, pageId string, joinSpace bool) (string, error) {
	treeStorage, err := spc.Storage().TreeStorage(ctx, pageId)
	if err != nil {
		return "", err
	}
	heads, err := treeStorage.Heads(ctx)
	if err != nil {
		return "", err
	}
	slices.Sort(heads)
	h := &Version{Heads: heads, JoinSpace: joinSpace}
	jsonData, err := json.Marshal(h)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func (s *service) getPublishLimit(globalName string) (int64, error) {
	if globalName != "" {
		return s.limitsConfig.MembershipLimit, nil
	}
	return s.limitsConfig.DefaultLimit, nil
}

func (s *service) Publish(ctx context.Context, spaceId, pageId, uri string, joinSpace bool) (res PublishResult, err error) {
	identity, _, details := s.identityService.GetMyProfileDetails(ctx)
	globalName := details.GetString(bundle.RelationKeyGlobalName)

	err = s.publishToPublishServer(ctx, spaceId, pageId, uri, globalName, joinSpace)

	if err != nil {
		log.Error("Failed to publish", zap.Error(err))
		return
	}
	url := s.makeUrl(uri, identity, globalName)

	return PublishResult{Url: url}, nil
}

func (s *service) makeUrl(uri, identity, globalName string) string {
	var domain string
	if globalName != "" {
		domain = fmt.Sprintf(s.limitsConfig.MemberUrlTemplate, globalName)
	} else {
		domain = fmt.Sprintf(s.limitsConfig.DefaultUrlTemplate, identity)
	}
	url := fmt.Sprintf("%s/%s", domain, uri)
	return url
}

func (s *service) Unpublish(ctx context.Context, spaceId, pageObjId string) error {
	return s.publishClientService.UnPublish(ctx, &publishapi.UnPublishRequest{
		SpaceId:  spaceId,
		ObjectId: pageObjId,
	})
}

func (s *service) PublishList(ctx context.Context, spaceId string) ([]*pb.RpcPublishingPublishState, error) {
	publishes, err := s.publishClientService.ListPublishes(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	pbPublishes := make([]*pb.RpcPublishingPublishState, 0, len(publishes))
	for _, publish := range publishes {
		version := s.retrieveVersion(publish)
		details := s.retrieveObjectDetails(publish)
		pbPublishes = append(pbPublishes, &pb.RpcPublishingPublishState{
			SpaceId:   publish.SpaceId,
			ObjectId:  publish.ObjectId,
			Uri:       publish.Uri,
			Status:    pb.RpcPublishingPublishStatus(publish.Status),
			Version:   publish.Version,
			Timestamp: publish.Timestamp,
			Size_:     publish.Size,
			JoinSpace: version.JoinSpace,
			Details:   details,
		})
	}
	return pbPublishes, nil
}

func (s *service) retrieveObjectDetails(publish *publishapi.Publish) *types.Struct {
	records, err := s.objectStore.SpaceIndex(publish.SpaceId).QueryByIds([]string{publish.ObjectId})
	if err != nil {
		log.Error("failed to extract object details", zap.Error(err))
		return nil
	}
	if len(records) == 0 {
		log.Error("details weren't found in store")
		return nil
	}
	details := records[0].Details
	return details.ToProto()
}

func (s *service) retrieveVersion(publish *publishapi.Publish) *Version {
	version := &Version{}
	err := json.Unmarshal([]byte(publish.Version), version)
	if err != nil {
		log.Error("failed to unmarshal publish version", zap.Error(err))
	}
	return version
}

func (s *service) ResolveUri(ctx context.Context, uri string) (*pb.RpcPublishingPublishState, error) {
	publish, err := s.publishClientService.ResolveUri(ctx, uri)
	if err != nil {
		return nil, err
	}
	version := s.retrieveVersion(publish)
	return &pb.RpcPublishingPublishState{
		SpaceId:   publish.SpaceId,
		ObjectId:  publish.ObjectId,
		Uri:       publish.Uri,
		Status:    pb.RpcPublishingPublishStatus(publish.Status),
		Version:   publish.Version,
		Timestamp: publish.Timestamp,
		Size_:     publish.Size,
		JoinSpace: version.JoinSpace,
	}, nil
}

func (s *service) GetStatus(ctx context.Context, spaceId string, objectId string) (*pb.RpcPublishingPublishState, error) {
	status, err := s.publishClientService.GetPublishStatus(ctx, spaceId, objectId)
	if err != nil {
		return nil, err
	}
	version := s.retrieveVersion(status)
	return &pb.RpcPublishingPublishState{
		SpaceId:   status.SpaceId,
		ObjectId:  status.ObjectId,
		Uri:       status.Uri,
		Status:    pb.RpcPublishingPublishStatus(status.Status),
		Version:   status.Version,
		Timestamp: status.Timestamp,
		Size_:     status.Size,
		JoinSpace: version.JoinSpace,
	}, nil
}
