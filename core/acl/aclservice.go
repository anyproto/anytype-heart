package acl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/identityrepo/identityrepoproto"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "common.acl.aclservice"

var log = logging.Logger(CName).Desugar()

var sleepTime = time.Millisecond * 500

type NodeConfGetter interface {
	GetNodeConf() (conf nodeconf.Configuration)
}

type AccountPermissions struct {
	Account     crypto.PubKey
	Permissions model.ParticipantPermissions
}

type AclService interface {
	app.Component
	GenerateInvite(ctx context.Context, spaceId string) (domain.InviteInfo, error)
	RevokeInvite(ctx context.Context, spaceId string) error
	GetCurrentInvite(ctx context.Context, spaceId string) (domain.InviteInfo, error)
	GetGuestUserInvite(ctx context.Context, spaceId string) (domain.InviteInfo, error)
	ViewInvite(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (domain.InviteView, error)
	Join(ctx context.Context, spaceId, networkId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error
	ApproveLeave(ctx context.Context, spaceId string, identities []crypto.PubKey) error
	MakeShareable(ctx context.Context, spaceId string) error
	StopSharing(ctx context.Context, spaceId string) error
	CancelJoin(ctx context.Context, spaceId string) (err error)
	Accept(ctx context.Context, spaceId string, identity crypto.PubKey, permissions model.ParticipantPermissions) error
	Decline(ctx context.Context, spaceId string, identity crypto.PubKey) (err error)
	Leave(ctx context.Context, spaceId string) (err error)
	Remove(ctx context.Context, spaceId string, identities []crypto.PubKey) (err error)
	ChangePermissions(ctx context.Context, spaceId string, perms []AccountPermissions) (err error)
	AddAccount(ctx context.Context, spaceId string, pubKey crypto.PubKey, metadata []byte, permissions list.AclPermissions) error
	AddGuestAccount(ctx context.Context, spaceId string) (privKey crypto.PrivKey, err error)
}

func New() AclService {
	return &aclService{}
}

type identityRepoClient interface {
	app.Component
	IdentityRepoPut(ctx context.Context, identity string, data []*identityrepoproto.Data) (err error)
	IdentityRepoGet(ctx context.Context, identities []string, kinds []string) (res []*identityrepoproto.DataWithIdentity, err error)
}

type aclService struct {
	nodeConfigGetter NodeConfGetter
	joiningClient    aclclient.AclJoiningClient
	spaceService     space.Service
	inviteService    inviteservice.InviteService
	accountService   account.Service
	coordClient      coordinatorclient.CoordinatorClient
	identityRepo     identityRepoClient
}

func (a *aclService) Init(ap *app.App) (err error) {
	a.nodeConfigGetter = app.MustComponent[NodeConfGetter](ap)
	a.joiningClient = app.MustComponent[aclclient.AclJoiningClient](ap)
	a.spaceService = app.MustComponent[space.Service](ap)
	a.accountService = app.MustComponent[account.Service](ap)
	a.inviteService = app.MustComponent[inviteservice.InviteService](ap)
	a.coordClient = app.MustComponent[coordinatorclient.CoordinatorClient](ap)
	a.identityRepo = app.MustComponent[identityRepoClient](ap)
	return nil
}

func (a *aclService) Name() (name string) {
	return CName
}

func (a *aclService) MakeShareable(ctx context.Context, spaceId string) error {
	err := a.coordClient.SpaceMakeShareable(ctx, spaceId)
	if err != nil {
		return convertedOrInternalError("make shareable", err)
	}
	info := spaceinfo.NewSpaceLocalInfo(spaceId)
	info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
	err = a.spaceService.TechSpace().SetLocalInfo(ctx, info)
	if err != nil {
		return convertedOrInternalError("set local info", err)
	}
	return nil
}

func (a *aclService) pushGuest(ctx context.Context, privKey crypto.PrivKey) (metadata []byte, err error) {
	metadataModel, _, err := space.DeriveAccountMetadata(privKey)
	if err != nil {
		return nil, fmt.Errorf("derive account metadata: %w", err)
	}
	metadata, err = metadataModel.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return
}

func (a *aclService) AddGuestAccount(ctx context.Context, spaceId string) (privKey crypto.PrivKey, err error) {
	pk, pubKey, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return nil, err
	}
	metadata, err := a.pushGuest(ctx, pk)
	if err != nil {
		return nil, err
	}
	return pk, a.AddAccount(ctx, spaceId, pubKey, metadata, list.AclPermissionsGuest)
}

func (a *aclService) AddAccount(ctx context.Context, spaceId string, pubKey crypto.PubKey, metadata []byte, permission list.AclPermissions) error {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	err = sp.CommonSpace().AclClient().AddAccounts(ctx, list.AccountsAddPayload{Additions: []list.AccountAdd{
		{
			Identity:    pubKey,
			Metadata:    metadata,
			Permissions: permission,
		},
	}})
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	return nil
}

func (a *aclService) Remove(ctx context.Context, spaceId string, identities []crypto.PubKey) error {
	removeSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	newPrivKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return convertedOrInternalError("generate random key pair", err)
	}
	cl := removeSpace.CommonSpace().AclClient()
	err = cl.RemoveAccounts(ctx, list.AccountRemovePayload{
		Identities: identities,
		Change: list.ReadKeyChangePayload{
			MetadataKey: newPrivKey,
			ReadKey:     crypto.NewAES(),
		},
	})
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	return nil
}

