package application

import (
	"errors"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) AccountRecover() error {
	if s.mnemonic == "" {
		return ErrNoMnemonicProvided
	}

	res, err := core.WalletAccountAt(s.mnemonic, 0)
	if err != nil {
		return errors.Join(ErrBadInput, err)
	}

	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountShow{
		AccountShow: &pb.EventAccountShow{
			Account: &model.Account{
				Id: res.Identity.GetPublic().Account(),
			},
		},
	}))

	return nil
}
