package inviteservice

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/util/cidutil"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/mock_domain"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore/mock_invitestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/encode"
)

type mockInviteObject struct {
	smartblock.SmartBlock
	*mock_domain.MockInviteObject
}

// expectInviteObject lets the service reach the workspace, which holds an invite shared within the
// space, and the marker of one the owner holds
func (fx *fixture) expectInviteObject() {
	fx.mockSpaceService.EXPECT().Get(ctx, "spaceId").Return(fx.mockSpace, nil)
	fx.mockSpace.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
		Workspace: "workspaceId",
	})
	fx.mockSpace.EXPECT().Do("workspaceId", mock.Anything).RunAndReturn(func(s string, f func(smartblock.SmartBlock) error) error {
		return f(mockInviteObject{SmartBlock: smarttest.New("root"), MockInviteObject: fx.mockInviteObject})
	})
}

// expectSpaceView lets the service reach the owner's space view, which holds an invite the owner
// keeps in their own account
func (fx *fixture) expectSpaceView() {
	fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
	fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
		func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
			return f(fx.mockSpaceView)
		})
}

func newCidFromBytes(data []byte) (cid.Cid, error) {
	hash, err := mh.Sum(data, mh.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.DagCBOR, hash), nil
}

func TestInviteService_GetCurrent(t *testing.T) {
	t.Run("invite shared within the space is read from the workspace", func(t *testing.T) {
		// given a space whose invite the members can share: it lives in the workspace, which is read first
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		want := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(want)

		// when the current invite is asked for
		got, err := fx.GetCurrent(ctx, "spaceId")

		// then it comes back whole, and the space view is never consulted
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("invite held by the owner is read from their space view", func(t *testing.T) {
		// given the owner's device: the workspace holds only the marker, so the invite is read from the
		// space view after the workspace comes back without a cid
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		fx.expectSpaceView()
		want := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
			HeldByOwner:   true,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{HeldByOwner: true})
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(want)

		// when the current invite is asked for
		got, err := fx.GetCurrent(ctx, "spaceId")

		// then it comes back whole
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("a stale space view invite is ignored when the workspace holds one too", func(t *testing.T) {
		// given the anomaly an old client can leave behind: it wrote a cid to the workspace without
		// clearing the owner's space view, so both hold a cid. The workspace one is the live invite.
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		workspaceInvite := domain.InviteInfo{
			InviteFileCid: "liveCid",
			InviteFileKey: "liveKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsReader,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(workspaceInvite)

		// when the current invite is asked for
		got, err := fx.GetCurrent(ctx, "spaceId")

		// then the workspace invite wins and the stale space view is never read
		require.NoError(t, err)
		require.Equal(t, workspaceInvite, got)
		require.False(t, got.HeldByOwner)
	})

	t.Run("member of a space gets the marker without the invite", func(t *testing.T) {
		// given a member's device: the workspace carries the marker, and nothing else
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		want := domain.InviteInfo{HeldByOwner: true}
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(want)

		// when the current invite is asked for
		got, err := fx.GetCurrent(ctx, "spaceId")

		// then the member learns an invite exists and that the owner is the one to ask, without ever
		// holding the link
		require.NoError(t, err)
		require.Equal(t, want, got)
		require.Empty(t, got.InviteFileCid)
		require.Empty(t, got.InviteFileKey)
	})

	t.Run("no invite at all", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})

		_, err := fx.GetCurrent(ctx, "spaceId")
		require.ErrorIs(t, err, ErrInviteNotExists)
	})
}

func TestInviteService_RemoveExisting(t *testing.T) {
	t.Run("invite shared within the space", func(t *testing.T) {
		// given an invite the workspace holds
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		res, err := cidutil.NewCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		want := domain.InviteInfo{
			InviteFileCid: res,
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		invCid, err := cid.Decode(want.InviteFileCid)
		require.NoError(t, err)
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return(domain.InviteInfo{}, nil)
		fx.mockInviteObject.EXPECT().RemoveExistingInviteInfo().Return(want, nil)
		fx.mockInviteStore.EXPECT().RemoveInvite(ctx, invCid).Return(nil)

		// when it is removed
		got, err := fx.RemoveExisting(ctx, "spaceId")

		// then both objects are cleared and the file is gone
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("invite held by the owner", func(t *testing.T) {
		// given an invite the owner's space view holds
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		res, err := cidutil.NewCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		want := domain.InviteInfo{
			InviteFileCid: res,
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
			HeldByOwner:   true,
		}
		invCid, err := cid.Decode(want.InviteFileCid)
		require.NoError(t, err)
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return(want, nil)
		// the workspace is cleared too: it carries the marker of this invite
		fx.mockInviteObject.EXPECT().RemoveExistingInviteInfo().Return(domain.InviteInfo{HeldByOwner: true}, nil)
		fx.mockInviteStore.EXPECT().RemoveInvite(ctx, invCid).Return(nil)

		// when it is removed
		got, err := fx.RemoveExisting(ctx, "spaceId")

		// then the invite the owner held is the one reported, and its file is gone
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("the file is deleted even when clearing the workspace fails", func(t *testing.T) {
		// given an owner-held invite whose cid lives only in the space view
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		res, err := cidutil.NewCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		held := domain.InviteInfo{
			InviteFileCid: res,
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeDefault,
			HeldByOwner:   true,
		}
		invCid, err := cid.Decode(held.InviteFileCid)
		require.NoError(t, err)
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return(held, nil)
		fx.mockInviteObject.EXPECT().RemoveExistingInviteInfo().Return(domain.InviteInfo{}, fmt.Errorf("apply failed"))
		// the file is still deleted off the cid recovered from the space view, so it is never stranded on
		// the node with no object left pointing at it
		fx.mockInviteStore.EXPECT().RemoveInvite(ctx, invCid).Return(nil)

		// when the removal runs and the workspace clear fails
		_, err = fx.RemoveExisting(ctx, "spaceId")

		// then the error is reported, but the file has already been deleted
		require.Error(t, err)
	})
}

func TestInviteService_ShareWithinSpace(t *testing.T) {
	t.Run("the invite moves from the owner's space view into the workspace", func(t *testing.T) {
		// given an invite the owner holds in their own account
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		held := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeDefault,
			HeldByOwner:   true,
		}
		want := held
		want.HeldByOwner = false
		// the workspace is read first and carries only the marker, so the invite is read from the space view
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{HeldByOwner: true})
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(held)
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return(held, nil)
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(want).Return(nil)

		// when it is shared within the space
		got, err := fx.ShareWithinSpace(ctx, "spaceId")

		// then the workspace holds the very same invite -- same cid, same key, same link
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("an invite that is already shared is left alone", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		want := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeDefault,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(want)

		got, err := fx.ShareWithinSpace(ctx, "spaceId")
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("an invite anyone can join with is published only when it grants read access", func(t *testing.T) {
		// given an anyoneCanJoin invite that lets whoever holds the link write in the space
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		fx.expectSpaceView()
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{HeldByOwner: true})
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
			HeldByOwner:   true,
		})

		// when it is asked to be shared within the space
		_, err := fx.ShareWithinSpace(ctx, "spaceId")

		// then it is refused: nobody approves a join through such a link, so handing it to every member
		// hands every member a way to let strangers write
		require.ErrorIs(t, err, ErrInviteNotShareable)
	})

	t.Run("no invite to share", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})

		_, err := fx.ShareWithinSpace(ctx, "spaceId")
		require.ErrorIs(t, err, ErrInviteNotExists)
	})
}

func TestInviteService_Generate(t *testing.T) {
	t.Run("generate invite anyone, no existing info", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		acc, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().AccountID().Return(acc.SignKey.GetPublic().Account())
		profile := account.Profile{
			Id:        "profileId",
			AccountId: acc.SignKey.GetPublic().Account(),
			Name:      "Misha",
		}
		spaceDescription := spaceinfo.SpaceDescription{
			Name:      "space",
			IconImage: "icon",
		}
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceDescription)
		fx.mockFileAcl.EXPECT().GetInfoForFileSharing(spaceDescription.IconImage).Return("iconCid", nil, nil)
		fx.expectSpaceView()
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything, mock.Anything, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsReader,
			HeldByOwner:   true,
		}
		fx.mockSpaceView.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		info, err := fx.inviteService.Generate(ctx, GenerateInviteParams{
			SpaceId:     "spaceId",
			Key:         acc.PeerKey,
			InviteType:  domain.InviteTypeAnyone,
			Permissions: list.AclPermissionsReader,
		}, func() error {
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, inviteInfo, info)
	})
	t.Run("generate invite anyone, invite exists of different type", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		returnedInfo := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeDefault,
			Permissions:   list.AclPermissionsReader,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(returnedInfo)
		acc, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().AccountID().Return(acc.SignKey.GetPublic().Account())
		profile := account.Profile{
			Id:        "profileId",
			AccountId: acc.SignKey.GetPublic().Account(),
			Name:      "Misha",
		}
		spaceDescription := spaceinfo.SpaceDescription{
			Name:      "space",
			IconImage: "icon",
		}
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceDescription)
		fx.mockFileAcl.EXPECT().GetInfoForFileSharing(spaceDescription.IconImage).Return("iconCid", nil, nil)
		fx.expectSpaceView()
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything, mock.Anything, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsReader,
			HeldByOwner:   true,
		}
		fx.mockSpaceView.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		info, err := fx.inviteService.Generate(ctx, GenerateInviteParams{
			SpaceId:     "spaceId",
			Key:         acc.PeerKey,
			InviteType:  domain.InviteTypeAnyone,
			Permissions: list.AclPermissionsReader,
		}, func() error {
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, inviteInfo, info)
	})
	t.Run("generate invite request join, no existing info", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		acc, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().AccountID().Return(acc.SignKey.GetPublic().Account())
		profile := account.Profile{
			Id:        "profileId",
			AccountId: acc.SignKey.GetPublic().Account(),
			Name:      "Misha",
		}
		spaceDescription := spaceinfo.SpaceDescription{
			Name:      "space",
			IconImage: "icon",
		}
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceDescription)
		fx.mockFileAcl.EXPECT().GetInfoForFileSharing(spaceDescription.IconImage).Return("iconCid", nil, nil)
		fx.expectSpaceView()
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything, mock.Anything, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeDefault,
			Permissions:   list.AclPermissionsReader,
			HeldByOwner:   true,
		}
		fx.mockSpaceView.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		info, err := fx.inviteService.Generate(ctx, GenerateInviteParams{
			SpaceId:     "spaceId",
			Key:         acc.PeerKey,
			InviteType:  domain.InviteTypeDefault,
			Permissions: list.AclPermissionsReader,
		}, func() error {
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, inviteInfo, info)
	})
	t.Run("generate invite request join, fail to send", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		acc, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().AccountID().Return(acc.SignKey.GetPublic().Account())
		profile := account.Profile{
			Id:        "profileId",
			AccountId: acc.SignKey.GetPublic().Account(),
			Name:      "Misha",
		}
		spaceDescription := spaceinfo.SpaceDescription{
			Name:      "space",
			IconImage: "icon",
		}
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceDescription)
		fx.mockFileAcl.EXPECT().GetInfoForFileSharing(spaceDescription.IconImage).Return("iconCid", nil, nil)
		fx.expectSpaceView()
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything, mock.Anything, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeDefault,
			Permissions:   list.AclPermissionsReader,
			HeldByOwner:   true,
		}
		fx.mockSpaceView.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return(inviteInfo, nil)
		fx.mockInviteObject.EXPECT().RemoveExistingInviteInfo().Return(inviteInfo, nil)
		fx.mockInviteStore.EXPECT().RemoveInvite(ctx, inviteCid).Return(nil)
		_, err = fx.inviteService.Generate(ctx, GenerateInviteParams{
			SpaceId:     "spaceId",
			Key:         acc.PeerKey,
			InviteType:  domain.InviteTypeDefault,
			Permissions: list.AclPermissionsReader,
		}, func() error {
			return fmt.Errorf("failed to send")
		})
		require.Error(t, err)
	})
	t.Run("generate invite anyone, invite exists", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		fx.expectSpaceView()
		returnedInfo := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
			HeldByOwner:   true,
		}
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{HeldByOwner: true})
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(returnedInfo)
		info, err := fx.inviteService.Generate(ctx, GenerateInviteParams{
			SpaceId:     "spaceId",
			InviteType:  domain.InviteTypeAnyone,
			Permissions: list.AclPermissionsReader,
		}, func() error {
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, returnedInfo, info)
	})

	t.Run("invite shared within the space is written to the workspace", func(t *testing.T) {
		// given a space with no invite
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectSpaceView()
		fx.expectInviteObject()
		fx.mockSpaceView.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(domain.InviteInfo{})
		acc, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().AccountID().Return(acc.SignKey.GetPublic().Account())
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockAccountService.EXPECT().ProfileInfo().Return(account.Profile{
			Id:        "profileId",
			AccountId: acc.SignKey.GetPublic().Account(),
			Name:      "Misha",
		}, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		spaceDescription := spaceinfo.SpaceDescription{Name: "space", IconImage: "icon"}
		fx.mockSpaceView.EXPECT().GetSpaceDescription().Return(spaceDescription)
		fx.mockFileAcl.EXPECT().GetInfoForFileSharing(spaceDescription.IconImage).Return("iconCid", nil, nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything, mock.Anything, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		want := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsReader,
		}
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(want).Return(nil)
		// the space view is cleared: an invite the owner held before must not survive the switch
		fx.mockSpaceView.EXPECT().RemoveExistingInviteInfo().Return(domain.InviteInfo{}, nil)

		// when an invite is generated with shareWithinSpace
		got, err := fx.inviteService.Generate(ctx, GenerateInviteParams{
			SpaceId:          "spaceId",
			Key:              acc.PeerKey,
			InviteType:       domain.InviteTypeAnyone,
			Permissions:      list.AclPermissionsReader,
			ShareWithinSpace: true,
		}, func() error {
			return nil
		})

		// then it lands in the workspace, where every member of the space can read it
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

func TestInviteService_InviteView(t *testing.T) {
	t.Run("view ok", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		acc, err := accountdata.NewRandom()
		require.NoError(t, err)
		rawKey, err := acc.PeerKey.Marshall()
		require.NoError(t, err)
		payload := &model.InvitePayload{
			CreatorIdentity: acc.SignKey.GetPublic().Account(),
			CreatorName:     "Misha",
			SpaceName:       "spaceName",
			AclKey:          rawKey,
			SpaceId:         "spaceId",
			SpaceIconCid:    "spaceIconCid",
			SpaceUxType:     uint32(model.SpaceUxType_Data),
			InviteType:      model.InviteType_WithoutApprove,
		}
		expectedView := domain.InviteView{
			InviteType:   domain.InviteTypeAnyone,
			SpaceId:      "spaceId",
			SpaceName:    "spaceName",
			SpaceIconCid: "spaceIconCid",
			SpaceUxType:  model.SpaceUxType_Data,
			SpaceType:    model.SpaceType_SpaceTypeRegular,
			CreatorName:  "Misha",
			AclKey:       rawKey,
		}
		marshaled, err := payload.Marshal()
		require.NoError(t, err)
		signature, err := acc.SignKey.Sign(marshaled)
		require.NoError(t, err)
		invite := &model.Invite{
			Payload:   marshaled,
			Signature: signature,
		}
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().GetInvite(ctx, inviteCid, inviteKey).Return(invite, nil)
		fx.mockFileAcl.EXPECT().StoreFileKeys(mock.Anything, mock.Anything).Return(nil)
		view, err := fx.inviteService.View(ctx, inviteCid, inviteKey)
		require.NoError(t, err)
		require.Equal(t, expectedView, view)
	})
}

var ctx = context.Background()

type fixture struct {
	*inviteService
	a                  *app.App
	ctrl               *gomock.Controller
	mockInviteStore    *mock_invitestore.MockService
	mockFileAcl        *mock_fileacl.MockService
	mockAccountService *mock_account.MockService
	mockSpaceService   *mock_space.MockService
	mockTechSpace      *mock_techspace.MockTechSpace
	mockSpaceView      *mock_techspace.MockSpaceView
	mockSpace          *mock_clientspace.MockSpace
	mockInviteObject   *mock_domain.MockInviteObject
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(nil)
	mockInviteStore := mock_invitestore.NewMockService(t)
	mockFileAcl := mock_fileacl.NewMockService(t)
	mockAccountService := mock_account.NewMockService(t)
	mockSpaceService := mock_space.NewMockService(t)
	mockTechSpace := mock_techspace.NewMockTechSpace(t)
	mockSpaceView := mock_techspace.NewMockSpaceView(t)
	mockSpace := mock_clientspace.NewMockSpace(t)
	mockInviteObject := mock_domain.NewMockInviteObject(t)
	fx := &fixture{
		inviteService:      New().(*inviteService),
		a:                  new(app.App),
		ctrl:               ctrl,
		mockInviteStore:    mockInviteStore,
		mockFileAcl:        mockFileAcl,
		mockAccountService: mockAccountService,
		mockSpaceService:   mockSpaceService,
		mockTechSpace:      mockTechSpace,
		mockSpaceView:      mockSpaceView,
		mockSpace:          mockSpace,
		mockInviteObject:   mockInviteObject,
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockInviteStore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockFileAcl)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockAccountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceService)).
		Register(fx)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}
