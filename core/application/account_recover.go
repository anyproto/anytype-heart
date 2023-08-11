package application

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"errors"
)

func (s *Service) AccountRecover() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.mnemonic == "" {
		return ErrNoMnemonicProvided
	}

	res, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return errors.Join(ErrBadInput, err)
	}

	event := &pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfAccountShow{
					AccountShow: &pb.EventAccountShow{
						Account: &model.Account{
							Id:   res.Identity.GetPublic().Account(),
							Name: "",
						},
					},
				},
			},
		},
	}
	s.eventSender.Broadcast(event)

	return nil
}