func (a *aclService) CancelJoin(ctx context.Context, spaceId string) (err error) {
	err = a.joiningClient.CancelJoin(ctx, spaceId)
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	err = a.spaceService.Delete(ctx, spaceId)
	if err != nil {
		return convertedOrInternalError("delete space", err)
	}
	return nil
}

func (a *aclService) Decline(ctx context.Context, spaceId string, identity crypto.PubKey) (err error) {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	cl := sp.CommonSpace().AclClient()
	err = cl.DeclineRequest(ctx, identity)
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	return nil
}

func (a *aclService) RevokeInvite(ctx context.Context, spaceId string) error {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	cl := sp.CommonSpace().AclClient()
	err = cl.RevokeAllInvites(ctx)
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	err = a.inviteService.RemoveExisting(ctx, spaceId)
	if err != nil {
		return convertedOrInternalError("remove existing invite", err)
	}
	return nil
}

func (a *aclService) ChangePermissions(ctx context.Context, spaceId string, perms []AccountPermissions) error {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	var listPerms []list.PermissionChangePayload
	acl := sp.CommonSpace().Acl()
	acl.RLock()
	for _, perm := range perms {
		var aclPerms list.AclPermissions
		switch perm.Permissions {
		case model.ParticipantPermissions_Reader:
			aclPerms = list.AclPermissionsReader
		case model.ParticipantPermissions_Writer:
			aclPerms = list.AclPermissionsWriter
		default:
			acl.RUnlock()
			return ErrIncorrectPermissions
		}
		curPerms := acl.AclState().Permissions(perm.Account)
		if curPerms.NoPermissions() {
			acl.RUnlock()
			return ErrNoSuchAccount
		}
		if curPerms == aclPerms {
			continue
		}
		listPerms = append(listPerms, list.PermissionChangePayload{
			Identity:    perm.Account,
			Permissions: aclPerms,
		})
	}
	acl.RUnlock()
	cl := sp.CommonSpace().AclClient()
	err = cl.ChangePermissions(ctx, list.PermissionChangesPayload{
		Changes: listPerms,
	})
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	return nil
}

