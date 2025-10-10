package inboxclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	ic "github.com/anyproto/any-sync/coordinator/inboxclient"
	"github.com/anyproto/anytype-heart/core/wallet"
	"go.uber.org/zap"
)

const CName = "heart.inboxclient"

var log = logger.NewNamed(CName)

func New() ic.InboxClient {
	newIc := ic.New()
	return &inboxclient{InboxClient: newIc}
}

type inboxclient struct {
	ic.InboxClient

	mu     sync.Mutex
	offset string
	wallet wallet.Wallet
}

func (s *inboxclient) Init(a *app.App) (err error) {
	err = s.InboxClient.Init(a)
	if err != nil {
		return
	}
	s.wallet = app.MustComponent[wallet.Wallet](a)
	err = s.SetMessageReceiver(s.ReceiveNotify)
	if err != nil {
		return
	}

	return
}

func (s *inboxclient) Name() (name string) {
	return CName
}

func (s *inboxclient) Run(ctx context.Context) error {
	err := s.InboxClient.Run(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *inboxclient) fetchMessages() []*coordinatorproto.InboxMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

fetch:
	log.Debug("--inbox: fethching messages", zap.String("offset", s.offset))
	msgs, hasMore, err := s.InboxFetch(context.TODO(), s.offset)
	if err != nil {
		log.Error("--inbox: fetching after notify err", zap.Error(err))
	}
	if len(msgs) != 0 {
		// assuming that msgs are sorted
		s.offset = msgs[len(msgs)-1].Id
		for _, msg := range msgs {
			encrypted := msg.Packet.Payload.Body
			body, err := s.wallet.Account().SignKey.Decrypt(encrypted)
			if err != nil {
				log.Error("--inbox: error decrypting body", zap.Error(err))
			}
			msg.Packet.Payload.Body = body
		}
	}
	if hasMore {
		goto fetch
	}
	log.Debug("--inbox: final:", zap.String("offset", s.offset))

	return msgs

}

func (s *inboxclient) ReceiveNotify(event *coordinatorproto.NotifySubscribeEvent) {
	log.Debug("--inbox: got notify event", zap.String("event", fmt.Sprintf("%#v", event)))
	messages := s.fetchMessages()
	if len(messages) == 0 {
		log.Info("--inbox: ReceiveNotify: msgs len == 0")
	}
	for _, msg := range messages {
		log.Info("--inbox: got a message", zap.String("body", string(msg.Packet.Payload.Body)))
	}

}
func (s *inboxclient) Close(_ context.Context) (err error) {
	return nil
}
