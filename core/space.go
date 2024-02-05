package core

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/acl"
	"github.com/anyproto/anytype-heart/core/identity"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (mw *Middleware) SpaceDelete(cctx context.Context, req *pb.RpcSpaceDeleteRequest) *pb.RpcSpaceDeleteResponse {
	spaceService := mw.applicationService.GetApp().MustComponent(space.CName).(space.Service)
	err := spaceService.Delete(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceDeleteResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceDeleteResponseError_NO_SUCH_SPACE),
	)
	return &pb.RpcSpaceDeleteResponse{
		Error: &pb.RpcSpaceDeleteResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceInviteGenerate(cctx context.Context, req *pb.RpcSpaceInviteGenerateRequest) *pb.RpcSpaceInviteGenerateResponse {
	aclService := getService[acl.AclService](mw)
	inviteInfo, err := aclService.GenerateInvite(cctx, req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(space.ErrSpaceDeleted, pb.RpcSpaceInviteGenerateResponseError_SPACE_IS_DELETED),
			errToCode(space.ErrSpaceNotExists, pb.RpcSpaceInviteGenerateResponseError_NO_SUCH_SPACE),
			errToCode(acl.ErrPersonalSpace, pb.RpcSpaceInviteGenerateResponseError_BAD_INPUT),
			errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceInviteGenerateResponseError_REQUEST_FAILED),
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
	}
}

func (mw *Middleware) SpaceInviteGetCurrent(cctx context.Context, req *pb.RpcSpaceInviteGetCurrentRequest) *pb.RpcSpaceInviteGetCurrentResponse {
	aclService := getService[acl.AclService](mw)
	inviteInfo, err := aclService.GetCurrentInvite(req.SpaceId)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(acl.ErrInviteNotExist, pb.RpcSpaceInviteGetCurrentResponseError_NO_ACTIVE_INVITE),
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
	}
}