func (a *aclService) ApproveLeave(ctx context.Context, spaceId string, identities []crypto.PubKey) error {
	sp, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	acl := sp.CommonSpace().Acl()
	acl.RLock()
	st := acl.AclState()
	identitiesMap := map[string]struct{}{}
	for _, identity := range identities {
		identitiesMap[identity.Account()] = struct{}{}
	}
	for _, rec := range st.RemoveRecords() {
		for _, identity := range identities {
			if rec.RequestIdentity.Equals(identity) {
				delete(identitiesMap, identity.Account())
			}
		}
	}
	if len(identitiesMap) != 0 {
		acl.RUnlock()
		identities := make([]string, 0, len(identitiesMap))
		for identity := range identitiesMap {
			identities = append(identities, identity)
		}
		return fmt.Errorf("%w with identities: %s", ErrRequestNotExists, strings.Join(identities, ", "))
	}
	acl.RUnlock()
	return a.Remove(ctx, spaceId, identities)
}

func (a *aclService) Leave(ctx context.Context, spaceId string) error {
	removeSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		// space storage missing can occur only in case of missing space
		if errors.Is(err, space.ErrSpaceStorageMissig) || errors.Is(err, space.ErrSpaceDeleted) {
			return nil
		}
		return convertedOrSpaceErr(err)
	}
	identity := a.accountService.Keys().SignKey.GetPublic()
	if !removeSpace.GetAclIdentity().Equals(identity) {
		// this is a streamable space
		// we exist there under ephemeral guest identity and should not remove it
		return nil
	}

	cl := removeSpace.CommonSpace().AclClient()
	err = cl.RequestSelfRemove(ctx)
	if err != nil {
		errs := []error{
			list.ErrPendingRequest,
			list.ErrIsOwner,
			list.ErrNoSuchAccount,
			coordinatorproto.ErrSpaceIsDeleted,
			coordinatorproto.ErrSpaceNotExists,
		}
		for _, e := range errs {
			if errors.Is(err, e) {
				return nil
			}
		}
		return convertedOrAclRequestError(err)
	}
	return nil
}

func (a *aclService) StopSharing(ctx context.Context, spaceId string) error {
	removeSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	var (
		commonSpace = removeSpace.CommonSpace()
		acl         = commonSpace.Acl()
		techSpace   = a.spaceService.TechSpace()
		localInfo   spaceinfo.SpaceLocalInfo
	)
	err = techSpace.DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		localInfo = spaceView.GetLocalInfo()
		return nil
	})
	if err != nil {
		return convertedOrInternalError("get local info", err)
	}
	newPrivKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	if err != nil {
		return convertedOrInternalError("generate random key pair", err)
	}
	cl := commonSpace.AclClient()
	err = cl.StopSharing(ctx, list.ReadKeyChangePayload{
		MetadataKey: newPrivKey,
		ReadKey:     crypto.NewAES(),
	})
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	acl.RLock()
	head := acl.Head().Id
	acl.RUnlock()
	err = a.inviteService.RemoveExisting(ctx, spaceId)
	if err != nil {
		return convertedOrInternalError("remove existing invite", err)
	}
	if localInfo.GetShareableStatus() != spaceinfo.ShareableStatusShareable {
		return nil
	}
	for {
		err = a.coordClient.SpaceMakeUnshareable(ctx, spaceId, head)
		if errors.Is(err, coordinatorproto.ErrAclHeadIsMissing) {
			time.Sleep(sleepTime)
			continue
		}
		break
	}
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	info := spaceinfo.NewSpaceLocalInfo(spaceId)
	info.SetShareableStatus(spaceinfo.ShareableStatusNotShareable)
	err = techSpace.SetLocalInfo(ctx, info)
	if err != nil {
		return convertedOrInternalError("set local info", err)
	}
	return nil
}

