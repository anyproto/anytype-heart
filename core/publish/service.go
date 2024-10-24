package publish

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/util/crypto"
	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/encode"
)

const CName = "common.core.publishservice"

var log = logger.NewNamed(CName)
var cidBuilder = cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256}

type PublishResult struct {
	Cid string
	Key string
}
type Service interface {
	app.ComponentRunnable
	Publish(ctx context.Context, spaceId, pageObjId string) (res PublishResult, err error)
}

type service struct {
	commonFile      fileservice.FileService
	fileSyncService filesync.FileSync
	spaceService    space.Service
	dagService      ipld.DAGService
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.dagService = s.commonFile.DAGService()
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.spaceService = app.MustComponent[space.Service](a)

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

// current structure of published ufs dir:
// ```
// - keys.json <- encrypted with main key, has keys for all the other files
// - index.json <- renderer input
// - asset1
// - asset2....
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

	// pageId is json content for now
	indexJson := fileObject{
		FileName: "index.json",
		Content:  []byte(pageId),
	}

	asset1 := fileObject{
		FileName: "foo.jpg",
		Content:  []byte("foo.jpg content"),
	}
	asset2 := fileObject{
		FileName: "bar.jpg",
		Content:  []byte("bar.jpg content"),
	}

	files = append(files, indexJson, asset1, asset2)

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

func (s *service) Publish(ctx context.Context, spaceId, input string) (res PublishResult, err error) {
	// shortcut, because anytype-ts uses custom mapping to make json object
	// so I just pass the whole object for now instead of id.
	res, err = s.publishUfs(ctx, spaceId, input)
	if err != nil {
		log.Error("Failed to publish", zap.Error(err))
	}

	return

}
