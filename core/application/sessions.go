package application

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) CreateSession(req *pb.RpcWalletCreateSessionRequest) (token string, accountId string, err error) {
	// test if mnemonic is correct
	mnemonic := req.GetMnemonic()
	appKey := req.GetAppKey()
	providedToken := req.GetToken()
	accountKey := req.GetAccountKey()

	if appKey != "" {
		app := s.GetApp()
		if app == nil {
			return "", "", ErrApplicationIsNotRunning
		}
		wallet := app.Component(walletComp.CName)
		if wallet == nil {
			return "", "", fmt.Errorf("appToken auth not yet supported for the main app")
		}
		w := wallet.(walletComp.Wallet)
		appLink, err := w.ReadAppLink(appKey)
		if err != nil {
			return "", "", err
		}
		log.Infof("appLink auth %s", appLink.AppName)
		token, err := s.sessions.StartSession(s.sessionSigningKey, model.AccountAuthLocalApiScope(appLink.Scope)) // nolint:gosec
		if err != nil {
			return "", "", err
		}
		s.lock.Lock()
		defer s.lock.Unlock()
		s.sessionsByAppHash[appLink.AppHash] = token

		return token, w.Account().SignKey.GetPublic().Account(), nil
	}

	if providedToken != "" {
		scope, err := s.sessions.ValidateToken(s.sessionSigningKey, providedToken)
		if err != nil {
			return "", "", err
		}
		token, err = s.sessions.StartSession(s.sessionSigningKey, scope) // nolint:gosec
		return token, "", err
	}

	var derived crypto.DerivationResult

	if accountKey != "" {
		derived, err = core.WalletDeriveFromAccountMasterNode(accountKey)
		if err != nil {
			return "", "", errors.Join(ErrBadInput, fmt.Errorf("invalid account key: %w", err))
		}
	} else {
		if s.derivedKeys == nil {
			return "", "", errors.Join(ErrBadInput, fmt.Errorf("wallet not initialized"))
		}

		// Derive keys from provided mnemonic to verify it's correct
		derived, err = core.WalletAccountAt(mnemonic, 0)
		if err != nil {
			return "", "", errors.Join(ErrBadInput, fmt.Errorf("invalid mnemonic"))
		}
	}

	// Compare account IDs to verify we are at the same account
	if derived.Identity.GetPublic().Account() != s.derivedKeys.Identity.GetPublic().Account() {
		return "", "", errors.Join(ErrBadInput, fmt.Errorf("incorrect mnemonic"))
	}
	token, err = s.sessions.StartSession(s.sessionSigningKey, model.AccountAuth_Full)
	if err != nil {
		return "", "", err
	}
	// todo: account is empty, to be implemented with GO-1854
	return token, "", nil
}

func (s *Service) CloseSession(req *pb.RpcWalletCloseSessionRequest) error {
	if sender, ok := s.eventSender.(session.Closer); ok {
		sender.CloseSession(req.Token)
	}
	return s.sessions.CloseSession(req.Token)
}

func (s *Service) ValidateSessionToken(token string) (model.AccountAuthLocalApiScope, error) {
	return s.sessions.ValidateToken(s.sessionSigningKey, token)
}

func (s *Service) LinkLocalStartNewChallenge(scope model.AccountAuthLocalApiScope, clientInfo *pb.EventAccountLinkChallengeClientInfo) (id string, err error) {
	if s.app == nil {
		return "", ErrApplicationIsNotRunning
	}

	id, value, err := s.sessions.StartNewChallenge(scope, clientInfo)
	if err != nil {
		return "", err
	}
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountLinkChallenge{
		AccountLinkChallenge: &pb.EventAccountLinkChallenge{
			Challenge:  value,
			ClientInfo: clientInfo,
			Scope:      scope,
		},
	}))
	return id, nil
}

func (s *Service) LinkLocalSolveChallenge(req *pb.RpcAccountLocalLinkSolveChallengeRequest) (token string, appKey string, err error) {
	if s.app == nil {
		return "", "", ErrApplicationIsNotRunning
	}
	clientInfo, token, scope, err := s.sessions.SolveChallenge(req.ChallengeId, req.Answer, s.sessionSigningKey)
	if err != nil {
		return "", "", err
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	name := clientInfo.Name
	if name == "" {
		name = clientInfo.ProcessName
	}
	appInfo, err := wallet.PersistAppLink(name, scope)
	if err != nil {
		return token, appKey, err
	}

	s.lock.Lock()
	s.sessionsByAppHash[appInfo.AppHash] = token
	s.lock.Unlock()
	appKey = appInfo.AppKey
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountLinkChallengeHide{
		AccountLinkChallengeHide: &pb.EventAccountLinkChallengeHide{
			Challenge: req.Answer,
		},
	}))
	return
}

func (s *Service) LinkLocalCreateApp(req *pb.RpcAccountLocalLinkCreateAppRequest) (appKey string, err error) {
	if s.app == nil {
		return "", ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	appInfo, err := wallet.PersistAppLink(req.App.AppName, req.App.Scope)
	return appInfo.AppKey, err
}

func (s *Service) LinkLocalListApps() ([]*model.AccountAuthAppInfo, error) {
	if s.app == nil {
		return nil, ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	links, err := wallet.ListAppLinks()
	if err != nil {
		return nil, err
	}
	appsList := make([]*model.AccountAuthAppInfo, len(links))
	s.lock.RLock()
	defer s.lock.RUnlock()

	for i, app := range links {
		if app.AppName == "" {
			app.AppName = app.AppHash
		}
		_, isActive := s.sessionsByAppHash[app.AppHash]
		appsList[i] = &model.AccountAuthAppInfo{
			AppHash:   app.AppHash,
			AppName:   app.AppName,
			AppKey:    app.AppKey,
			CreatedAt: app.CreatedAt,
			ExpireAt:  app.ExpireAt,
			Scope:     model.AccountAuthLocalApiScope(app.Scope),
			IsActive:  isActive,
		}
	}
	return appsList, nil
}

func (s *Service) LinkLocalRevokeApp(req *pb.RpcAccountLocalLinkRevokeAppRequest) error {
	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	err := wallet.RevokeAppLink(req.AppHash)
	if err != nil {
		return err
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	if token, ok := s.sessionsByAppHash[req.AppHash]; ok {
		delete(s.sessionsByAppHash, req.AppHash)
		closeErr := s.sessions.CloseSession(token)
		if closeErr != nil {
			log.Errorf("error while closing session: %v", err)
		}
	}

	return err

}
