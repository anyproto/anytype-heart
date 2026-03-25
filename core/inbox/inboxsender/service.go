package inboxsender

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inbox/inboxclient"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "heart.inboxsender"

var log = logger.NewNamed(CName)

var ErrSendPayloadInvite = errors.New("failed to send inbox payload")

// inboxPayloadTypeRegularInvite is a temporary payload type for space invites
// until any-sync is updated with the official enum value.
// TODO: remove this constant
const inboxPayloadTypeRegularInvite = coordinatorproto.InboxPayloadType(1)

const (
	sendInviteIntervalSec = 30
	sendInviteTimeout     = 30 * time.Second
)

// TODO: rewrite payload
type SpaceInvitePayload struct {
	SpaceId      string `json:"spaceId"`
	SpaceName    string `json:"spaceName"`
	SpaceIconCid string `json:"spaceIconCid,omitempty"`
	IconOption   int64  `json:"iconOption,omitempty"`
}

type blockService interface {
	SpaceInitChat(ctx context.Context, spaceId string, addAnalyticsId bool) error
	CreateOneToOneFromInbox(ctx context.Context, bobProfile *model.IdentityProfileWithKey, inviteSentStatus spaceinfo.OneToOneInboxSentStatus) (spaceID string, startingPageId string, err error)
}

type spaceService interface {
	TechSpace() *clientspace.TechSpace
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
		log.Error("failed to init inbox receiver", zap.Error(err))
		return err
	}
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
	return nil
}

func (s *inboxSender) processOneToOneInvite(packet *coordinatorproto.InboxPacket) (err error) {
	inboxBody := packet.Payload.Body

	if inboxBody == nil {
		return fmt.Errorf("processOneToOneInvite: got nil body")
	}

	var identityProfileWithKey model.IdentityProfileWithKey
	err = proto.Unmarshal(inboxBody, &identityProfileWithKey)
	if err != nil {
		return
	}

	spaceId, _, err := s.blockService.CreateOneToOneFromInbox(context.Background(), &identityProfileWithKey, spaceinfo.OneToOneInboxSentStatusReceived)
	if err != nil {
		log.Error("create onetoone space from inbox", zap.Error(err))
		return fmt.Errorf("processOneToOneInvite error: %s", err.Error())
	}
	err = s.blockService.SpaceInitChat(context.Background(), spaceId, true)
	if err != nil {
		log.Error("create onetoone space from inbox, SpaceInitChat", zap.String("spaceId", spaceId), zap.Error(err))
	}

	return err
}

func (s *inboxSender) processRegularSpaceInvite(packet *coordinatorproto.InboxPacket) error {
	if packet.Payload == nil || packet.Payload.Body == nil {
		return fmt.Errorf("processRegularSpaceInvite: got nil payload body")
	}

	var payload SpaceInvitePayload
	if err := json.Unmarshal(packet.Payload.Body, &payload); err != nil {
		return fmt.Errorf("unmarshal space invite payload: %w", err)
	}

	// TODO: trigger space join flow once any-sync API is updated
	return nil
}

func (s *inboxSender) SendRegularSpaceInvites(ctx context.Context, spaceId string, receiverIdentities ...string) error {
	description, err := s.getSpaceDescription(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space description: %w", err)
	}

	payload := SpaceInvitePayload{
		SpaceId:      spaceId,
		SpaceName:    description.Name,
		SpaceIconCid: description.IconImage,
		IconOption:   int64(description.IconOption),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal space invite payload: %w", err)
	}

	for _, identity := range receiverIdentities {
		if err = s.buildAndSendInvite(ctx, identity, inboxPayloadTypeRegularInvite, body); err != nil {
			return errors.Join(ErrSendPayloadInvite, err)
		}
	}

	return nil
}

func (s *inboxSender) getSpaceDescription(ctx context.Context, spaceId string) (spaceinfo.SpaceDescription, error) {
	var description spaceinfo.SpaceDescription
	err := s.techSpace.DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		description = spaceView.GetSpaceDescription()
		return nil
	})
	if err != nil {
		return spaceinfo.SpaceDescription{}, err
	}
	return description, nil
}

func (s *inboxSender) sendOneToOneInvite(ctx context.Context, receiverIdentity string) (err error) {
	myIdentity := s.accountService.Account().SignKey.GetPublic().Account()
	myProfile, err := s.identityService.WaitProfileWithKey(ctx, myIdentity)
	if err != nil {
		return
	}

	body, err := myProfile.Marshal()
	if err != nil {
		return
	}

	return s.buildAndSendInvite(ctx, receiverIdentity, coordinatorproto.InboxPayloadType_InboxPayloadOneToOneInvite, body)
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

// ResendFailedOneToOneInvites interrupts periodicInboxRetry, calls inboxResend
// and resets sendInviteInterval.
func (s *inboxSender) ResendFailedOneToOneInvites(ctx context.Context) error {
	return s.periodicInboxRetry.Reset(ctx)
}

// inboxResend runs periodically with sendInviteInterval, checks space views with invites which are not
// being sent yet (e.g. freshly created 1-1 spaces, failed inbox invites due to lack of network etc)
// and resend them.
//
// In case of success it sets space view inbox status to success.
func (s *inboxSender) inboxResend(ctx context.Context) (err error) {
	records, err := s.objectStore.SpaceIndex(s.techSpace.TechSpaceId()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyOneToOneInboxSentStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(spaceinfo.OneToOneInboxSentStatusToSend),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	})
	if err != nil {
		log.Error("inboxResend: failed to query type object", zap.Error(err))
		return
	}
	if len(records) == 0 {
		log.Info("inboxResend: no inbox invites to send, return")
		return
	}

	for _, record := range records {
		bobIdentity := record.Details.GetString(bundle.RelationKeyOneToOneIdentity)
		err := s.sendOneToOneInvite(ctx, bobIdentity)
		if err != nil {
			log.Error("inboxResend: error (re)sending inbox invite", zap.String("identity", bobIdentity), zap.Error(err))
		} else {
			spaceId := record.Details.GetString(bundle.RelationKeyTargetSpaceId)
			err = s.techSpace.DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
				return spaceView.SetOneToOneInboxInviteStatus(spaceinfo.OneToOneInboxSentStatusSent)
			})

			if err != nil {
				log.Error("inboxResend: error writing invite status to spaceView", zap.Error(err))
			}
		}
	}

	return
}
