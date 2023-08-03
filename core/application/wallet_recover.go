package application

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/core/domain"
	"os"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

func (s *Service) WalletRecover(req *pb.RpcWalletRecoverRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Already recovered
	if s.mnemonic == req.Mnemonic {
		return nil
	}

	// test if mnemonic is correct
	_, err := core.WalletAccountAt(req.Mnemonic, 0)
	if err != nil {
		return domain.WrapErrorWithCode(err, pb.RpcWalletRecoverResponseError_BAD_INPUT)
	}

	err = os.MkdirAll(req.RootPath, 0700)
	if err != nil {
		return domain.WrapErrorWithCode(err, pb.RpcWalletRecoverResponseError_FAILED_TO_CREATE_LOCAL_REPO)
	}

	if err = s.setMnemonic(req.Mnemonic); err != nil {
		return err
	}
	s.rootPath = req.RootPath
	return nil
}
