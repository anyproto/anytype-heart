package invitestore

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileoffloader"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "invitestore"

type Service interface {
	app.ComponentRunnable
	StoreInvite(ctx context.Context, invite *model.Invite) (id cid.Cid, key crypto.SymKey, err error)
	RemoveInvite(ctx context.Context, id cid.Cid) error
	GetInvite(ctx context.Context, id cid.Cid, key crypto.SymKey) (*model.Invite, error)
}

type service struct {
	commonFile      fileservice.FileService
	fileOffloader   fileoffloader.Service
	fileSyncService filesync.FileSync
	spaceService    space.Service
	techSpaceId     string
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.fileOffloader = app.MustComponent[fileoffloader.Service](a)
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.fileSyncService = app.MustComponent[filesync.FileSync](a)
	s.spaceService = app.MustComponent[space.Service](a)
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

func (s *service) StoreInvite(ctx context.Context, invite *model.Invite) (cid.Cid, crypto.SymKey, error) {
	key, err := crypto.NewRandomAES()
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("generate key: %w", err)
	}

	rawInvite, err := proto.Marshal(invite)
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("marshal invite: %w", err)
	}
	data, err := key.Encrypt(rawInvite)
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("encrypt invite data: %w", err)
	}

	rd := bytes.NewReader(data)
	node, err := s.commonFile.AddFile(ctx, rd)
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("add data to IPFS: %w", err)
	}
	err = s.fileSyncService.UploadSynchronously(ctx, s.techSpaceId, domain.FileId(node.Cid().String()))
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("add file to sync queue: %w", err)
	}
	return node.Cid(), key, nil
}

func (s *service) RemoveInvite(ctx context.Context, id cid.Cid) error {
	_, err := s.fileOffloader.FileOffloadRaw(ctx, domain.FullFileId{
		SpaceId: s.techSpaceId,
		FileId:  domain.FileId(id.String()),
	})
	if err != nil {
		return fmt.Errorf("offload file: %w", err)
	}
	return s.fileSyncService.DeleteFileSynchronously(domain.FullFileId{SpaceId: s.techSpaceId, FileId: domain.FileId(id.String())})
}

func (s *service) GetInvite(ctx context.Context, id cid.Cid, key crypto.SymKey) (*model.Invite, error) {
	rd, err := s.commonFile.GetFile(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get data from IPFS: %w", err)
	}
	defer rd.Close()

	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	data, err = key.Decrypt(data)
	if err != nil {
		return nil, fmt.Errorf("decrypt data: %w", err)
	}

	var invite model.Invite
	err = proto.Unmarshal(data, &invite)
	if err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return &invite, nil
}
