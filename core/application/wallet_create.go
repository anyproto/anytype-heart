package application

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

const wordCount int = 12

var ErrWalletNotInitialized = errors.New("wallet not initialized")

func (s *Service) WalletCreate(req *pb.RpcWalletCreateRequest) (mnemonic, accountKey string, err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.rootPath = req.RootPath
	s.fulltextPrimaryLanguage = req.FulltextPrimaryLanguage

	err = os.MkdirAll(s.rootPath, 0700)
	if err != nil {
		return "", "", errors.Join(ErrFailedToCreateLocalRepo, err)
	}

	mnemonic, err = core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return "", "", err
	}

	// Derive and store keys for mnemonic
	derivationResult, err := core.WalletAccountAt(mnemonic, 0)
	if err != nil {
		return "", "", fmt.Errorf("derive from mnemonic: %w", err)
	}
	s.derivedKeys = &derivationResult

	// Set session signing key
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	s.sessionSigningKey = buf
	accountMasterNode, err := derivationResult.MasterNode.MarshalBinary()
	if err != nil {
		return "", "", err
	}

	return mnemonic, base64.StdEncoding.EncodeToString(accountMasterNode), nil
}
