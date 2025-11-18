package inboxclient

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	ic "github.com/anyproto/any-sync/coordinator/inboxclient"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/techspace"
	"go.uber.org/zap"
)

const CName = "heart.inboxclient"

var log = logger.NewNamed(CName)

type InboxClient interface {
	ic.InboxClient
	SetReceiverByType(payloadType coordinatorproto.InboxPayloadType, handler func(*coordinatorproto.InboxPacket) error) error
}

func New() InboxClient {
	newIc := ic.New()
	return &inboxclient{InboxClient: newIc}
}

type SpaceService interface {
	TechSpace() *clientspace.TechSpace
}

type inboxclient struct {
	ic.InboxClient

	spaceService SpaceService
	wallet       wallet.Wallet

	mu        sync.Mutex
	techSpace techspace.TechSpace

	rmu       sync.Mutex
	receivers map[coordinatorproto.InboxPayloadType]func(*coordinatorproto.InboxPacket) error

	periodicCheck periodicsync.PeriodicSync
}

func (s *inboxclient) Init(a *app.App) (err error) {
	err = s.InboxClient.Init(a)
	if err != nil {
		return
	}
	s.periodicCheck = periodicsync.NewPeriodicSync(50, 0, s.checkMessages, log)
	s.spaceService = app.MustComponent[SpaceService](a)
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
	s.techSpace = s.spaceService.TechSpace()
	if s.techSpace == nil {
		return fmt.Errorf("inboxclient: techspace is nil")
	}
	err := s.InboxClient.Run(ctx)
	if err != nil {
		return err
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

func (s *inboxclient) fetchMessages() (messages []*coordinatorproto.InboxMessage, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	messages = make([]*coordinatorproto.InboxMessage, 0)
	offset, err := s.getOffset()
	// TODO: set old offset here to rollback in case of error
	if err != nil {
		return
	}

	// TODO: What to do with error, when we've got a part/batch?
	// 1. we can try to process just batch, if offset is set correct
	// 2. if error was while trying to set an offset, we probably shouldn't process it
	//    because it will cause double processing
	// we can follow (2.), but this means that all processes should be able to double process messages
	// Both situations can potentially lead to an infinite fetch loop.
	for {
		batch, hasMore, err := s.InboxFetch(context.TODO(), offset)
		if err != nil {
			log.Error("inbox: fetchMessages batch error", zap.Error(err))
			break
		}

		if len(batch) > 0 {
			newOffset := batch[len(batch)-1].Id
			// TODO: do setoffset in the end
			err = s.setOffset(newOffset)
			if err != nil {
				log.Error("inbox: error setting offset", zap.Error(err))
				break
			}

			for i := range batch {
				encrypted := batch[i].Packet.Payload.Body
				body, err := s.wallet.Account().SignKey.Decrypt(encrypted)
				if err != nil {
					// skipping a message if we fail to decrypt
					log.Error("inbox: error decrypting body", zap.Error(err))
					continue
				}
				batch[i].Packet.Payload.Body = body
			}

			messages = append(messages, batch...)
		}

		if !hasMore {
			break
		}
	}

	return
}

func (s *inboxclient) checkMessages(ctx context.Context) (err error) {
	log.Warn("inbox: checkMessages")
	s.ReceiveNotify(&coordinatorproto.NotifySubscribeEvent{})
	return nil
}

func (s *inboxclient) ReceiveNotify(event *coordinatorproto.NotifySubscribeEvent) {
	messages, err := s.fetchMessages()
	if err != nil {

		log.Error("inbox: failed to get inbox offset", zap.Error(err))
		// TODO: return, don't process batch
		return
		// we don't return here in case we have a partial batch of messages
	}

	if len(messages) == 0 {
		log.Warn("inbox: ReceiveNotify: msgs len == 0")
		return
	}
	for _, msg := range messages {
		log.Warn("inbox: got a message", zap.String("type", coordinatorproto.InboxPayloadType_name[int32(msg.Packet.Payload.PayloadType)]))
		// TODO: verify signature (coordinator does it too but still)
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

func (s *inboxclient) Close(_ context.Context) (err error) {
	s.periodicCheck.Close()
	return nil
}