func (a *aclService) Join(ctx context.Context, spaceId, networkId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error {
	if a.nodeConfigGetter.GetNodeConf().NetworkId != networkId {
		return fmt.Errorf("%w. Local network: '%s', network of space to join: '%s'", ErrDifferentNetwork, a.nodeConfigGetter.GetNodeConf().NetworkId, networkId)
	}
	invitePayload, err := a.inviteService.GetPayload(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return convertedOrInternalError("get invite payload", err)
	}
	if invitePayload.InviteType == model.InvitePayload_JoinAsGuest {
		guestKey, err := crypto.UnmarshalEd25519PrivateKeyProto(invitePayload.GuestKey)
		if err != nil {
			return convertedOrInternalError("unmarshal invite key", err)
		}
		return a.joinAsGuest(ctx, invitePayload.SpaceId, guestKey)
	}
	inviteKey, err := crypto.UnmarshalEd25519PrivateKeyProto(invitePayload.AclKey)
	if err != nil {
		return convertedOrInternalError("unmarshal invite key", err)
	}
	aclHeadId, err := a.joiningClient.RequestJoin(ctx, spaceId, list.RequestJoinPayload{
		InviteKey: inviteKey,
		Metadata:  a.spaceService.AccountMetadataPayload(),
	})
	if err != nil {
		if errors.Is(err, coordinatorproto.ErrSpaceIsDeleted) {
			return space.ErrSpaceDeleted
		}
		if errors.Is(err, list.ErrInsufficientPermissions) {
			err = a.joiningClient.CancelRemoveSelf(ctx, spaceId)
			if err != nil {
				return convertedOrAclRequestError(err)
			}
			err = a.spaceService.CancelLeave(ctx, spaceId)
			if err != nil {
				return convertedOrInternalError("cancel leave", err)
			}
		}
		return convertedOrAclRequestError(err)
	}
	err = a.spaceService.Join(ctx, spaceId, aclHeadId)
	if err != nil {
		return convertedOrInternalError("join space", err)
	}
	err = a.spaceService.TechSpace().SpaceViewSetData(ctx, spaceId,
		domain.NewDetails().
			SetString(bundle.RelationKeyName, invitePayload.SpaceName).
			SetString(bundle.RelationKeyIconImage, invitePayload.SpaceIconCid))
	if err != nil {
		return convertedOrInternalError("set space data", err)
	}
	return nil
}

func (a *aclService) ViewInvite(ctx context.Context, inviteCid cid.Cid, inviteFileKey crypto.SymKey) (view domain.InviteView, err error) {
	res, err := a.inviteService.View(ctx, inviteCid, inviteFileKey)
	if err != nil {
		return domain.InviteView{}, convertedOrInternalError("view invite", err)
	}
	if res.IsGuestUserInvite() {
		return domain.InviteView{
			SpaceId:      res.SpaceId,
			GuestKey:     res.GuestKey,
			SpaceName:    res.SpaceName,
			SpaceIconCid: res.SpaceIconCid,
			CreatorName:  res.CreatorName,
		}, nil
	}
	inviteKey, err := crypto.UnmarshalEd25519PrivateKeyProto(res.AclKey)
	if err != nil {
		return domain.InviteView{}, convertedOrInternalError("unmarshal invite key", err)
	}
	recs, err := a.joiningClient.AclGetRecords(ctx, res.SpaceId, "")
	if err != nil {
		return domain.InviteView{}, convertedOrAclRequestError(err)
	}
	if len(recs) == 0 {
		return domain.InviteView{}, fmt.Errorf("no acl records found for space: %s, %w", res.SpaceId, ErrAclRequestFailed)
	}
	store, err := list.NewInMemoryStorage(recs[0].Id, recs)
	if err != nil {
		return domain.InviteView{}, convertedOrAclRequestError(err)
	}
	lst, err := list.BuildAclListWithIdentity(a.accountService.Keys(), store, list.NoOpAcceptorVerifier{})
	if err != nil {
		return domain.InviteView{}, convertedOrAclRequestError(err)
	}
	for _, inv := range lst.AclState().Invites() {
		if inviteKey.GetPublic().Equals(inv) {
			return res, nil
		}
	}
	return domain.InviteView{}, inviteservice.ErrInviteNotExists
}

func (a *aclService) Accept(ctx context.Context, spaceId string, identity crypto.PubKey, permissions model.ParticipantPermissions) error {
	validPerms := permissions == model.ParticipantPermissions_Reader || permissions == model.ParticipantPermissions_Writer
	if !validPerms {
		return ErrIncorrectPermissions
	}
	acceptSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return convertedOrSpaceErr(err)
	}
	acl := acceptSpace.CommonSpace().Acl()
	acl.RLock()
	recs, err := acl.AclState().JoinRecords(false)
	if err != nil {
		acl.RUnlock()
		return convertedOrInternalError("join records get error", err)
	}
	var recId string
	for _, rec := range recs {
		if rec.RequestIdentity.Equals(identity) {
			recId = rec.RecordId
			break
		}
	}
	acl.RUnlock()
	if recId == "" {
		return fmt.Errorf("%w with identity: %s", ErrRequestNotExists, identity.Account())
	}
	cl := acceptSpace.CommonSpace().AclClient()
	var aclPerms list.AclPermissions
	switch permissions {
	case model.ParticipantPermissions_Reader:
		aclPerms = list.AclPermissionsReader
	case model.ParticipantPermissions_Writer:
		aclPerms = list.AclPermissionsWriter
	}
	err = cl.AcceptRequest(ctx, list.RequestAcceptPayload{
		RequestRecordId: recId,
		Permissions:     aclPerms,
	})
	if err != nil {
		return convertedOrAclRequestError(err)
	}
	return nil
}