func (mw *Middleware) SpaceInviteRevoke(cctx context.Context, req *pb.RpcSpaceInviteRevokeRequest) *pb.RpcSpaceInviteRevokeResponse {
	return &pb.RpcSpaceInviteRevokeResponse{
		Error: &pb.RpcSpaceInviteRevokeResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
}

func (mw *Middleware) SpaceInviteView(cctx context.Context, req *pb.RpcSpaceInviteViewRequest) *pb.RpcSpaceInviteViewResponse {
	aclService := getService[acl.AclService](mw)
	inviteView, err := viewInvite(cctx, aclService, req)
	if err != nil {
		code := mapErrorCode(err,
			errToCode(acl.ErrInviteBadSignature, pb.RpcSpaceInviteViewResponseError_INVITE_BAD_SIGNATURE),
		)
		return &pb.RpcSpaceInviteViewResponse{
			Error: &pb.RpcSpaceInviteViewResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcSpaceInviteViewResponse{
		CreatorName:  inviteView.CreatorName,
		SpaceId:      inviteView.SpaceId,
		SpaceName:    inviteView.SpaceName,
		SpaceIconCid: inviteView.SpaceIconCid,
	}
}

func viewInvite(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceInviteViewRequest) (*acl.InviteView, error) {
	inviteFileKey, err := acl.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return nil, err
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
	)
	return &pb.RpcSpaceJoinResponse{
		Error: &pb.RpcSpaceJoinResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceJoinCancel(cctx context.Context, req *pb.RpcSpaceJoinCancelRequest) *pb.RpcSpaceJoinCancelResponse {
	return &pb.RpcSpaceJoinCancelResponse{
		Error: &pb.RpcSpaceJoinCancelResponseError{
			Code:        1,
			Description: getErrorDescription(fmt.Errorf("not implemented")),
		},
	}
}

func (mw *Middleware) SpaceExit(cctx context.Context, req *pb.RpcSpaceExitRequest) *pb.RpcSpaceExitResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := aclService.Exit(cctx, req.SpaceId)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceExitResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceExitResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceExitResponseError_REQUEST_FAILED),
	)
	return &pb.RpcSpaceExitResponse{
		Error: &pb.RpcSpaceExitResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) SpaceRequestApprove(cctx context.Context, req *pb.RpcSpaceRequestApproveRequest) *pb.RpcSpaceRequestApproveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := accept(cctx, req.SpaceId, req.Identity, req.Permissions, aclService)
	if err == nil {
		err = mw.sendResponseNotification(cctx, req.Identity, req.SpaceId, req.Permissions, true)
	}
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestApproveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrNoSuchUser, pb.RpcSpaceRequestApproveResponseError_NO_SUCH_IDENTITY),
		errToCode(acl.ErrIncorrectPermissions, pb.RpcSpaceRequestApproveResponseError_INCORRECT_PERMISSIONS),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceRequestApproveResponseError_REQUEST_FAILED),
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
	if err == nil {
		err = mw.sendResponseNotification(cctx, req.Identity, req.SpaceId, 0, false)
	}
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceRequestDeclineResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceRequestDeclineResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceRequestDeclineResponseError_REQUEST_FAILED),
	)

	return &pb.RpcSpaceRequestDeclineResponse{
		Error: &pb.RpcSpaceRequestDeclineResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) sendResponseNotification(cctx context.Context, id, spaceId string, permissions model.ParticipantPermissions, isApproved bool) error {
	identityService := app.MustComponent[identity.Service](mw.GetApp())
	details, err := identityService.GetDetails(cctx, id)
	if err != nil {
		return err
	}
	name := pbtypes.GetString(details, bundle.RelationKeyName.String())
	image := pbtypes.GetString(details, bundle.RelationKeyIconImage.String())
	err = app.MustComponent[notifications.Notifications](mw.GetApp()).CreateAndSend(&model.Notification{
		//Id:     TODO что тут за id???,
		IsLocal: false,
		Payload: &model.NotificationPayloadOfRequestResponse{RequestResponse: &model.NotificationRequestResponse{
			Identity:     id,
			IdentityName: name,
			IdentityIcon: image,
			IsApproved:   isApproved,
			Permission:   permissions,
		}},
		Space: spaceId,
	})
	return err
}

func (mw *Middleware) SpaceParticipantRemove(cctx context.Context, req *pb.RpcSpaceParticipantRemoveRequest) *pb.RpcSpaceParticipantRemoveResponse {
	aclService := mw.applicationService.GetApp().MustComponent(acl.CName).(acl.AclService)
	err := remove(cctx, req.SpaceId, req.Identities, aclService)
	code := mapErrorCode(err,
		errToCode(space.ErrSpaceDeleted, pb.RpcSpaceParticipantRemoveResponseError_SPACE_IS_DELETED),
		errToCode(space.ErrSpaceNotExists, pb.RpcSpaceParticipantRemoveResponseError_NO_SUCH_SPACE),
		errToCode(acl.ErrAclRequestFailed, pb.RpcSpaceParticipantRemoveResponseError_REQUEST_FAILED),
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
	)
	return &pb.RpcSpaceParticipantPermissionsChangeResponse{
		Error: &pb.RpcSpaceParticipantPermissionsChangeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func join(ctx context.Context, aclService acl.AclService, req *pb.RpcSpaceJoinRequest) (err error) {
	inviteFileKey, err := acl.DecodeKeyFromBase58(req.InviteFileKey)
	if err != nil {
		return
	}
	inviteCid, err := cid.Decode(req.InviteCid)
	if err != nil {
		return
	}
	return aclService.Join(ctx, req.SpaceId, inviteCid, inviteFileKey)
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
	var keys []crypto.PubKey
	for _, identity := range identities {
		key, err := crypto.DecodeAccountAddress(identity)
		if err != nil {
			return err
		}
		keys = append(keys, key)
	}
	return aclService.Remove(ctx, spaceId, keys)
}

func permissionsChange(ctx context.Context, spaceId string, changes []*model.ParticipantPermissionChange, aclService acl.AclService) error {
	var accPermissions []acl.AccountPermissions
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
