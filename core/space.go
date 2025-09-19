package core

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/core/order"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/encode"
)

func (mw *Middleware) SpaceDelete(cctx context.Context, req *pb.RpcSpaceDeleteRequest) *pb.RpcSpaceDeleteResponse {
	spaceService := mustService[space.Service](mw)
	err := spaceService.Delete(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(techspace.ErrSpaceViewNotExists, pb.RpcSpaceDeleteResponseError_SPACE_IS_DELETED),
	)
	return &pb.RpcSpaceDeleteResponse{
		Error: &pb.RpcSpaceDeleteResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceMakeShareable(cctx context.Context, req *pb.RpcSpaceMakeShareableRequest) *pb.RpcSpaceMakeShareableResponse {
	aclService := mustService[acl.AclService](mw)
	err := aclService.MakeShareable(cctx, req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(space.ErrSpaceDeleted, pb.RpcSpaceMakeShareableResponseError_SPACE_IS_DELETED),
			errToCode(space.ErrSpaceNotExists, pb.RpcSpaceMakeShareableResponseError_NO_SUCH_SPACE),
			errToCode(acl.ErrPersonalSpace, pb.RpcSpaceMakeShareableResponseError_BAD_INPUT),
			errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceMakeShareableResponseError_REQUEST_FAILED),
			errToCode(acl.ErrLimitReached, pb.RpcSpaceMakeShareableResponseError_LIMIT_REACHED),
		)
		return &pb.RpcSpaceMakeShareableResponse{
			Error: &pb.RpcSpaceMakeShareableResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	err = mw.doBlockService(func(bs *block.Service) (err error) {
		err = bs.SpaceInitChat(cctx, req.SpaceId)
		return err
	})

	if err != nil {
		return &pb.RpcSpaceMakeShareableResponse{
			Error: &pb.RpcSpaceMakeShareableResponseError{
				Code:        pb.RpcSpaceMakeShareableResponseError_UNKNOWN_ERROR,
				Description: getErrorDescription(err),
			},
		}
	}

	return &pb.RpcSpaceMakeShareableResponse{&pb.RpcSpaceMakeShareableResponseError{}}
}

func (mw *Middleware) SpaceInviteGenerate(cctx context.Context, req *pb.RpcSpaceInviteGenerateRequest) *pb.RpcSpaceInviteGenerateResponse {
	aclService := mustService[acl.AclService](mw)
	inviteInfo, err := aclService.GenerateInvite(cctx, req.SpaceId, req.InviteType, req.Permissions)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteGenerateResponseError_SPACE_IS_DELETED),
			errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteGenerateResponseError_NO_SUCH_SPACE),
			errToCode(acl.ErrPersonalSpace, pb.RpcSpaceInviteGenerateResponseError_BAD_INPUT),
			errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceInviteGenerateResponseError_REQUEST_FAILED),
			errToCode(acl.ErrLimitReached, pb.RpcSpaceInviteGenerateResponseError_LIMIT_REACHED),
			errToCode(acl.ErrNotShareable, pb.RpcSpaceInviteGenerateResponseError_NOT_SHAREABLE),
		)
		return &pb.RpcSpaceInviteGenerateResponse{
			Error: &pb.RpcSpaceInviteGenerateResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteGenerateResponse{
		InviteCid:     inviteInfo.InviteFileCid,
		InviteFileKey: inviteInfo.InviteFileKey,
		// nolint: gosec
		InviteType:  model.InviteType(inviteInfo.InviteType),
		Permissions: domain.ConvertAclPermissions(inviteInfo.Permissions),
	}
}

func (mw *Middleware) SpaceInviteChange(cctx context.Context, req *pb.RpcSpaceInviteChangeRequest) *pb.RpcSpaceInviteChangeResponse {
	aclService := mustService[acl.AclService](mw)
	err := aclService.ChangeInvite(cctx, req.SpaceId, req.Permissions)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteChangeResponseError_SPACE_IS_DELETED),
			errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteChangeResponseError_NO_SUCH_SPACE),
			errToCode(acl.ErrPersonalSpace, pb.RpcSpaceInviteChangeResponseError_BAD_INPUT),
			errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceInviteChangeResponseError_REQUEST_FAILED),
		)
		return &pb.RpcSpaceInviteChangeResponse{
			Error: &pb.RpcSpaceInviteChangeResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteChangeResponse{}
}

