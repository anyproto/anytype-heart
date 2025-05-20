package application

import (
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) CreateSession(req *pb.RpcWalletCreateSessionRequest) (token string, accountId string, err error) {
	// test if mnemonic is correct
	mnemonic := req.GetMnemonic()
	appKey := req.GetAppKey()

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
		return token, w.Account().SignKey.GetPublic().Account(), nil
	}

	if s.mnemonic == "" {
		// todo: rewrite this after appKey auth is implemented
		// we can derive and check the account in this case
		return "", "", errors.Join(ErrBadInput, fmt.Errorf("app authed without mnemonic"))
	}
	if s.mnemonic != mnemonic {
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

func (s *Service) LinkLocalStartNewChallenge(scope model.AccountAuthLocalApiScope, clientInfo *pb.EventAccountLinkChallengeClientInfo, name string) (id string, err error) {
	if s.app == nil {
		return "", ErrApplicationIsNotRunning
	}

	id, value, err := s.sessions.StartNewChallenge(scope, clientInfo, name)
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
	appKey, err = wallet.PersistAppLink(&walletComp.AppLinkInfo{
		AppName:   clientInfo.ProcessName,
		CreatedAt: time.Now().Unix(),
		Scope:     int(scope),
	})

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
	appKey, err = wallet.PersistAppLink(&walletComp.AppLinkInfo{
		AppName:   req.App.AppName,
		CreatedAt: time.Now().Unix(),
		Scope:     int(req.App.Scope),
	})

	return
}

func (s *Service) LinkLocalListApps() ([]*walletComp.AppLinkInfo, error) {
	if s.app == nil {
		return nil, ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	return wallet.ListAppLinks()
}

func (s *Service) LinkLocalRevokeApp(req *pb.RpcAccountLocalLinkRevokeAppRequest) error {
	if s.app == nil {
		return ErrApplicationIsNotRunning
	}

	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	return wallet.RevokeAppLink(req.AppHash)

}
