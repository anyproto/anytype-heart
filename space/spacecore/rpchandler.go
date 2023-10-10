package spacecore

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore/clientspaceproto"
)

type rpcHandler struct {
	s *service
}

// nolint:revive
func (r *rpcHandler) AclAddRecord(ctx context.Context, request *spacesyncproto.AclAddRecordRequest) (*spacesyncproto.AclAddRecordResponse, error) {
	return nil, fmt.Errorf("nt implemented")
}

// nolint:revive
func (r *rpcHandler) AclGetRecords(ctx context.Context, request *spacesyncproto.AclGetRecordsRequest) (*spacesyncproto.AclGetRecordsResponse, error) {
	return nil, fmt.Errorf("nt implemented")
}

func (r *rpcHandler) ObjectSync(ctx context.Context, req *spacesyncproto.ObjectSyncMessage) (resp *spacesyncproto.ObjectSyncMessage, err error) {
	sp, err := r.s.Get(ctx, req.SpaceId)
	if err != nil {
		if err != spacesyncproto.ErrSpaceMissing {
			err = spacesyncproto.ErrUnexpected
		}
		return
	}
	resp, err = sp.HandleSyncRequest(ctx, req)
	return
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
			portAddrs = append(portAddrs, fmt.Sprintf("%spaceCore:%d", ip, request.LocalServer.Port))
		}
		r.s.peerService.SetPeerAddrs(peerId, portAddrs)
		r.s.peerStore.UpdateLocalPeer(peerId, request.SpaceIds)
		log.Info("updated local peer", zap.Strings("ips", portAddrs), zap.String("peerId", peerId), zap.Strings("spaceIds", request.SpaceIds))
	}
	log.Debug("returning list with ids", zap.Strings("spaceIds", allIds))
	resp = &clientspaceproto.SpaceExchangeResponse{SpaceIds: allIds}
	return
}

func (r *rpcHandler) SpacePull(ctx context.Context, request *spacesyncproto.SpacePullRequest) (resp *spacesyncproto.SpacePullResponse, err error) {
	sp, err := r.s.Get(ctx, request.Id)
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
	_, err = r.s.Get(ctx, description.SpaceHeader.GetId())
	if err != nil {
		return
	}
	resp = &spacesyncproto.SpacePushResponse{}
	return
}

func (r *rpcHandler) HeadSync(ctx context.Context, req *spacesyncproto.HeadSyncRequest) (*spacesyncproto.HeadSyncResponse, error) {
	sp, err := r.s.Get(ctx, req.SpaceId)
	if err != nil {
		return nil, spacesyncproto.ErrSpaceMissing
	}
	return sp.HandleRangeRequest(ctx, req)
}

func (r *rpcHandler) ObjectSyncStream(stream spacesyncproto.DRPCSpaceSync_ObjectSyncStreamStream) error {
	return r.s.streamPool.ReadStream(stream)
}