func (mw *Middleware) SpaceInviteGetCurrent(cctx context.Context, req *pb.RpcSpaceInviteGetCurrentRequest) *pb.RpcSpaceInviteGetCurrentResponse {
	aclService := mustService[acl.AclService](mw)
	inviteInfo, err := aclService.GetCurrentInvite(cctx, req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(inviteservice.ErrInviteNotExists, pb.RpcSpaceInviteGetCurrentResponseError_NO_ACTIVE_INVITE),
			errToCode(inviteservice.ErrInviteGet, pb.RpcSpaceInviteGetCurrentResponseError_NO_ACTIVE_INVITE),
		)
		return &pb.RpcSpaceInviteGetCurrentResponse{
			Error: &pb.RpcSpaceInviteGetCurrentResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteGetCurrentResponse{
		InviteCid:     inviteInfo.InviteFileCid,
		InviteFileKey: inviteInfo.InviteFileKey,
		// nolint: gosec
		InviteType:  model.InviteType(inviteInfo.InviteType),
		Permissions: domain.ConvertAclPermissions(inviteInfo.Permissions),
	}
}

func (mw *Middleware) SpaceInviteGetGuest(cctx context.Context, req *pb.RpcSpaceInviteGetGuestRequest) *pb.RpcSpaceInviteGetGuestResponse {
	aclService := mustService[acl.AclService](mw)
	inviteInfo, err := aclService.GetGuestUserInvite(cctx, req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(inviteservice.ErrInvalidSpaceType, pb.RpcSpaceInviteGetGuestResponseError_INVALID_SPACE_TYPE),
		)
		return &pb.RpcSpaceInviteGetGuestResponse{
			Error: &pb.RpcSpaceInviteGetGuestResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteGetGuestResponse{
		InviteCid:     inviteInfo.InviteFileCid,
		InviteFileKey: inviteInfo.InviteFileKey,
	}
}

func (mw *Middleware) SpaceInviteRevoke(cctx context.Context, req *pb.RpcSpaceInviteRevokeRequest) *pb.RpcSpaceInviteRevokeResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := aclService.RevokeInvite(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteRevokeResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteRevokeResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceInviteRevokeResponseError_REQUEST_FAILED),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceInviteRevokeResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceInviteRevokeResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceInviteRevokeResponse{
		Error: &pb.RpcSpaceInviteRevokeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceInviteView(cctx context.Context, req *pb.RpcSpaceInviteViewRequest) *pb.RpcSpaceInviteViewResponse {
	aclService := mustService[acl.AclService](mw)
	inviteView, err := viewInvite(cctx, aclService, req)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(inviteservice.ErrInviteNotExists, pb.RpcSpaceInviteViewResponseError_INVITE_NOT_FOUND),
			errToCode(inviteservice.ErrInviteGet, pb.RpcSpaceInviteViewResponseError_INVITE_NOT_FOUND),
			errToCode(inviteservice.ErrInviteBadContent, pb.RpcSpaceInviteViewResponseError_INVITE_BAD_CONTENT),
			errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteViewResponseError_SPACE_IS_DELETED),
		)
		return &pb.RpcSpaceInviteViewResponse{
			Error: &pb.RpcSpaceInviteViewResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteViewResponse{
		CreatorName:       inviteView.CreatorName,
		SpaceId:           inviteView.SpaceId,
		SpaceName:         inviteView.SpaceName,
		SpaceIconCid:      inviteView.SpaceIconCid,
		IsGuestUserInvite: inviteView.IsGuestUserInvite(),
		// nolint: gosec
		InviteType: model.InviteType(inviteView.InviteType),
	}
}

func viewInvite(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceInviteViewRequest) (domain.InviteView, error) {
	inviteFileKey, err := encode.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return domain.InviteView{}, fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return domain.InviteView{}, fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	return aclService.ViewInvite(ctx, inviteCid, inviteFileKey)
}

func (mw *Middleware) SpaceJoin(cctx context.Context, req *pb.RpcSpaceJoinRequest) *pb.RpcSpaceJoinResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := join(cctx, aclService, req)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceJoinResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceJoinResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceJoinResponseError_REQUEST_FAILED),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceJoinResponseError_LIMIT_REACHED),
		errToCode(inviteservice.ErrInviteNotExists, pb.RpcSpaceJoinResponseError_INVITE_NOT_FOUND),
		errToCode(inviteservice.ErrInviteGet, pb.RpcSpaceJoinResponseError_INVITE_NOT_FOUND),
		errToCode(inviteservice.ErrInviteBadContent, pb.RpcSpaceJoinResponseError_INVITE_BAD_CONTENT),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceJoinResponseError_NOT_SHAREABLE),
		errToCode(acl.ErrDifferentNetwork, pb.RpcSpaceJoinResponseError_DIFFERENT_NETWORK),
	)
	return &pb.RpcSpaceJoinResponse{
		Error: &pb.RpcSpaceJoinResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceStopSharing(cctx context.Context, req *pb.RpcSpaceStopSharingRequest) *pb.RpcSpaceStopSharingResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := aclService.StopSharing(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceStopSharingResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceStopSharingResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceStopSharingResponseError_REQUEST_FAILED),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceStopSharingResponseError_LIMIT_REACHED),
	)
	return &pb.RpcSpaceStopSharingResponse{
		Error: &pb.RpcSpaceStopSharingResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceJoinCancel(cctx context.Context, req *pb.RpcSpaceJoinCancelRequest) *pb.RpcSpaceJoinCancelResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := aclService.CancelJoin(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceJoinCancelResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceJoinCancelResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceJoinCancelResponseError_REQUEST_FAILED),
		errToCode(acl.ErrRequestNotExists, pb.RpcSpaceJoinCancelResponseError_NO_SUCH_REQUEST),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceJoinCancelResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceJoinCancelResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceJoinCancelResponse{
		Error: &pb.RpcSpaceJoinCancelResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestApprove(cctx context.Context, req *pb.RpcSpaceRequestApproveRequest) *pb.RpcSpaceRequestApproveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := accept(cctx, req.SpaceId, req.Identity, req.Permissions, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestApproveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrRequestNotExists, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_REQUEST),
		errToCode(acl.ErrIncorrectPermissions, pb.RpcSpaceRequestApproveResponseError_INCORRECT_PERMISSIONS),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceRequestApproveResponseError_REQUEST_FAILED),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceRequestApproveResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceRequestApproveResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceRequestApproveResponse{
		Error: &pb.RpcSpaceRequestApproveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestDecline(cctx context.Context, req *pb.RpcSpaceRequestDeclineRequest) *pb.RpcSpaceRequestDeclineResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := decline(cctx, req.SpaceId, req.Identity, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestDeclineResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestDeclineResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceRequestDeclineResponseError_REQUEST_FAILED),
		errToCode(acl.ErrRequestNotExists, pb.RpcSpaceRequestDeclineResponseError_NO_SUCH_REQUEST),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceRequestDeclineResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceRequestDeclineResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceRequestDeclineResponse{
		Error: &pb.RpcSpaceRequestDeclineResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceParticipantRemove(cctx context.Context, req *pb.RpcSpaceParticipantRemoveRequest) *pb.RpcSpaceParticipantRemoveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := remove(cctx, req.SpaceId, req.Identities, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceParticipantRemoveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceParticipantRemoveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceParticipantRemoveResponseError_REQUEST_FAILED),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceParticipantRemoveResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNoSuchAccount, pb.RpcSpaceParticipantRemoveResponseError_PARTICIPANT_NOT_FOUND),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceParticipantRemoveResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceParticipantRemoveResponse{
		Error: &pb.RpcSpaceParticipantRemoveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceParticipantPermissionsChange(cctx context.Context, req *pb.RpcSpaceParticipantPermissionsChangeRequest) *pb.RpcSpaceParticipantPermissionsChangeResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := permissionsChange(cctx, req.SpaceId, req.Changes, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceParticipantPermissionsChangeResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceParticipantPermissionsChangeResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceParticipantPermissionsChangeResponseError_REQUEST_FAILED),
		errToCode(acl.ErrNoSuchAccount, pb.RpcSpaceParticipantPermissionsChangeResponseError_PARTICIPANT_NOT_FOUND),
		errToCode(acl.ErrIncorrectPermissions, pb.RpcSpaceParticipantPermissionsChangeResponseError_INCORRECT_PERMISSIONS),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceParticipantPermissionsChangeResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceParticipantPermissionsChangeResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceParticipantPermissionsChangeResponse{
		Error: &pb.RpcSpaceParticipantPermissionsChangeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceLeaveApprove(cctx context.Context, req *pb.RpcSpaceLeaveApproveRequest) *pb.RpcSpaceLeaveApproveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := approveLeave(cctx, req.SpaceId, req.Identities, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceLeaveApproveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceLeaveApproveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceLeaveApproveResponseError_REQUEST_FAILED),
		errToCode(acl.ErrRequestNotExists, pb.RpcSpaceLeaveApproveResponseError_NO_APPROVE_REQUESTS),
		errToCode(acl.ErrLimitReached, pb.RpcSpaceLeaveApproveResponseError_LIMIT_REACHED),
		errToCode(acl.ErrNotShareable, pb.RpcSpaceLeaveApproveResponseError_NOT_SHAREABLE),
	)
	return &pb.RpcSpaceLeaveApproveResponse{
		Error: &pb.RpcSpaceLeaveApproveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceSetOrder(_ context.Context, request *pb.RpcSpaceSetOrderRequest) *pb.RpcSpaceSetOrderResponse {
	response := func(code pb.RpcSpaceSetOrderResponseErrorCode, err error, finalOrder []string) *pb.RpcSpaceSetOrderResponse {
		m := &pb.RpcSpaceSetOrderResponse{Error: &pb.RpcSpaceSetOrderResponseError{Code: code}, SpaceViewOrder: finalOrder}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	orderService := app.MustComponent[order.OrderSetter](mw.applicationService.GetApp())
	finalOrder, err := orderService.SetSpaceViewOrder(request.GetSpaceViewOrder())
	if err != nil {
		return response(pb.RpcSpaceSetOrderResponseError_UNKNOWN_ERROR, err, nil)
	}
	return response(pb.RpcSpaceSetOrderResponseError_NULL, nil, finalOrder)
}

func (mw *Middleware) SpaceUnsetOrder(_ context.Context, request *pb.RpcSpaceUnsetOrderRequest) *pb.RpcSpaceUnsetOrderResponse {
	response := func(code pb.RpcSpaceUnsetOrderResponseErrorCode, err error) *pb.RpcSpaceUnsetOrderResponse {
		m := &pb.RpcSpaceUnsetOrderResponse{Error: &pb.RpcSpaceUnsetOrderResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	orderService := app.MustComponent[order.OrderSetter](mw.applicationService.GetApp())
	err := orderService.UnsetOrder(request.SpaceViewId)
	if err != nil {
		return response(pb.RpcSpaceUnsetOrderResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcSpaceUnsetOrderResponseError_NULL, nil)
}

func join(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceJoinRequest) (err error) {
	inviteFileKey, err := encode.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return fmt.Errorf("decode key: %w, %w", err, inviteservice.ErrInviteBadContent)
	}
	return aclService.Join(ctx, req.SpaceId, req.NetworkId, inviteCid, inviteFileKey)
}

func accept(ctx context.Context, spaceId, identity string, permissions model.ParticipantPermissions, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeAccountAddress(identity)
	if err != nil {
		return
	}
	return aclService.Accept(ctx, spaceId, key, permissions)
}

func decline(ctx context.Context, spaceId, identity string, aclService acl.AclService) (err error) {
	key, err := crypto.DecodeAccountAddress(identity)
	if err != nil {
		return
	}
	return aclService.Decline(ctx, spaceId, key)
}

func remove(ctx context.Context, spaceId string, identities []string, aclService acl.AclService) error {
	keys := make([]crypto.PubKey, 0, len(identities))
	for _, identity := range identities {
		key, err := crypto.DecodeAccountAddress(identity)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	return aclService.Remove(ctx, spaceId, keys)
}

func approveLeave(ctx context.Context, spaceId string, identities []string, aclService acl.AclService) error {
	keys := make([]crypto.PubKey, 0, len(identities))
	for _, identity := range identities {
		key, err := crypto.DecodeAccountAddress(identity)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	return aclService.ApproveLeave(ctx, spaceId, keys)
}

func permissionsChange(ctx context.Context, spaceId string, changes []*model.ParticipantPermissionChange, aclService acl.AclService) error {
	accPermissions := make([]acl.AccountPermissions, 0, len(changes))
	for _, change := range changes {
		key, err := crypto.DecodeAccountAddress(change.Identity)
		if err != nil {
			return err
		}
		accPermissions = append(accPermissions, acl.AccountPermissions{
			Account:     key,
			Permissions: change.Perms,
		})
	}
	return aclService.ChangePermissions(ctx, spaceId, accPermissions)
}
