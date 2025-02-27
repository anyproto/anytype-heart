package application

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

const wordCount int = 12

func (s *Service) WalletCreate(req *pb.RpcWalletCreateRequest) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.rootPath = req.RootPath
	s.lang = req.Lang

	err := os.MkdirAll(s.rootPath, 0700)
	if err != nil {
		return "", errors.Join(ErrFailedToCreateLocalRepo, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return "", err
	}

	if err = s.setMnemonic(mnemonic); err != nil {
		return "", fmt.Errorf("set mnemonic: %w", err)
	}
	return mnemonic, nil
}

func (s *Service) setMnemonic(mnemonic string) error {
	s.mnemonic = mnemonic
	// TODO: I guess we can use any random bytes here
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	s.sessionSigningKey = buf
	return nil
}
