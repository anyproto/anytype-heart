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
	s.fulltextPrimaryLanguage = req.FulltextPrimaryLanguage

	err := os.MkdirAll(s.rootPath, 0700)
	if err != nil {
		return "", errors.Join(ErrFailedToCreateLocalRepo, err)
	}

	// Generate new mnemonic
	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return "", err
	}

	// Derive and store keys for mnemonic
	derivationResult, err := core.WalletAccountAt(mnemonic, 0)
	if err != nil {
		return "", fmt.Errorf("derive from mnemonic: %w", err)
	}
	s.derivedKeys = &derivationResult
	
	// Set session signing key
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	s.sessionSigningKey = buf
	
	return mnemonic, nil
}
