package spacecore

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"

	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"go.uber.org/zap"
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
	return nil, fmt.Errorf("nt implemented")
}

func (r *rpcHandler) ObjectSyncRequestStream(req *spacesyncproto.ObjectSyncMessage, stream spacesyncproto.DRPCSpaceSync_ObjectSyncRequestStreamStream) (err error) {
	sp, err := r.s.Get(stream.Context(), req.SpaceId)
	if err != nil {
		return err
	}
	return sp.HandleStreamSyncRequest(stream.Context(), req, stream)
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
		r.s.recordLocalPeer(ctx, peerId, request.LocalServer, request.SpaceIds, false)
	}
	log.Debug("returning list with ids", zap.Strings("spaceIds", allIds))
	resp = &clientspaceproto.SpaceExchangeResponse{SpaceIds: allIds}
	return
}

// recordLocalPeer registers the addresses a LAN peer advertised and the spaces
// it shares with us, so subsequent syncing dials it directly. proof says
// whether spaceIds came out of a v2 token exchange (a v1 list is plaintext).
func (s *service) recordLocalPeer(ctx context.Context, peerId string, localServer *clientspaceproto.LocalServer, spaceIds []string, proof bool) {
	var portAddrs []string
	peerAddr := peer.CtxPeerAddr(ctx)

	if peerAddr != "" {
		// prioritize the address the remote peer connected us from — but
		// with its advertised listening port: the port in peerAddr is the
		// dialer's ephemeral source port, nothing listens on it
		if u, errParse := url.Parse(peerAddr); errParse == nil && u.Hostname() != "" {
			portAddrs = append(portAddrs, net.JoinHostPort(u.Hostname(), strconv.Itoa(int(localServer.Port))))
		}
	}

	for _, ip := range localServer.Ips {
		addr := fmt.Sprintf("%s:%d", ip, localServer.Port)
		if slices.Contains(portAddrs, addr) {
			continue
		}
		portAddrs = append(portAddrs, addr)
	}
	// addSchema pins the transport for local peers (yamux); see its comment
	addrsWithSchema := s.addSchema(portAddrs)
	s.peerService.SetPeerAddrs(peerId, addrsWithSchema)
	s.publishLocalPeer(peerId, spaceIds, proof)
	log.Info("updated local peer", zap.Strings("ips", addrsWithSchema), zap.String("peerId", peerId), zap.Strings("spaceIds", spaceIds))
}

// SpaceExchangeV2 is the inbound side of the token handshake: compute this
// device's expected request token for every space it holds a discovery key
// for, intersect with what the caller sent, and answer with membership proofs
// for the intersection only — keyed by the caller's nonce, so a response can
// be neither precomputed nor replayed.
func (r *rpcHandler) SpaceExchangeV2(ctx context.Context, request *clientspaceproto.SpaceExchangeV2Request) (*clientspaceproto.SpaceExchangeV2Response, error) {
	callerPeerId, err := peer.CtxPeerId(ctx)
	if err != nil {
		return nil, fmt.Errorf("caller peer id: %w", err)
	}
	if len(request.Nonce) != clientspaceproto.NonceSizeV2 {
		return nil, fmt.Errorf("space exchange v2: bad nonce size %d", len(request.Nonce))
	}
	if len(request.SpaceTokens) > clientspaceproto.MaxTokensV2 {
		return nil, fmt.Errorf("space exchange v2: too many tokens (%d)", len(request.SpaceTokens))
	}
	allIds, err := r.s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return nil, fmt.Errorf("all space ids: %w", err)
	}
	selfPeerId := r.s.wallet.GetDevicePrivkey().GetPublic().PeerId()
	keys := r.s.discoveryKeys.DiscoveryKeys(ctx, allIds)
	// we are the responder, so the pair is ordered (them, us)
	shared, respTokens := respondTokens(allIds, keys, request.SpaceTokens, request.Nonce, callerPeerId, selfPeerId)
	// a request without LocalServer is a plain probe: answer the proofs but
	// record nothing
	if request.LocalServer != nil {
		r.s.recordLocalPeer(ctx, callerPeerId, request.LocalServer, shared, true)
	}
	log.Debug("space exchange v2 received", zap.String("peerId", callerPeerId), zap.Int("shared", len(shared)))
	return &clientspaceproto.SpaceExchangeV2Response{SpaceTokens: respTokens}, nil
}

func (r *rpcHandler) SpacePull(ctx context.Context, request *spacesyncproto.SpacePullRequest) (resp *spacesyncproto.SpacePullResponse, err error) {
	// GO-7492: an account peer may ask for any space we might hold. Answer
	// from disk: Get would load the space and, if absent, pull it on the
	// caller's behalf — which recurses back to a caller whose own load of
	// that space is what is waiting on us.
	if !r.s.spaceStorageProvider.SpaceExists(request.Id) {
		return nil, spacesyncproto.ErrSpaceMissing
	}
	sp, err := r.s.Get(ctx, request.Id)
	if err != nil {
		if err != spacesyncproto.ErrSpaceMissing {
			err = spacesyncproto.ErrUnexpected
		}
		return
	}

	spaceDesc, err := sp.Description(ctx)
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
	return r.s.streamPool.ReadStream(stream, 300)
}

func (r *rpcHandler) StoreDiff(ctx context.Context, req *spacesyncproto.StoreDiffRequest) (*spacesyncproto.StoreDiffResponse, error) {
	space, err := r.s.Get(ctx, req.SpaceId)
	if err != nil {
		return nil, fmt.Errorf("get space: %w", err)
	}
	return space.KeyValue().HandleStoreDiffRequest(ctx, req)
}

func (r *rpcHandler) StoreElements(stream spacesyncproto.DRPCSpaceSync_StoreElementsStream) error {
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv first message: %w", err)
	}

	ctx := context.Background()
	space, err := r.s.Get(ctx, msg.SpaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}
	return space.KeyValue().HandleStoreElementsRequest(ctx, stream)
}
