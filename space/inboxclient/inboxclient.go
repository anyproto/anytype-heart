package inboxclient

import (
	"context"
	"fmt"
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
	SendOneToOneInvite(ctx context.Context, id *model.IdentityProfileWithKey) error
	SetReceiverByType(payloadType coordinatorproto.InboxPayloadType, handler func(*coordinatorproto.InboxPacket) error) error
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

	// TODO: add mb que for each reciever type
	rmu       sync.Mutex
	receivers map[coordinatorproto.InboxPayloadType]func(*coordinatorproto.InboxPacket) error
}

func (s *inboxclient) Init(a *app.App) (err error) {
	err = s.InboxClient.Init(a)
	if err != nil {
		return
	}
	s.wallet = app.MustComponent[wallet.Wallet](a)
	s.receivers = make(map[coordinatorproto.InboxPayloadType]func(*coordinatorproto.InboxPacket) error)
	err = s.SetMessageReceiver(s.ReceiveNotify)
	if err != nil {
		return
	}

	return
}

// 1. CreateInbox in Alice client;
// 2. Open bob client, it should fetch the inbox and onetone will be created automatically
func (s *inboxclient) SetReceiverByType(payloadType coordinatorproto.InboxPayloadType, handler func(*coordinatorproto.InboxPacket) error) error {
	s.rmu.Lock()
	defer s.rmu.Unlock()

	payloadTypeStr := coordinatorproto.InboxPayloadType_name[int32(payloadType)]
	if handler == nil {
		return fmt.Errorf("inbox: error registering receiver for type %s: handler must be a function but got nil", payloadTypeStr)
	}
	s.receivers[payloadType] = handler
	log.Info("inbox: registered receiver", zap.String("payloadType", payloadTypeStr))
	return nil
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
	all := make([]*coordinatorproto.InboxMessage, 0)

	for {
		msgs, hasMore, err := s.InboxFetch(context.TODO(), s.offset)
		if err != nil {
			log.Error("inbox: fetch error", zap.Error(err))
			break
		}

		if len(msgs) > 0 {
			s.offset = msgs[len(msgs)-1].Id

			for i := range msgs {
				encrypted := msgs[i].Packet.Payload.Body
				body, err := s.wallet.Account().SignKey.Decrypt(encrypted)
				if err != nil {
					log.Error("inbox: error decrypting body", zap.Error(err))
					continue
				}
				msgs[i].Packet.Payload.Body = body
			}

			all = append(all, msgs...)
		}

		if !hasMore {
			break
		}
	}

	return all
}

func (s *inboxclient) ReceiveNotify(event *coordinatorproto.NotifySubscribeEvent) {
	messages := s.fetchMessages()
	if len(messages) == 0 {
		log.Warn("inbox: ReceiveNotify: msgs len == 0")
	}
	for _, msg := range messages {
		log.Warn("inbox: got a message", zap.String("type", coordinatorproto.InboxPayloadType_name[int32(msg.Packet.Payload.PayloadType)]))
		// TODO: verify signature (coordinator does it too but still)
		if handler, ok := s.receivers[msg.Packet.Payload.PayloadType]; ok {
			herr := handler(msg.Packet)
			if herr != nil {
				log.Error("inbox: error while processing receiver handler", zap.Int("type", int(msg.Packet.Payload.PayloadType)))
			}
		} else {
			log.Warn("inbox: don't know how to process PayloadType", zap.Int("type", int(msg.Packet.Payload.PayloadType)))
		}
	}
}

func (s *inboxclient) Close(_ context.Context) (err error) {
	return nil
}

// TODO: move to somewhere, e.g. onetoone service
// 1. wrap identity and request metadata key to payload
// 2. change spaceinvite to onetoone request
// 3. auto accept inbox
// 4. don't send inbox if space already exists (check spaceveiw)
// 5. on inbox accept, skip creation if space exist
func (s *inboxclient) SendOneToOneInvite(ctx context.Context, idWithProfileKey *model.IdentityProfileWithKey) (err error) {
	// 1. put whole identity profile
	// 2. try to get this from WaitProfile or register this incoming identity
	// 3. createOneToOne(bPk)
	body, err := idWithProfileKey.Marshal()
	if err != nil {
		return
	}

	msg := &coordinatorproto.InboxMessage{
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			KeyType:          coordinatorproto.InboxKeyType_ed25519,
			ReceiverIdentity: idWithProfileKey.IdentityProfile.Identity,
			Payload: &coordinatorproto.InboxPayload{
				Body:        body,
				PayloadType: coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite,
			},
		},
	}

	participantPubKey, err := crypto.DecodeAccountAddress(idWithProfileKey.IdentityProfile.Identity)
	if err != nil {
		return
	}

	return s.InboxAddMessage(ctx, participantPubKey, msg)
}
