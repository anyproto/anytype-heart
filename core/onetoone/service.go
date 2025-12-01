package onetoone

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/periodicsync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inboxclient"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"

	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"
)

const CName = "heart.onetoone"

var log = logger.NewNamed(CName)

var (
	ErrSomeError = errors.New("some error")
)

const (
	sendInviteIntervalSec = 30
	sendInviteTimeout     = 30 * time.Second
)

type SpaceService interface {
	TechSpace() *clientspace.TechSpace
}

type IdentityService interface {
	AddIdentityProfile(identityProfile *model.IdentityProfile, key crypto.SymKey) error
	WaitProfileWithKey(ctx context.Context, identity string) (*model.IdentityProfileWithKey, error)
}

func New() Service {
	return new(onetoone)
}

type Service interface {
	app.ComponentRunnable
	SendOneToOneInvite(ctx context.Context, receiverIdentity string) (err error)
	ResendFailedOneToOneInvites(ctx context.Context) error
}

type BlockService interface {
	CreateOneToOneFromInbox(ctx context.Context, spaceDescription *spaceinfo.SpaceDescription, bobProfile *model.IdentityProfileWithKey) (err error)
	SpaceInitChat(ctx context.Context, spaceId string) error
}
type onetoone struct {
	inboxClient        inboxclient.InboxClient
	spaceService       SpaceService
	accountService     accountservice.Service
	objectStore        objectstore.ObjectStore
	identityService    IdentityService
	techSpace          techspace.TechSpace
	blockService       BlockService
	periodicInboxRetry periodicsync.PeriodicSync
}

func (s *onetoone) Init(a *app.App) (err error) {
	s.blockService = app.MustComponent[BlockService](a)
	s.spaceService = app.MustComponent[SpaceService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.identityService = app.MustComponent[IdentityService](a)
	s.inboxClient = app.MustComponent[inboxclient.InboxClient](a)
	s.periodicInboxRetry = periodicsync.NewPeriodicSync(sendInviteIntervalSec, sendInviteTimeout, s.inboxResend, log)
	err = s.inboxClient.SetReceiverByType(coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, s.processOneToOneInvite)
	if err != nil {
		log.Error("failed to init inbox receiver", zap.Error(err))
		return err
	}

	return
}

func (s *onetoone) Name() (name string) {
	return CName
}

func (s *onetoone) Run(ctx context.Context) error {
	s.techSpace = s.spaceService.TechSpace()
	if s.techSpace == nil {
		return fmt.Errorf("inboxclient: techspace is nil")
	}
	s.periodicInboxRetry.Run()
	return nil
}

func (s *onetoone) Close(_ context.Context) (err error) {
	if s.periodicInboxRetry != nil {
		s.periodicInboxRetry.Close()
	}

	return nil
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
	log.Warn("creating onetoone space from inbox.. ")

	key, err := crypto.UnmarshallAESKeyProto(identityProfileWithKey.RequestMetadata)
	if err != nil {
		return
	}

	// TODO: send encrypted rawProfile in inbox, with key?
	err = s.identityService.AddIdentityProfile(identityProfileWithKey.IdentityProfile, key)
	if err != nil {
		return
	}

	spaceDescription := &spaceinfo.SpaceDescription{
		Name:             identityProfileWithKey.IdentityProfile.Name,
		IconImage:        identityProfileWithKey.IdentityProfile.IconCid,
		SpaceUxType:      model.SpaceUxType_OneToOne,
		OneToOneIdentity: identityProfileWithKey.IdentityProfile.Identity,
	}

	err = s.blockService.CreateOneToOneFromInbox(context.TODO(), spaceDescription, &identityProfileWithKey)
	if err != nil {
		log.Error("create onetoone space from inbox", zap.Error(err))
		return fmt.Errorf("processOneToOneInvite error: %s", err.Error())
	}

	return err
}

func (s *onetoone) SendOneToOneInvite(ctx context.Context, receiverIdentity string) (err error) {
	myIdentity := s.accountService.Account().SignKey.GetPublic().Account()
	myProfile, err := s.identityService.WaitProfileWithKey(ctx, myIdentity)
	if err != nil {
		return
	}

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

// ResendFailedOneToOneInvites interrupts periodicInboxRetry, calls inboxResend
// and resets sendInviteInterval.
func (s *onetoone) ResendFailedOneToOneInvites(ctx context.Context) error {
	return s.periodicInboxRetry.Reset(ctx)
}

// inboxResend runs periodically with sendInviteInterval, checks space views with invites which are not
// being sent yet (e.g. freshly created 1-1 spaces, failed inbox invites due to lack of network etc)
// and resend them.
//
// In case of success it sets space view inbox status to success.
func (s *onetoone) inboxResend(ctx context.Context) (err error) {
	records, err := s.objectStore.SpaceIndex(s.techSpace.TechSpaceId()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyOneToOneInboxSentStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(spaceinfo.OneToOneInboxSentStatus_ToSend),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	})
	if err != nil {
		log.Error("onetoone: inboxResend: failed to query type object", zap.Error(err))
		return
	}
	if len(records) == 0 {
		log.Info("onetoone: inboxResend: no inbox invites to send, return")
		return
	}

	for _, record := range records {
		bobIdentity := record.Details.GetString(bundle.RelationKeyOneToOneIdentity)
		err := s.SendOneToOneInvite(context.TODO(), bobIdentity)
		if err != nil {
			log.Error("inboxResend: error (re)sending inbox invite", zap.String("identity", bobIdentity), zap.Error(err))
		} else {
			spaceId := record.Details.GetString(bundle.RelationKeyTargetSpaceId)
			err = s.techSpace.DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
				return spaceView.SetOneToOneInboxInviteStatus(spaceinfo.OneToOneInboxSentStatus_Sent)
			})

			if err != nil {
				log.Error("inboxResend: error writing invite status to spaceView", zap.Error(err))
			}
		}
	}

	return
}
