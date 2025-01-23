package core

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-core")

const tmpDir = "tmp"

type TempDirProvider interface {
	TempDir() string
}

type TempDirService struct {
	wallet              wallet.Wallet
	tmpFolderAutocreate sync.Once
	tempDir             string
}

func NewTempDirService() *TempDirService {
	return &TempDirService{}
}

func (s *TempDirService) Init(a *app.App) error {
	s.wallet = app.MustComponent[wallet.Wallet](a)
	return nil
}

func (s *TempDirService) Name() string {
	return "core.tmpdir"
}

func (s *TempDirService) TempDir() string {
	// it shouldn't be a case when it is called before wallet init, but just in case lets add the check here
	if s.wallet == nil || s.wallet.RootPath() == "" {
		return os.TempDir()
	}

	var err error
	// simultaneous calls to TempDir will wait for the once func to finish, so it will be fine
	s.tmpFolderAutocreate.Do(func() {
		path := filepath.Join(s.wallet.RootPath(), tmpDir)
		err = os.MkdirAll(path, 0700)
		if err != nil {
			log.Errorf("failed to make temp dir, use the default system one: %s", err)
			s.tempDir = os.TempDir()
		} else {
			s.tempDir = path
		}
	})

	return s.tempDir
}