func (a *aclService) GetCurrentInvite(ctx context.Context, spaceId string) (domain.InviteInfo, error) {
	return a.inviteService.GetCurrent(ctx, spaceId)
}

func (a *aclService) GenerateInvite(ctx context.Context, spaceId string) (result domain.InviteInfo, err error) {
	if spaceId == a.accountService.PersonalSpaceID() {
		err = ErrPersonalSpace
		return
	}
	current, err := a.inviteService.GetCurrent(ctx, spaceId)
	if err == nil {
		return current, nil
	}
	acceptSpace, err := a.spaceService.Get(ctx, spaceId)
	if err != nil {
		return
	}
	aclClient := acceptSpace.CommonSpace().AclClient()
	res, err := aclClient.GenerateInvite()
	if err != nil {
		err = convertedOrInternalError("couldn't generate acl invite", err)
		return
	}
	return a.inviteService.Generate(ctx, spaceId, res.InviteKey, func() error {
		err := aclClient.AddRecord(ctx, res.InviteRec)
		if err != nil {
			return convertedOrAclRequestError(err)
		}
		return nil
	})
}

func (a *aclService) GetGuestUserInvite(ctx context.Context, spaceId string) (info domain.InviteInfo, err error) {
	if spaceId == a.accountService.PersonalSpaceID() {
		err = ErrPersonalSpace
		return
	}
	current, err := a.inviteService.GetExistingGuestUserInvite(ctx, spaceId)
	if err == nil {
		return current, nil
	}
	var shareableStatus spaceinfo.ShareableStatus
	err = a.spaceService.TechSpace().DoSpaceView(ctx, spaceId, func(spaceView techspace.SpaceView) error {
		localInfo := spaceView.GetLocalInfo()
		shareableStatus = localInfo.GetShareableStatus()
		return nil
	})
	if err != nil {
		return
	}

	if shareableStatus != spaceinfo.ShareableStatusShareable {
		err = a.MakeShareable(ctx, spaceId)
		if err != nil {
			return
		}
	}
	// todo: race conds in case guest user already created?
	// we can iterate users to find the guest key
	guestKey, err := a.AddGuestAccount(ctx, spaceId)
	if err != nil {
		return domain.InviteInfo{}, convertedOrInternalError("add guest account", err)
	}
	info, err = a.inviteService.GenerateGuestUserInvite(ctx, spaceId, guestKey)
	if err != nil {
		return domain.InviteInfo{}, convertedOrInternalError("generate guest user invite", err)
	}
	return
}

func (a *aclService) joinAsGuest(ctx context.Context, spaceId string, guestUserKey crypto.PrivKey) (err error) {
	return a.spaceService.AddStreamable(ctx, spaceId, guestUserKey)
}
