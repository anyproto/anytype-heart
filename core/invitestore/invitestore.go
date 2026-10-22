package invitestore

/*
AI generated

Name: Encrypted Space Invite Storage
Scope: global

## Responsibility
- Store, retrieve, and remove encrypted space invite payloads via IPFS
- Encrypt invites with AES before upload, decrypt on retrieval
*/

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/proto"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs/pb"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "invitestore"

// ErrInviteUnreadable means the invite block was fetched but could not be turned back into an
// invite. Retrying will not change that.
var ErrInviteUnreadable = errors.New("invite is unreadable")

type Service interface {
	app.ComponentRunnable
	// StoreInvite uploads an invite file and binds it, on the coordinator, to the invite it belongs
	// to. inviteKey is the public part of the invite's acl key — of the guest key, for a guest invite.
	// The coordinator needs that binding to resolve a later removal request.
	StoreInvite(ctx context.Context, spaceId string, inviteKey crypto.PubKey, invite *model.Invite) (id cid.Cid, key crypto.SymKey, err error)
	RemoveInvite(ctx context.Context, id cid.Cid) error
	GetInvite(ctx context.Context, id cid.Cid, key crypto.SymKey) (*model.Invite, error)
}

type service struct {
	commonFile   fileservice.FileService
	coordinator  coordinatorclient.CoordinatorClient
	spaceService space.Service
	techSpaceId  string
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	s.coordinator = app.MustComponent[coordinatorclient.CoordinatorClient](a)
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

func (s *service) StoreInvite(ctx context.Context, spaceId string, inviteKey crypto.PubKey, invite *model.Invite) (cid.Cid, crypto.SymKey, error) {
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

	block, err := makeIpfsBlock(data)
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("make block: %w", err)
	}

	if err = s.coordinator.AclUploadInvite(ctx, spaceId, inviteKey, block); err != nil {
		return cid.Cid{}, nil, fmt.Errorf("add data to IPFS: %w", err)
	}
	return block.Cid(), key, nil
}

func (s *service) RemoveInvite(ctx context.Context, id cid.Cid) error {
	return nil
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
		return nil, fmt.Errorf("%w: decrypt data: %w", ErrInviteUnreadable, err)
	}

	var invite model.Invite
	err = proto.Unmarshal(data, &invite)
	if err != nil {
		return nil, fmt.Errorf("%w: unmarshal data: %w", ErrInviteUnreadable, err)
	}
	return &invite, nil
}

func makeIpfsBlock(data []byte) (blocks.Block, error) {
	uf := &pb.Data{
		Type:     pb.Data_File.Enum(),
		Data:     data,
		Filesize: proto.Uint64(uint64(len(data))),
	}
	ufBytes, err := proto.Marshal(uf)
	if err != nil {
		return nil, err
	}

	node := merkledag.NodeWithData(ufBytes)
	block, err := node.EncodeProtobuf(false)
	if err != nil {
		return nil, err
	}

	prefix := cid.Prefix{
		Version:  1,
		Codec:    cid.DagProtobuf, // 0x70
		MhType:   mh.SHA2_256,
		MhLength: -1, // default length
	}
	c, err := prefix.Sum(block)
	if err != nil {
		return nil, err
	}
	return blocks.NewBlockWithCid(block, c)
}
