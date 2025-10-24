package inboxclient

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	ic "github.com/anyproto/any-sync/coordinator/inboxclient"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"go.uber.org/zap"
)

const CName = "heart.inboxclient"

var log = logger.NewNamed(CName)

type InboxClient interface {
	ic.InboxClient
	SendOneToOneInvite(ctx context.Context, id *model.IdentityProfile) error
}

func New() InboxClient {
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
	msgs, hasMore, err := s.InboxFetch(context.TODO(), s.offset)
	if err != nil {
		log.Error("inbox: fetch error", zap.Error(err))
	}
	if len(msgs) != 0 {
		// assuming that msgs are sorted
		s.offset = msgs[len(msgs)-1].Id
		for _, msg := range msgs {
			encrypted := msg.Packet.Payload.Body
			body, err := s.wallet.Account().SignKey.Decrypt(encrypted)
			if err != nil {
				log.Error("inbox: error decrypting body", zap.Error(err))
			}
			msg.Packet.Payload.Body = body
		}
	}
	if hasMore {
		goto fetch
	}

	return msgs
}

func (s *inboxclient) ReceiveNotify(event *coordinatorproto.NotifySubscribeEvent) {
	messages := s.fetchMessages()
	if len(messages) == 0 {
		log.Warn("inbox: ReceiveNotify: msgs len == 0")
	}
	for _, msg := range messages {
		log.Warn("inbox: got a message", zap.String("type", coordinatorproto.InboxPayloadType_name[int32(msg.Packet.Payload.PayloadType)]))
		// TODO: verify signature?
		switch msg.Packet.Payload.PayloadType {
		case coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite:
			s.processOneToOneInvite(msg.Packet.Payload.Body)
		default:
			log.Warn("inbox: don't know how to process PayloadType", zap.Int("type", int(msg.Packet.Payload.PayloadType)))
		}
	}

}

func (s *inboxclient) Close(_ context.Context) (err error) {
	return nil
}

func (s *inboxclient) processOneToOneInvite(inboxBody []byte) (err error) {
	return nil
}

// TODO: inbox
// 1. wrap identity and request metadata key to payload
// 2. change spaceinvite to onetoone request
// 3. auto accept inbox
// 4. don't send inbox if space already exists (check spaceveiw)
// 5. on inbox accept, skip creation if space exist
func (s *inboxclient) SendOneToOneInvite(ctx context.Context, idWithProfileKey *model.IdentityProfile) (err error) {
	// 1. put whole identity profile
	// 2. try to get this from WaitProfile or register this incoming identity
	// 3. createOneToOne(bPk)
	var body []byte
	msg := &coordinatorproto.InboxMessage{
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			KeyType:          coordinatorproto.InboxKeyType_ed25519,
			ReceiverIdentity: idWithProfileKey.Identity,
			Payload: &coordinatorproto.InboxPayload{
				Body:        body,
				PayloadType: coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite,
			},
		},
	}

	bPk, err := crypto.DecodeAccountAddress(idWithProfileKey.Identity)
	if err != nil {
		return
	}

	return s.InboxAddMessage(ctx, bPk, msg)

}
