package application

import (
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// AccountRecover broadcasts the recovered wallet's account as an accountShow
// event.
//
// Deprecated: the account is returned synchronously by WalletCreateSession, and
// that is what the client should read. This is kept only until swift and kotlin
// are off the event (GO-7495); an event broadcast here is dropped outright if the
// caller's ListenSessionEvents stream has not attached yet, which is what used to
// wedge desktop login (GO-7494).
func (s *Service) AccountRecover() error {
	accountId := s.walletAccountId()
	if accountId == "" {
		return ErrWalletNotInitialized
	}

	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountShow{
		AccountShow: &pb.EventAccountShow{
			Account: &model.Account{
				Id: accountId,
			},
		},
	}))

	return nil
}
