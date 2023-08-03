package account

import (
	"github.com/anyproto/anytype-heart/pb"
	"fmt"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
)

func (s *Service) CreateSession(req *pb.RpcWalletCreateSessionRequest) (token string, err error) {
	// test if mnemonic is correct
	if s.mnemonic != req.Mnemonic {
		return "", domain.WrapErrorWithCode(fmt.Errorf("incorrect mnemonic"), pb.RpcWalletCreateSessionResponseError_BAD_INPUT)
	}
	return s.sessions.StartSession(s.sessionKey)
}

func (s *Service) CloseSession(req *pb.RpcWalletCloseSessionRequest) error {
	if sender, ok := s.eventSender.(session.Closer); ok {
		sender.CloseSession(req.Token)
	}
	return s.sessions.CloseSession(req.Token)
}

func (s *Service) ValidateSessionToken(token string) error {
	return s.sessions.ValidateToken(s.sessionKey, token)
}
