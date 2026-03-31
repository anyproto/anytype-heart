package inboxsender

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/gogo/protobuf/proto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const (
	sendInviteIntervalSec = 30
	sendInviteTimeout     = 30 * time.Second
)

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

	spaceId, _, err := s.blockService.CreateOneToOneFromInbox(s.componentCtx, &identityProfileWithKey, spaceinfo.OneToOneInboxSentStatusReceived)
	if err != nil {
		log.Error("create onetoone space from inbox", zap.Error(err))
		return fmt.Errorf("processOneToOneInvite error: %s", err.Error())
	}
	err = s.blockService.SpaceInitChat(s.componentCtx, spaceId, true)
	if err != nil {
		log.Error("create onetoone space from inbox, SpaceInitChat", zap.String("spaceId", spaceId), zap.Error(err))
	}

	return err
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
