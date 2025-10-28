package application

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

func (s *Service) WalletRecover(req *pb.RpcWalletRecoverRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Validate that only one auth method is provided
	if req.Mnemonic != "" && req.AccountKey != "" {
		return errors.Join(ErrBadInput, fmt.Errorf("cannot provide both mnemonic and accountKey"))
	}

	if req.Mnemonic == "" && req.AccountKey == "" {
		return errors.Join(ErrBadInput, fmt.Errorf("either mnemonic or accountKey must be provided"))
	}

	err := os.MkdirAll(req.RootPath, 0700)
	if err != nil {
		return errors.Join(ErrFailedToCreateLocalRepo, err)
	}

	// Handle accountKey recovery
	if req.AccountKey != "" {
		// Derive keys from the provided account key
		derivationResult, err := core.WalletDeriveFromAccountMasterNode(req.AccountKey)
		if err != nil {
			return errors.Join(ErrBadInput, fmt.Errorf("invalid account key: %w", err))
		}

		// Store the derived keys
		s.derivedKeys = &derivationResult

		// Set session signing key
		buf := make([]byte, 64)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		s.sessionSigningKey = buf

		s.rootPath = req.RootPath
		s.fulltextPrimaryLanguage = req.FulltextPrimaryLanguage
		return nil
	}

	// Handle mnemonic recovery
	// Derive keys from mnemonic
	derivationResult, err := core.WalletAccountAt(req.Mnemonic, 0)
	if err != nil {
		return errors.Join(ErrBadInput, err)
	}

	// Store the derived keys
	s.derivedKeys = &derivationResult

	// Set session signing key
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	s.sessionSigningKey = buf

	s.rootPath = req.RootPath
	s.fulltextPrimaryLanguage = req.FulltextPrimaryLanguage
	return nil
}
