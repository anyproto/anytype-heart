package application

import (
	"errors"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) AccountRecover() error {
	if s.derivedKeys == nil {
		return errors.New("wallet not initialized")
	}

	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountShow{
		AccountShow: &pb.EventAccountShow{
			Account: &model.Account{
				Id: s.derivedKeys.Identity.GetPublic().Account(),
			},
		},
	}))

	return nil
}
