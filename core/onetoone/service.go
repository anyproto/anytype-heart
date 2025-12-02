package onetoone

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/inboxclient"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"
)

const CName = "heart.onetoone"

var log = logger.NewNamed(CName)

var (
	ErrSomeError = errors.New("some error")
)

type IdentityService interface {
	AddIdentityProfile(identityProfile *model.IdentityProfile, key crypto.SymKey) error
}

func New() Service {
	return new(onetoone)
}

type Service interface {
	app.ComponentRunnable
	SendOneToOneInvite(ctx context.Context, receiverIdentity string, myProfile *model.IdentityProfileWithKey) (err error)
}

type BlockService interface {
	CreateOneToOneFromInbox(ctx context.Context, bobProfile *model.IdentityProfileWithKey) (spaceID string, startingPageId string, err error)
	SpaceInitChat(ctx context.Context, spaceId string) error
}
type onetoone struct {
	inboxClient     inboxclient.InboxClient
	identityService IdentityService
	blockService    BlockService
}

func (s *onetoone) Init(a *app.App) (err error) {
	s.blockService = app.MustComponent[BlockService](a)
	s.identityService = app.MustComponent[IdentityService](a)
	s.inboxClient = app.MustComponent[inboxclient.InboxClient](a)
	err = s.inboxClient.SetReceiverByType(coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, s.processOneToOneInvite)
	if err != nil {
		log.Error("failed to init inbox receiver", zap.Error(err))
		return err
	}

	return
}

func (s *onetoone) processOneToOneInvite(packet *coordinatorproto.InboxPacket) (err error) {
	inboxBody := packet.Payload.Body

	if inboxBody == nil {
		return fmt.Errorf("processOneToOneInvite: got nil body")
	}

	var identityProfileWithKey model.IdentityProfileWithKey
	err = proto.Unmarshal(inboxBody, &identityProfileWithKey)
	if err != nil {
		return
	}

	_, _, err = s.blockService.CreateOneToOneFromInbox(context.TODO(), &identityProfileWithKey)
	if err != nil {
		log.Error("create onetoone space from inbox", zap.Error(err))
		return fmt.Errorf("processOneToOneInvite error: %s", err.Error())
	}

	return err
}
func (s *onetoone) Name() (name string) {
	return CName
}

func (s *onetoone) Run(ctx context.Context) error {
	return nil
}

func (s *onetoone) Close(_ context.Context) (err error) {
	return nil
}

func (s *onetoone) SendOneToOneInvite(ctx context.Context, receiverIdentity string, myProfile *model.IdentityProfileWithKey) (err error) {
	body, err := myProfile.Marshal()
	if err != nil {
		return
	}

	msg := &coordinatorproto.InboxMessage{
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			KeyType:          coordinatorproto.InboxKeyType_ed25519,
			ReceiverIdentity: receiverIdentity,
			Payload: &coordinatorproto.InboxPayload{
				Body:        body,
				PayloadType: coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite,
			},
		},
	}

	receiverPubKey, err := crypto.DecodeAccountAddress(receiverIdentity)
	if err != nil {
		return
	}

	return s.inboxClient.InboxAddMessage(ctx, receiverPubKey, msg)
}
