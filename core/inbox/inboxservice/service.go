package inboxservice

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/periodicsync"

	"github.com/anyproto/anytype-heart/core/inbox/inboxclient"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "heart.inboxsender"

var log = logger.NewNamed(CName)

type blockService interface {
	SpaceInitChat(ctx context.Context, spaceId string, addAnalyticsId bool) error
	CreateOneToOneFromInbox(ctx context.Context, bobProfile *model.IdentityProfileWithKey, inviteSentStatus spaceinfo.OneToOneInboxSentStatus) (spaceID string, startingPageId string, err error)
}

type spaceService interface {
	TechSpace() *clientspace.TechSpace
	InviteJoin(ctx context.Context, id, aclHeadId string) error
}

type identityService interface {
	AddIdentityProfile(identityProfile *model.IdentityProfile, key crypto.SymKey) error
	WaitProfileWithKey(ctx context.Context, identity string) (*model.IdentityProfileWithKey, error)
}

func New() Sender {
	return new(inboxSender)
}

type Sender interface {
	app.ComponentRunnable
	SendRegularSpaceInvites(ctx context.Context, spaceId string, receiverIdentities ...string) error
	ResendFailedOneToOneInvites(ctx context.Context) error
}

type inboxSender struct {
	inboxClient     inboxclient.InboxClient
	spaceService    spaceService
	blockService    blockService
	identityService identityService
	accountService  accountservice.Service
	objectStore     objectstore.ObjectStore

	techSpace          techspace.TechSpace
	periodicInboxRetry periodicsync.PeriodicSync

	componentCtx    context.Context
	componentCancel context.CancelFunc
}

func (s *inboxSender) Init(a *app.App) (err error) {
	s.inboxClient = app.MustComponent[inboxclient.InboxClient](a)
	s.spaceService = app.MustComponent[spaceService](a)
	s.blockService = app.MustComponent[blockService](a)
	s.identityService = app.MustComponent[identityService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)

	s.periodicInboxRetry = periodicsync.NewPeriodicSync(sendInviteIntervalSec, sendInviteTimeout, s.inboxResend, log)
	err = s.inboxClient.SetReceiverByType(inboxPayloadTypeRegularInvite, s.processRegularSpaceInvite)
	if err != nil {
		return fmt.Errorf("register inbox receiver: %w", err)
	}
	err = s.inboxClient.SetReceiverByType(coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, s.processOneToOneInvite)
	if err != nil {
		return fmt.Errorf("register onetoone inbox receiver: %w", err)
	}

	s.componentCtx, s.componentCancel = context.WithCancel(context.Background())
	return nil
}

func (s *inboxSender) Name() (name string) {
	return CName
}

func (s *inboxSender) Run(ctx context.Context) error {
	s.techSpace = s.spaceService.TechSpace()
	if s.techSpace == nil {
		return fmt.Errorf("inboxsender: techspace is nil")
	}
	s.periodicInboxRetry.Run()
	return nil
}

func (s *inboxSender) Close(_ context.Context) (err error) {
	if s.periodicInboxRetry != nil {
		s.periodicInboxRetry.Close()
	}
	if s.componentCancel != nil {
		s.componentCancel()
	}
	return nil
}

func (s *inboxSender) buildAndSendInvite(ctx context.Context, receiverIdentity string, payloadType coordinatorproto.InboxPayloadType, body []byte) error {
	msg := &coordinatorproto.InboxMessage{
		PacketType: coordinatorproto.InboxPacketType_Default,
		Packet: &coordinatorproto.InboxPacket{
			KeyType:          coordinatorproto.InboxKeyType_ed25519,
			ReceiverIdentity: receiverIdentity,
			Payload: &coordinatorproto.InboxPayload{
				Body:        body,
				PayloadType: payloadType,
			},
		},
	}

	receiverPubKey, err := crypto.DecodeAccountAddress(receiverIdentity)
	if err != nil {
		return fmt.Errorf("decode receiver identity: %w", err)
	}

	return s.inboxClient.InboxAddMessage(ctx, receiverPubKey, msg)
}
