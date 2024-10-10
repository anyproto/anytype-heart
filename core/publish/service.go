package publish

import (
	"bytes"
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/encode"
)

const CName = "common.core.publishservice"

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
	blockService    *block.Service
	techSpaceId     string
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.spaceService = app.MustComponent[space.Service](a)
	s.blockService = a.MustComponent(block.CName).(*block.Service)
	return nil
}

func (s *service) Run(_ context.Context) error {
	s.techSpaceId = s.spaceService.TechSpaceId()
	return nil
}

func (s *service) Close(_ context.Context) error {
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Publish(ctx context.Context, spaceId, pageObjId string) (res PublishResult, err error) {
	id := domain.FullID{
		SpaceID:  spaceId,
		ObjectID: pageObjId,
	}

	obj, err := s.blockService.ShowBlock(id, true)
	if err != nil {
		return
	}

	key, err := crypto.NewRandomAES()
	if err != nil {
		return
	}

	rawObj, err := proto.Marshal(obj)
	if err != nil {
		return
	}
	data, err := key.Encrypt(rawObj)
	if err != nil {
		return
	}

	rd := bytes.NewReader(data)
	node, err := s.commonFile.AddFile(ctx, rd)
	if err != nil {
		return
	}

	cidStr := node.Cid().String()
	err = s.fileSyncService.UploadSynchronously(ctx, s.techSpaceId, domain.FileId(cidStr))
	if err != nil {
		return
	}

	keyStr, err := encode.EncodeKeyToBase58(key)
	if err != nil {
		return
	}

	res.Cid = cidStr
	res.Key = keyStr
	return
}
