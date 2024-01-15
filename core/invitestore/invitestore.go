package invitestore

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
)

const CName = "invitestore"

type Service interface {
	app.Component
	StoreInvite(ctx context.Context, invite string) (id cid.Cid, key crypto.SymKey, err error)
	GetInvite(ctx context.Context, id cid.Cid, key crypto.SymKey) (string, error)
}

type service struct {
	commonFile fileservice.FileService
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) error {
	s.commonFile = app.MustComponent[fileservice.FileService](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) StoreInvite(ctx context.Context, invite string) (cid.Cid, crypto.SymKey, error) {
	key, err := crypto.NewRandomAES()
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("generate key: %w", err)
	}

	data, err := key.Encrypt([]byte(invite))
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("encrypt invite data: %w", err)
	}

	rd := bytes.NewReader(data)
	node, err := s.commonFile.AddFile(ctx, rd)
	if err != nil {
		return cid.Cid{}, nil, fmt.Errorf("add data to IPFS: %w", err)
	}
	return node.Cid(), key, nil
}

func (s *service) GetInvite(ctx context.Context, id cid.Cid, key crypto.SymKey) (string, error) {
	rd, err := s.commonFile.GetFile(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get data from IPFS: %w", err)
	}
	defer rd.Close()

	data, err := io.ReadAll(rd)
	if err != nil {
		return "", fmt.Errorf("read data: %w", err)
	}

	data, err = key.Decrypt(data)
	if err != nil {
		return "", fmt.Errorf("decrypt data: %w", err)
	}
	return string(data), nil
}
