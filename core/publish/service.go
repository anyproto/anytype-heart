package publish

import (
	"compress/gzip"
	"context"
	"encoding/json"
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
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	jsonM = jsonpb.Marshaler{Indent: "  "}
)

const CName = "common.core.publishservice"

var log = logger.NewNamed(CName)

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
	Publish(ctx context.Context, spaceId, pageObjId, uri string) (res PublishResult, err error)
}

type service struct {
	commonFile           fileservice.FileService
	exportService        export.Export
	publishClientService publishclient.Client
	accountService       accountservice.Service
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.exportService = app.MustComponent[export.Export](a)
	s.publishClientService = app.MustComponent[publishclient.Client](a)
	s.accountService = app.MustComponent[accountservice.Service](a)

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

func (s *service) publishToServer(ctx context.Context, spaceId, pageId, uri string) (err error) {

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

	tempPublishDir := filepath.Join(os.TempDir(), uniqName())
	defer os.RemoveAll(tempPublishDir)

	if err := os.MkdirAll(tempPublishDir, 0777); err != nil {
		return err
	}

	uberSnapshot := PublishingUberSnapshot{
		Meta: PublishingUberSnapshotMeta{
			SpaceId:    spaceId,
			RootPageId: pageId,
		},
		PbFiles: make(map[string]string),
	}

	for _, entry := range dirEntries {
		if !entry.IsDir() {
			err = fmt.Errorf("unexpeted file on export root level: %s", entry.Name())
			return

		}
		var dirFiles []fs.DirEntry
		dirName := entry.Name()

		if dirName == "files" {
			err = os.CopyFS(filepath.Join(tempPublishDir, "files"), os.DirFS(filepath.Join(exportPath, "files")))
			if err != nil {
				return
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

	err = file.Close()
	if err != nil {
		return
	}

	log.Error("publishing started", zap.String("pageid", pageId), zap.String("uri", uri))
	publishReq := &publishapi.PublishRequest{
		SpaceId:  spaceId,
		ObjectId: pageId,
		Uri:      uri,
		Version:  "fake-ver-" + uniqName(),
	}

	uploadUrl, err := s.publishClientService.Publish(ctx, publishReq)
	if err != nil {
		return
	}

	log.Error("publishing upload started", zap.String("pageid", pageId), zap.String("uploadUrl", uploadUrl))
	err = s.publishClientService.UploadDir(ctx, uploadUrl, tempPublishDir)
	if err != nil {
		return
	}

	log.Error("publishing finished", zap.String("pageid", pageId))
	return nil

}

func (s *service) Publish(ctx context.Context, spaceId, pageId, uri string) (res PublishResult, err error) {
	log.Info("Publish called", zap.String("pageId", pageId))
	err = s.publishToServer(ctx, spaceId, pageId, uri)

	if err != nil {
		log.Error("Failed to publish", zap.Error(err))
		return
	}

	// for now: staging-url/identity/pageid
	// will be fixed in GO-4758
	stagingUrl := "https://any.coop"
	identity := s.accountService.Account().SignKey.GetPublic().Account()
	url := fmt.Sprintf("%s/%s/%s", stagingUrl, identity, uri)

	return PublishResult{
		Cid: url,
		Key: "fakekey",
	}, nil

}
