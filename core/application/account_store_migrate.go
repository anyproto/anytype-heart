package application

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

var ErrAccountNotFound = errors.New("account not found")

func (s *Service) AccountMigrate(ctx context.Context, req *pb.RpcAccountMigrateRequest) error {
	return s.migrate(ctx, req.Id, req.RootPath)
}

func (s *Service) migrate(ctx context.Context, id string, rootPath string) error {
	if rootPath != "" {
		s.rootPath = rootPath
	}
	res, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(s.rootPath, id)); err != nil {
		if os.IsNotExist(err) {
			return ErrAccountNotFound
		}
		return err
	}
	cfg := anytype.BootstrapConfig(false, os.Getenv("ANYTYPE_STAGING") == "1")
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, res),
		s.eventSender,
	}
	spaceStorePath := path.Join(s.rootPath, id, "spacestore")
	a := &app.App{}
	anytype.BootstrapMigration(spaceStorePath, a, comps...)
	err = a.Start(ctx)
	if err != nil {
		return err
	}
	return a.Close(ctx)
}
