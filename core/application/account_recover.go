package application

import (
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// AccountRecover broadcasts the recovered wallet's account as an accountShow
// event. Desktop no longer needs it -- WalletCreateSession answers with the
// account -- but it is still how swift and kotlin learn theirs, so it is not
// going away on its own schedule.
//
// Mobile reaches this through the gomobile bridge, which calls handlers with
// context.Background() and no auth interceptor: it holds no session token and
// never calls WalletCreateSession, so the synchronous answer is not reachable
// from there. Retiring this needs a mobile-visible replacement first (the
// account on a response they do call, e.g. WalletRecover) -- see GO-7495.
//
// Mobile is not exposed to the delivery race that made this unfit for desktop
// login (GO-7494): its sender is the in-process CallbackSender, so a broadcast
// always has a live sink, with no ListenSessionEvents stream to lose a race
// against.
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
