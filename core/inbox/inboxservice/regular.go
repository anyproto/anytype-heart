package inboxservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

var ErrSendPayloadInvite = errors.New("failed to send inbox payload")

// inboxPayloadTypeRegularInvite is a temporary payload type for space invites
// until any-sync is updated with the official enum value.
// TODO: remove this constant
const inboxPayloadTypeRegularInvite = coordinatorproto.InboxPayloadType(1)

type spaceInvitePayload struct {
	SpaceId      string `json:"spaceId"`
	SpaceName    string `json:"spaceName"`
	SpaceIconCid string `json:"spaceIconCid,omitempty"`
	IconOption   int64  `json:"iconOption,omitempty"`
}

func (s *inboxSender) processRegularSpaceInvite(packet *coordinatorproto.InboxPacket) error {
	if packet.Payload == nil || packet.Payload.Body == nil {
		return fmt.Errorf("processRegularSpaceInvite: got nil payload body")
	}

	var payload spaceInvitePayload
	if err := json.Unmarshal(packet.Payload.Body, &payload); err != nil {
		return fmt.Errorf("unmarshal space invite payload: %w", err)
	}

	if err := s.spaceService.InviteJoin(s.componentCtx, payload.SpaceId, ""); err != nil {
		log.Error("join regular space from inbox", zap.String("spaceId", payload.SpaceId), zap.Error(err))
		return fmt.Errorf("invite join space %s: %w", payload.SpaceId, err)
	}
	err := s.techSpace.SpaceViewSetData(s.componentCtx, payload.SpaceId,
		domain.NewDetails().
			SetString(bundle.RelationKeyName, payload.SpaceName).
			SetString(bundle.RelationKeyIconImage, payload.SpaceIconCid).
			SetInt64(bundle.RelationKeyIconOption, payload.IconOption))
	if err != nil {
		log.Error("set space view details", zap.Error(err))
		return fmt.Errorf("set space view data: %w", err)
	}
	return nil
}

func (s *inboxSender) SendRegularSpaceInvites(ctx context.Context, spaceId string, receiverIdentities ...string) error {
	description, err := s.getSpaceDescription(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space description: %w", err)
	}

	payload := spaceInvitePayload{
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
