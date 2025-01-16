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

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/anytype-publish-server/publishclient"
	"github.com/anyproto/anytype-publish-server/publishclient/publishapi"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/hash"
)

var (
	jsonM = jsonpb.Marshaler{Indent: "  "}
)

const CName = "common.core.publishservice"

const (
	membershipLimit       = 100 << 20
	defaultLimit          = 10 << 20
	inviteLinkUrlTemplate = "https://invite.any.coop/%s#%s"
	memberUrlTemplate     = "https://%s.coop"
	defaultUrlTemplate    = "https://any.coop/%s"
)

var log = logger.NewNamed(CName)

var ErrLimitExceeded = errors.New("limit exceeded")

type PublishResult struct {
	Cid string
	Key string
}

type PublishingUberSnapshotMeta struct {
	SpaceId    string `json:"spaceId,omitempty"`
	RootPageId string `json:"rootPageId,omitempty"`
	InviteLink string `json:"inviteLink,omitempty"`
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

type MembershipStatusProvider interface {
	MembershipStatus() model.MembershipStatus
}

type service struct {
	commonFile               fileservice.FileService
	fileSyncService          filesync.FileSync
	spaceService             space.Service
	dagService               ipld.DAGService
	exportService            export.Export
	publishClientService     publishclient.Client
	accountService           accountservice.Service
	membershipStatusProvider MembershipStatusProvider
	identityService          identity.Service
	inviteService            inviteservice.InviteService
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.dagService = s.commonFile.DAGService()
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.exportService = app.MustComponent[export.Export](a)
	s.publishClientService = app.MustComponent[publishclient.Client](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.membershipStatusProvider = app.MustComponent[MembershipStatusProvider](a)
	s.identityService = app.MustComponent[identity.Service](a)
	s.inviteService = app.MustComponent[inviteservice.InviteService](a)
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

func (s *service) exportToDir(ctx context.Context, spaceId, pageId string) (dirEntries []fs.DirEntry, exportPath string, err error) {
	tempDir := os.TempDir()
	exportPath, _, err = s.exportService.Export(ctx, pb.RpcObjectListExportRequest{
		SpaceId:      spaceId,
		Format:       model.Export_Protobuf,
		IncludeFiles: true,
		IsJson:       false,
		Zip:          false,
		Path:         tempDir,
		ObjectIds:    []string{pageId},
		NoProgress:   true,
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

func (s *service) publishToPublishServer(ctx context.Context, spaceId, pageId, uri string, joinSpace bool) (err error) {

	dirEntries, exportPath, err := s.exportToDir(ctx, spaceId, pageId)
	if err != nil {
		return
	}
	defer os.RemoveAll(exportPath)

	// go through dir dirs
	// if files/, copy them to `publishTmpFolder`
	// else
	// for each file in $dir
	// create a json in "main" map like
	// {
	//   pbfiles: {
	//     "objects/arstoarseitnwfuy.pb": <jsonpb content of this file>,
	//   }
	// }
	// after that, also add
	// "meta": { "root-page", "inviteLink", and other things}
	// then, add this `index.json` in `publishTmpFolder`

	limit := s.getPublishLimit()
	tempPublishDir := filepath.Join(os.TempDir(), uniqName())
	defer os.RemoveAll(tempPublishDir)

	if err := os.MkdirAll(tempPublishDir, 0777); err != nil {
		return err
	}

	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	inviteLink, err := s.extractInviteLink(ctx, spaceId, joinSpace, spc.IsPersonal())
	if err != nil {
		return err
	}
	uberSnapshot := PublishingUberSnapshot{
		Meta: PublishingUberSnapshotMeta{
			SpaceId:    spaceId,
			RootPageId: pageId,
			InviteLink: inviteLink,
		},
		PbFiles: make(map[string]string),
	}

	var size int64
	for _, entry := range dirEntries {
		if entry.IsDir() {
			var dirFiles []fs.DirEntry
			dirName := entry.Name()

			if dirName == export.Files {
				fileDir := filepath.Join(tempPublishDir, export.Files)
				err = os.CopyFS(fileDir, os.DirFS(filepath.Join(exportPath, export.Files)))
				if err != nil {
					return
				}
				err = filepath.Walk(fileDir, func(path string, info os.FileInfo, err error) error {
					if !info.IsDir() {
						size = size + info.Size()
						if size > limit {
							return ErrLimitExceeded
						}
					}
					return nil
				})
				if err != nil {
					return err
				}
				continue
			}

			dirFiles, err = os.ReadDir(filepath.Join(exportPath, dirName))
			if err != nil {
				return
			}

			for _, file := range dirFiles {
				withDirName := filepath.Join(dirName, file.Name())
				var snapshotData []byte
				snapshotData, err = os.ReadFile(filepath.Join(exportPath, withDirName))
				if err != nil {
					return
				}

				snapshot := pb.SnapshotWithType{}
				err = proto.Unmarshal(snapshotData, &snapshot)
				if err != nil {
					return
				}

				var jsonData string
				jsonData, err = jsonM.MarshalToString(&snapshot)
				if err != nil {
					return
				}

				fileNameKey := fmt.Sprintf("%s/%s", dirName, file.Name())
				uberSnapshot.PbFiles[fileNameKey] = jsonData

			}
		} else {
			err = fmt.Errorf("unexpeted file on export root level: %s", entry.Name())
			return
		}

	}

	var jsonData []byte
	jsonData, err = json.Marshal(&uberSnapshot)
	if err != nil {
		return
	}

	outputFile := filepath.Join(tempPublishDir, "index.json.gz")

	var file *os.File
	file, err = os.Create(outputFile)
	if err != nil {
		return
	}

	writer := gzip.NewWriter(file)
	_, err = writer.Write(jsonData)
	err = writer.Close()
	if err != nil {
		file.Close()
		return
	}

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	err = file.Close()
	if err != nil {
		return
	}
	size = size + stat.Size()
	if size > limit {
		return ErrLimitExceeded
	}

	version, err := s.evaluateDocumentVersion(spc, pageId)
	if err != nil {
		return err
	}
	publishReq := &publishapi.PublishRequest{
		SpaceId:  spaceId,
		ObjectId: pageId,
		Uri:      uri,
		Version:  version,
	}

	uploadUrl, err := s.publishClientService.Publish(ctx, publishReq)
	if err != nil {
		return
	}

	err = s.publishClientService.UploadDir(ctx, uploadUrl, tempPublishDir)
	if err != nil {
		return
	}
	return nil

}

func (s *service) extractInviteLink(ctx context.Context, spaceId string, joinSpace, isPersonal bool) (string, error) {
	var inviteLink string
	if joinSpace && !isPersonal {
		inviteInfo, err := s.inviteService.GetCurrent(ctx, spaceId)
		if err != nil && errors.Is(err, inviteservice.ErrInviteNotExists) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		inviteLink = fmt.Sprintf(inviteLinkUrlTemplate, inviteInfo.InviteFileCid, inviteInfo.InviteFileKey)
	}
	return inviteLink, nil
}

func (s *service) evaluateDocumentVersion(spc clientspace.Space, pageId string) (string, error) {
	treeStorage, err := spc.Storage().TreeStorage(pageId)
	if err != nil {
		return "", err
	}
	heads, err := treeStorage.Heads()
	if err != nil {
		return "", err
	}
	version := hash.HeadsHash(heads)
	return version, nil
}

func (s *service) getPublishLimit() int64 {
	status := s.membershipStatusProvider.MembershipStatus()
	limit := defaultLimit
	if status == model.Membership_StatusActive {
		limit = membershipLimit
	}
	return int64(limit)
}

func (s *service) Publish(ctx context.Context, spaceId, pageId, uri string, joinSpace bool) (res PublishResult, err error) {
	log.Info("Publish called", zap.String("pageId", pageId))
	err = s.publishToPublishServer(ctx, spaceId, pageId, uri, joinSpace)

	if err != nil {
		log.Error("Failed to publish", zap.Error(err))
		return
	}
	url := s.makeUrl(ctx, uri)

	return PublishResult{Cid: url}, nil
}

func (s *service) makeUrl(ctx context.Context, uri string) string {
	identity, _, details := s.identityService.GetMyProfileDetails(ctx)
	globalName := details.GetString(bundle.RelationKeyGlobalName)
	var domain string
	if globalName != "" {
		domain = fmt.Sprintf(memberUrlTemplate, globalName)
	} else {
		domain = fmt.Sprintf(defaultUrlTemplate, identity)
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
		pbPublishes = append(pbPublishes, &pb.RpcPublishingPublishState{
			SpaceId:   publish.SpaceId,
			ObjectId:  publish.ObjectId,
			Uri:       publish.Uri,
			Status:    pb.RpcPublishingPublishStatus(publish.Status),
			Version:   publish.Version,
			Timestamp: publish.Timestamp,
			Size_:     publish.Size_,
		})
	}
	return pbPublishes, nil
}

func (s *service) ResolveUri(ctx context.Context, uri string) (*pb.RpcPublishingPublishState, error) {
	publish, err := s.publishClientService.ResolveUri(ctx, uri)
	if err != nil {
		return nil, err
	}
	return &pb.RpcPublishingPublishState{
		SpaceId:   publish.SpaceId,
		ObjectId:  publish.ObjectId,
		Uri:       publish.Uri,
		Status:    pb.RpcPublishingPublishStatus(publish.Status),
		Version:   publish.Version,
		Timestamp: publish.Timestamp,
		Size_:     publish.Size_,
	}, nil
}

func (s *service) GetStatus(ctx context.Context, spaceId string, objectId string) (*pb.RpcPublishingPublishState, error) {
	status, err := s.publishClientService.GetPublishStatus(ctx, spaceId, objectId)
	if err != nil {
		return nil, err
	}
	return &pb.RpcPublishingPublishState{
		SpaceId:   status.SpaceId,
		ObjectId:  status.ObjectId,
		Uri:       status.Uri,
		Status:    pb.RpcPublishingPublishStatus(status.Status),
		Version:   status.Version,
		Timestamp: status.Timestamp,
		Size_:     status.Size_,
	}, nil
}
