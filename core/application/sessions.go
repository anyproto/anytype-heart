package application

import (
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
)

func (s *Service) CreateSession(req *pb.RpcWalletCreateSessionRequest) (token string, err error) {
	// test if mnemonic is correct
	mnemonic := req.GetMnemonic()
	appToken := req.GetAppToken()

	if appToken != "" {
		wallet := s.app.Component(walletComp.CName)
		if wallet == nil {
			return "", fmt.Errorf("appToken auth not yet supported for the main app")
		}
		w := wallet.(walletComp.Wallet)
		appLink, err := w.ReadAppLink(appToken)
		if err != nil {
			return "", err
		}
		log.Infof("appLink auth %s", appLink.AppName)
		return s.sessions.StartSession(s.sessionSigningKey)
	}
	if s.mnemonic != mnemonic {
		return "", errors.Join(ErrBadInput, fmt.Errorf("incorrect mnemonic"))
	}
	return s.sessions.StartSession(s.sessionSigningKey)
}

func (s *Service) CloseSession(req *pb.RpcWalletCloseSessionRequest) error {
	if sender, ok := s.eventSender.(session.Closer); ok {
		sender.CloseSession(req.Token)
	}
	return s.sessions.CloseSession(req.Token)
}

func (s *Service) ValidateSessionToken(token string) error {
	return s.sessions.ValidateToken(s.sessionSigningKey, token)
}

func (s *Service) LinkLocalStartNewChallenge(clientInfo *pb.EventAccountLinkChallengeClientInfo) (id string) {
	id, value := s.sessions.StartNewChallenge(clientInfo)
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfAccountLinkChallenge{
					AccountLinkChallenge: &pb.EventAccountLinkChallenge{
						Challenge:  value,
						ClientInfo: clientInfo,
					},
				},
			},
		},
	})
	return id
}

func (s *Service) LinkLocalSolveChallenge(req *pb.RpcAccountLocalLinkSolveChallengeRequest) (token string, appKey string, err error) {
	clientInfo, token, err := s.sessions.SolveChallenge(req.ChallengeId, req.Answer, s.sessionSigningKey)
	if err != nil {
		return "", "", err
	}
	wallet := s.app.Component(walletComp.CName).(walletComp.Wallet)
	appKey, err = wallet.PersistAppLink(&walletComp.AppLinkPayload{
		AppName:   clientInfo.ProcessName,
		AppPath:   clientInfo.ProcessPath,
		CreatedAt: time.Now().Unix(),
	})

	return
}
