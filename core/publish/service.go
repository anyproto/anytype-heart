package publish

import (
	"bytes"
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
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-publish-server/publishclient"
	"github.com/anyproto/anytype-publish-server/publishclient/publishapi"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/encode"
)

var (
	jsonM = jsonpb.Marshaler{Indent: "  "}
)

const CName = "common.core.publishservice"

const (
	membershipLimit = 100 << 20
	defaultLimit    = 10 << 20
)

var log = logger.NewNamed(CName)
var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

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
	Publish(ctx context.Context, spaceId, pageObjId, uri string) (res PublishResult, err error)
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

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

type keyObject struct {
	Cid string `json:"cid"`
	Key string `json:"key"`
}

type fileObject struct {
	FileName string
	Content  []byte
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

func makeFileObject(dirPath, fileName string) (asset fileObject, err error) {
	var content []byte

	content, err = os.ReadFile(filepath.Join(dirPath, fileName))
	if err != nil {
		return
	}
	asset = fileObject{
		FileName: fileName,
		Content:  content,
	}

	return

}

// current structure of published ufs dir:
// ```
//   - keys.json <- encrypted with main key, has keys for all the other files
//   - rootPath  <- contains path to root object
//   - encrypted asset1
//   - encrypted asset2
//     ...
//
// ```
func (s *service) publishUfs(ctx context.Context, spaceId, pageId string) (res PublishResult, err error) {
	dagService := s.dagServiceForSpace(spaceId)
	outer := uio.NewDirectory(dagService)
	outer.SetCidBuilder(cidBuilder)

	mainKey, err := crypto.NewRandomAES()
	if err != nil {
		return
	}
	// will be converted to json and encrypted by main key
	keys := make(map[string]keyObject, 0)

	// will be added via commonFile.AddFile
	files := make([]fileObject, 0)

	dirEntries, exportPath, err := s.exportToDir(ctx, spaceId, pageId)
	if err != nil {
		return
	}

	for _, entry := range dirEntries {
		var asset fileObject
		if entry.IsDir() {
			var dirFiles []fs.DirEntry
			dirName := entry.Name()

			dirFiles, err = os.ReadDir(filepath.Join(exportPath, dirName))
			if err != nil {
				return
			}

			for _, file := range dirFiles {
				withDirName := filepath.Join(dirName, file.Name())
				asset, err = makeFileObject(exportPath, withDirName)
				if err != nil {
					return
				}

				files = append(files, asset)
			}
		} else {
			asset, err = makeFileObject(exportPath, entry.Name())
			if err != nil {
				return
			}

			files = append(files, asset)
		}
	}

	// add all files via common file, to outer ipfs dir and to keys
	for _, file := range files {
		var key *crypto.AESKey
		key, err = crypto.NewRandomAES()
		if err != nil {
			return
		}

		var encContent []byte
		encContent, err = key.Encrypt(file.Content)
		if err != nil {
			return
		}

		var node ipld.Node
		node, err = s.commonFile.AddFile(ctx, bytes.NewReader(encContent))
		if err != nil {
			return
		}

		err = dagService.Add(ctx, node)
		if err != nil {
			return
		}

		cid := node.Cid().String()
		err = helpers.AddLinkToDirectory(ctx, dagService, outer, file.FileName, cid)
		if err != nil {
			return
		}

		var keyStr string
		keyStr, err = encode.EncodeKeyToBase58(key)
		if err != nil {
			return
		}

		keys[file.FileName] = keyObject{
			Cid: cid,
			Key: keyStr,
		}
	}

	// now, add keys to files and encrypt with the main key which will be returned
	var keysJson []byte
	keysJson, err = json.Marshal(keys)
	if err != nil {
		return
	}

	var encKeys []byte
	encKeys, err = mainKey.Encrypt(keysJson)
	if err != nil {
		return
	}

	var node ipld.Node
	node, err = s.commonFile.AddFile(ctx, bytes.NewReader(encKeys))
	if err != nil {
		return
	}

	err = dagService.Add(ctx, node)
	if err != nil {
		return
	}

	cid := node.Cid().String()
	err = helpers.AddLinkToDirectory(ctx, dagService, outer, "keys.json", cid)
	if err != nil {
		return
	}

	rootPath := filepath.Join("objects", pageId+".pb")
	node, err = s.commonFile.AddFile(ctx, bytes.NewReader([]byte(rootPath)))
	if err != nil {
		return
	}

	err = dagService.Add(ctx, node)
	if err != nil {
		return
	}

	cid = node.Cid().String()
	err = helpers.AddLinkToDirectory(ctx, dagService, outer, "rootPath", cid)
	if err != nil {
		return
	}

	var mainKeyStr string
	mainKeyStr, err = encode.EncodeKeyToBase58(mainKey)
	if err != nil {
		return
	}

	var outerNode ipld.Node
	outerNode, err = outer.GetNode()
	if err != nil {
		return
	}

	err = dagService.Add(ctx, outerNode)
	if err != nil {
		return
	}

	outerNodeCid := outerNode.Cid().String()

	// upload ufs root node Cid
	err = s.fileSyncService.UploadSynchronously(ctx, spaceId, domain.FileId(outerNodeCid))
	if err != nil {
		return
	}

	// and return node Cid and mainKey
	res.Cid = outerNodeCid
	res.Key = mainKeyStr
	return
}

func (s *service) publishToPublishServer(ctx context.Context, spaceId, pageId, uri string) (err error) {

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

	uberSnapshot := PublishingUberSnapshot{
		Meta: PublishingUberSnapshotMeta{
			SpaceId:    spaceId,
			RootPageId: pageId,
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

func (s *service) getPublishLimit() int64 {
	status := s.membershipStatusProvider.MembershipStatus()
	limit := defaultLimit
	if status == model.Membership_StatusActive {
		limit = membershipLimit
	}
	return int64(limit)
}

func (s *service) Publish(ctx context.Context, spaceId, pageId, uri string) (res PublishResult, err error) {
	log.Info("Publish called", zap.String("pageId", pageId))
	err = s.publishToPublishServer(ctx, spaceId, pageId, uri)

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
