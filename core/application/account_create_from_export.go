package application

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/anytype"
	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/anyerror"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/metricsid"
)

var (
	ErrAccountMismatch = errors.New("backup was made from different account")
)

func getUserProfile(req *pb.RpcAccountRecoverFromLegacyExportRequest) (*pb.Profile, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	f, err := archive.Open(constant.ProfileFile)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var profile pb.Profile

	err = profile.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

type RecoverFromLegacyResponse struct {
	AccountId       string
	PersonalSpaceId string
}

func (s *Service) RecoverFromLegacy(req *pb.RpcAccountRecoverFromLegacyExportRequest) (RecoverFromLegacyResponse, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	profile, err := getUserProfile(req)
	if err != nil {
		return RecoverFromLegacyResponse{}, anyerror.CleanupError(err)
	}

	err = s.stop()
	if err != nil {
		return RecoverFromLegacyResponse{}, err
	}

	if s.derivedKeys == nil {
		return RecoverFromLegacyResponse{}, ErrWalletNotInitialized
	}
	address := s.derivedKeys.Identity.GetPublic().Account()
	if profile.Address != s.derivedKeys.OldAccountKey.GetPublic().Account() && profile.Address != address {
		return RecoverFromLegacyResponse{}, ErrAccountMismatch
	}
	s.rootPath = req.RootPath
	s.fulltextPrimaryLanguage = req.FulltextPrimaryLanguage
	err = os.MkdirAll(s.rootPath, 0700)
	if err != nil {
		return RecoverFromLegacyResponse{}, anyerror.CleanupError(err)
	}
	if _, statErr := os.Stat(filepath.Join(s.rootPath, address)); os.IsNotExist(statErr) {
		if walletErr := core.WalletInitRepo(s.rootPath, s.derivedKeys.Identity); walletErr != nil {
			return RecoverFromLegacyResponse{}, walletErr
		}
	}
	cfg, err := s.getBootstrapConfig(req)
	if err != nil {
		return RecoverFromLegacyResponse{}, err
	}

	if profile.AnalyticsId != "" {
		cfg.AnalyticsId = profile.AnalyticsId
	} else {
		cfg.AnalyticsId, err = metricsid.DeriveMetricsId(s.derivedKeys.Identity)
		if err != nil {
			return RecoverFromLegacyResponse{}, err
		}
	}

	err = s.startApp(cfg, *s.derivedKeys)
	if err != nil {
		return RecoverFromLegacyResponse{}, err
	}

	err = s.setDetails(profile, req.Icon)
	if err != nil {
		return RecoverFromLegacyResponse{}, err
	}

	spaceID := app.MustComponent[account.Service](s.app).PersonalSpaceID()
	if err = s.app.MustComponent(builtinobjects.CName).(builtinobjects.BuiltinObjects).InjectMigrationDashboard(spaceID); err != nil {
		return RecoverFromLegacyResponse{}, errors.Join(ErrBadInput, err)
	}

	return RecoverFromLegacyResponse{
		AccountId:       address,
		PersonalSpaceId: spaceID,
	}, nil
}

func (s *Service) startApp(cfg *config.Config, derivationResult crypto.DerivationResult) error {
	comps := []app.Component{
		cfg,
		anytype.BootstrapWallet(s.rootPath, derivationResult, s.fulltextPrimaryLanguage),
		s.eventSender,
	}

	ctxWithValue := context.WithValue(context.Background(), metrics.CtxKeyEntrypoint, "account_create")
	var err error
	if s.app, err = anytype.StartNewApp(ctxWithValue, s.clientWithVersion, comps...); err != nil {
		return err
	}
	return nil
}

func (s *Service) getBootstrapConfig(req *pb.RpcAccountRecoverFromLegacyExportRequest) (*config.Config, error) {
	archive, err := zip.OpenReader(req.Path)
	if err != nil {
		return nil, err
	}
	oldCfg, err := extractConfig(archive)
	if err != nil {
		return nil, fmt.Errorf("failed to extract config: %w", err)
	}

	cfg := anytype.BootstrapConfig(true, "")
	cfg.LegacyFileStorePath = oldCfg.LegacyFileStorePath
	return cfg, nil
}

func (s *Service) setDetails(profile *pb.Profile, icon int64) error {
	profileDetails, accountDetails := buildDetails(profile, icon)
	ds := app.MustComponent[detailservice.Service](s.app)

	spaceService := app.MustComponent[space.Service](s.app)
	spc, err := spaceService.GetPersonalSpace(context.Background())
	if err != nil {
		return fmt.Errorf("get personal space: %w", err)
	}
	accountObjects := spc.DerivedIDs()

	if err := ds.SetDetails(nil,
		accountObjects.Profile,
		profileDetails,
	); err != nil {
		return err
	}
	if err := ds.SetDetails(nil,
		accountObjects.Workspace,
		accountDetails,
	); err != nil {
		return err
	}
	return nil
}

func buildDetails(profile *pb.Profile, icon int64) (profileDetails []domain.Detail, accountDetails []domain.Detail) {
	profileDetails = []domain.Detail{{
		Key:   bundle.RelationKeyName,
		Value: domain.String(profile.Name),
	}}
	if profile.Avatar == "" {
		profileDetails = append(profileDetails, domain.Detail{
			Key:   bundle.RelationKeyIconOption,
			Value: domain.Int64(icon),
		})
	} else {
		profileDetails = append(profileDetails, domain.Detail{
			Key:   bundle.RelationKeyIconImage,
			Value: domain.String(profile.Avatar),
		})
	}
	accountDetails = []domain.Detail{{
		Key:   bundle.RelationKeyIconOption,
		Value: domain.Int64(icon),
	}}
	return
}

func extractConfig(archive *zip.ReadCloser) (*config.Config, error) {
	for _, f := range archive.File {
		if f.Name == config.ConfigFileName {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}

			var conf config.Config
			err = json.NewDecoder(r).Decode(&conf)
			if err != nil {
				return nil, err
			}
			return &conf, nil
		}
	}
	return nil, fmt.Errorf("config.json not found in archive")
}
