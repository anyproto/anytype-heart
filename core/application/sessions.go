package application

import (
	"fmt"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"errors"
)

func (s *Service) CreateSession(req *pb.RpcWalletCreateSessionRequest) (token string, err error) {
	// test if mnemonic is correct
	if s.mnemonic != req.Mnemonic {
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
