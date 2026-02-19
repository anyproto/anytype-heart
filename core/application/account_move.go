package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	cp "github.com/otiai10/copy"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

var (
	ErrFailedToGetConfig          = errors.New("get config")
	ErrFailedToIdentifyAccountDir = errors.New("failed to identify account dir")
)

func (s *Service) AccountMove(req *pb.RpcAccountMoveRequest) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	dirs := []string{filestorage.FlatfsDirName}
	conf := s.app.MustComponent(config.CName).(*config.Config)

	configPath := conf.GetConfigPath()
	srcPath := conf.RepoPath
	fileConf := config.ConfigRequired{}
	if err := config.GetFileConfig(configPath, &fileConf); err != nil {
		return errors.Join(ErrFailedToGetConfig, err)
	}
	if fileConf.CustomFileStorePath != "" {
		srcPath = fileConf.CustomFileStorePath
	}

	parts := strings.Split(srcPath, string(filepath.Separator))
	accountDir := parts[len(parts)-1]
	if accountDir == "" {
		return ErrFailedToIdentifyAccountDir
	}

	destination := filepath.Join(req.NewPath, accountDir)
	if srcPath == destination {
		return errors.Join(ErrFailedToCreateLocalRepo, errors.New("source path should not be equal destination path"))
	}

	if _, err := os.Stat(destination); !os.IsNotExist(err) { // if already exist (in case of the previous fail moving)
		if err := removeDirsRelativeToPath(destination, dirs); err != nil {
			return errors.Join(ErrFailedToRemoveAccountData, anyerror.CleanupError(err))
		}
	}

	err := os.MkdirAll(destination, 0700)
	if err != nil {
		return errors.Join(ErrFailedToCreateLocalRepo, anyerror.CleanupError(err))
	}

	err = s.stop()
	if err != nil {
		return errors.Join(ErrFailedToStopApplication, err)
	}

	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(srcPath, dir)); !os.IsNotExist(err) { // copy only if exist such dir
			if err := cp.Copy(filepath.Join(srcPath, dir), filepath.Join(destination, dir), cp.Options{PreserveOwner: true}); err != nil {
				return errors.Join(ErrFailedToCreateLocalRepo, err)
			}
		}
	}

	err = config.WriteJsonConfig(configPath, config.ConfigRequired{CustomFileStorePath: destination})
	if err != nil {
		return errors.Join(ErrFailedToWriteConfig, err)
	}

	if err := removeDirsRelativeToPath(srcPath, dirs); err != nil {
		return errors.Join(ErrFailedToRemoveAccountData, anyerror.CleanupError(err))
	}

	if srcPath != conf.RepoPath { // remove root account dir, if move not from anytype source dir
		if err := os.RemoveAll(srcPath); err != nil {
			return errors.Join(ErrFailedToRemoveAccountData, anyerror.CleanupError(err))
		}
	}
	return nil
}

func removeDirsRelativeToPath(rootPath string, dirs []string) error {
	for _, dir := range dirs {
		if err := os.RemoveAll(filepath.Join(rootPath, dir)); err != nil {
			return err
		}
	}
	return nil
}
