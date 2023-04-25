package space

import (
	"context"
	"fmt"

	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/net/peer"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/space/clientspaceproto"
)

type rpcHandler struct {
	s *service
}

func (r *rpcHandler) SpaceExchange(ctx context.Context, request *clientspaceproto.SpaceExchangeRequest) (resp *clientspaceproto.SpaceExchangeResponse, err error) {
	allIds, err := r.s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return
	}
	if request.LocalServer != nil {
		peerId, err := peer.CtxPeerId(ctx)
		if err != nil {
			return nil, err
		}
		var portAddrs []string
		for _, ip := range request.LocalServer.Ips {
			portAddrs = append(portAddrs, fmt.Sprintf("%s:%d", ip, request.LocalServer.Port))
		}
		r.s.dialer.SetPeerAddrs(peerId, portAddrs)
		r.s.peerStore.UpdateLocalPeer(peerId, request.SpaceIds)
		log.Info("updated local peer", zap.Strings("ips", portAddrs), zap.String("peerId", peerId), zap.Strings("spaceIds", request.SpaceIds))
	}
	log.Debug("returning list with ids", zap.Strings("spaceIds", allIds))
	resp = &clientspaceproto.SpaceExchangeResponse{SpaceIds: allIds}
	return
}

func (r *rpcHandler) SpacePull(ctx context.Context, request *spacesyncproto.SpacePullRequest) (resp *spacesyncproto.SpacePullResponse, err error) {
	sp, err := r.s.GetSpace(ctx, request.Id)
	if err != nil {
		if err != spacesyncproto.ErrSpaceMissing {
			err = spacesyncproto.ErrUnexpected
		}
		return
	}

	spaceDesc, err := sp.Description()
	if err != nil {
		err = spacesyncproto.ErrUnexpected
		return
	}

	resp = &spacesyncproto.SpacePullResponse{
		Payload: &spacesyncproto.SpacePayload{
			SpaceHeader:            spaceDesc.SpaceHeader,
			AclPayloadId:           spaceDesc.AclId,
			AclPayload:             spaceDesc.AclPayload,
			SpaceSettingsPayload:   spaceDesc.SpaceSettingsPayload,
			SpaceSettingsPayloadId: spaceDesc.SpaceSettingsId,
		},
	}
	return
}

func (r *rpcHandler) SpacePush(ctx context.Context, req *spacesyncproto.SpacePushRequest) (resp *spacesyncproto.SpacePushResponse, err error) {
	description := commonspace.SpaceDescription{
		SpaceHeader:          req.Payload.SpaceHeader,
		AclId:                req.Payload.AclPayloadId,
		AclPayload:           req.Payload.AclPayload,
		SpaceSettingsPayload: req.Payload.SpaceSettingsPayload,
		SpaceSettingsId:      req.Payload.SpaceSettingsPayloadId,
	}
	ctx = context.WithValue(ctx, commonspace.AddSpaceCtxKey, description)
	_, err = r.s.GetSpace(ctx, description.SpaceHeader.GetId())
	if err != nil {
		return
	}
	resp = &spacesyncproto.SpacePushResponse{}
	return
}

func (r *rpcHandler) HeadSync(ctx context.Context, req *spacesyncproto.HeadSyncRequest) (*spacesyncproto.HeadSyncResponse, error) {
	sp, err := r.s.GetSpace(ctx, req.SpaceId)
	if err != nil {
		return nil, spacesyncproto.ErrSpaceMissing
	}
	return sp.HeadSync().HandleRangeRequest(ctx, req)
}

func (r *rpcHandler) ObjectSyncStream(stream spacesyncproto.DRPCSpaceSync_ObjectSyncStreamStream) error {
	return r.s.streamPool.ReadStream(stream)
}
