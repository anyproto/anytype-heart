package inboxclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	anysyncinboxclient "github.com/anyproto/any-sync/coordinator/inboxclient"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/periodicsync"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/techspace"

	"go.uber.org/zap"
)

const CName = "heart.inboxclient"

var log = logger.NewNamed(CName)

type InboxClient interface {
	app.ComponentRunnable
	SetReceiverByType(payloadType coordinatorproto.InboxPayloadType, handler func(*coordinatorproto.InboxPacket) error) error
	InboxAddMessage(ctx context.Context, receiverPubKey crypto.PubKey, message *coordinatorproto.InboxMessage) (err error)
}

func New() InboxClient {
	return &inboxclient{}
}

type SpaceService interface {
	TechSpace() *clientspace.TechSpace
}

type inboxclient struct {
	inboxClient  anysyncinboxclient.InboxClient
	spaceService SpaceService
	wallet       wallet.Wallet

	mu        sync.Mutex
	techSpace techspace.TechSpace

	rmu       sync.Mutex
	receivers map[coordinatorproto.InboxPayloadType]func(*coordinatorproto.InboxPacket) error

	periodicCheck periodicsync.PeriodicSync
}

func (s *inboxclient) Init(a *app.App) (err error) {
	s.periodicCheck = periodicsync.NewPeriodicSync(300, 0, s.checkMessages, log)
	s.spaceService = app.MustComponent[SpaceService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)

	s.receivers = make(map[coordinatorproto.InboxPayloadType]func(*coordinatorproto.InboxPacket) error)
	s.inboxClient = app.MustComponent[anysyncinboxclient.InboxClient](a)
	err = s.inboxClient.SetMessageReceiver(s.ReceiveNotify)
	if err != nil {
		return
	}

	return
}

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
	s.techSpace = s.spaceService.TechSpace()
	if s.techSpace == nil {
		return fmt.Errorf("inboxclient: techspace is nil")
	}
	s.periodicCheck.Run()
	return nil
}

func (s *inboxclient) setOffset(offset string) (err error) {
	err = s.techSpace.DoAccountObject(context.Background(), func(accountObject techspace.AccountObject) error {
		err := accountObject.SetInboxOffset(offset)
		if err != nil {
			return err
		}
		return nil
	})
	return

}

func (s *inboxclient) getOffset() (offset string, err error) {
	err = s.techSpace.DoAccountObject(context.Background(), func(accountObject techspace.AccountObject) error {
		offset, err = accountObject.GetInboxOffset()
		if err != nil {
			return err
		}
		return nil
	})
	return
}

func (s *inboxclient) verifyPacketSignature(packet *coordinatorproto.InboxPacket) error {
	senderIdentity, err := crypto.DecodeAccountAddress(packet.SenderIdentity)
	if err != nil {
		return fmt.Errorf("decode sender identity: %w", err)
	}

	ok, err := senderIdentity.Verify(packet.Payload.Body, packet.SenderSignature)
	if err != nil {
		return fmt.Errorf("verify signature: %w", err)
	}
	if !ok {
		return fmt.Errorf("signature is invalid")
	}

	return nil
}

func (s *inboxclient) fetchMessages() (messages []*coordinatorproto.InboxMessage, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages = make([]*coordinatorproto.InboxMessage, 0)

	offset, err := s.getOffset()
	oldOffset := offset
	if err != nil {
		return
	}

	for {
		batch, hasMore, err := s.inboxClient.InboxFetch(context.Background(), offset)
		if err != nil {
			offset = oldOffset
			log.Error("inbox: fetchMessages batch error", zap.Error(err))
			break
		}

		if len(batch) > 0 {
			offset = batch[len(batch)-1].Id
			for i := range batch {
				// verify signature before decryption
				if err := s.verifyPacketSignature(batch[i].Packet); err != nil {
					log.Error("inbox: signature verification failed", zap.Error(err))
					continue
				}

				encrypted := batch[i].Packet.Payload.Body
				body, err := s.wallet.Account().SignKey.Decrypt(encrypted)
				if err != nil {
					// skipping a message if we fail to decrypt
					log.Error("inbox: error decrypting body", zap.Error(err))
					continue
				}
				batch[i].Packet.Payload.Body = body

				// only add successfully verified and decrypted messages
				messages = append(messages, batch[i])
			}
		}

		if !hasMore {
			break
		}
	}

	if offset != oldOffset {
		err = s.setOffset(offset)
		if err != nil {
			log.Error("inbox: error setting offset", zap.Error(err))
			return
		}
	}

	return
}

func (s *inboxclient) checkMessages(ctx context.Context) (err error) {
	s.ReceiveNotify(&coordinatorproto.NotifySubscribeEvent{})
	return nil
}

func (s *inboxclient) ReceiveNotify(event *coordinatorproto.NotifySubscribeEvent) {
	messages, err := s.fetchMessages()
	if err != nil {
		log.Error("inbox: failed to fetch messages", zap.Error(err))
		return
	}

	if len(messages) == 0 {
		log.Warn("inbox: ReceiveNotify: msgs len == 0")
		return
	}
	for _, msg := range messages {
		log.Info("inbox: got a message", zap.String("type", coordinatorproto.InboxPayloadType_name[int32(msg.Packet.Payload.PayloadType)]))
		if handler, ok := s.receivers[msg.Packet.Payload.PayloadType]; ok {
			herr := handler(msg.Packet)
			if herr != nil {
				log.Error("inbox: error while processing receiver handler", zap.String("type", coordinatorproto.InboxPayloadType_name[int32(msg.Packet.Payload.PayloadType)]), zap.Error(herr))
			}
		} else {
			log.Warn("inbox: don't know how to process PayloadType", zap.Int("type", int(msg.Packet.Payload.PayloadType)))
		}
	}
}

func (s *inboxclient) InboxAddMessage(ctx context.Context, receiverPubKey crypto.PubKey, message *coordinatorproto.InboxMessage) (err error) {
	return s.inboxClient.InboxAddMessage(ctx, receiverPubKey, message)
}
func (s *inboxclient) Close(_ context.Context) (err error) {
	s.periodicCheck.Close()
	return nil
}
