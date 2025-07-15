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

func (fx *fixture) expectInviteObject() {
	fx.mockSpaceService.EXPECT().Get(ctx, "spaceId").Return(fx.mockSpace, nil)
	fx.mockSpace.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
		Workspace: "workspaceId",
	})
	fx.mockSpace.EXPECT().Do("workspaceId", mock.Anything).RunAndReturn(func(s string, f func(smartblock.SmartBlock) error) error {
		return f(mockInviteObject{SmartBlock: smarttest.New("root"), MockInviteObject: fx.mockInviteObject})
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
	t.Run("get current no migration", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		returnedInfo := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(returnedInfo)
		info, err := fx.GetCurrent(ctx, "spaceId")
		require.NoError(t, err)
		require.Equal(t, returnedInfo, info)
	})
}

func TestInviteService_RemoveExisting(t *testing.T) {
	t.Run("remove ok", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
		res, err := cidutil.NewCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		returnedInfo := domain.InviteInfo{
			InviteFileCid: res,
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		invCid, err := cid.Decode(returnedInfo.InviteFileCid)
		require.NoError(t, err)
		fx.mockInviteObject.EXPECT().RemoveExistingInviteInfo().Return(returnedInfo, nil)
		fx.mockInviteStore.EXPECT().RemoveInvite(ctx, invCid).Return(nil)
		err = fx.RemoveExisting(ctx, "spaceId")
		require.NoError(t, err)
	})
}

func TestInviteService_Generate(t *testing.T) {
	t.Run("generate invite anyone, no existing info", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.ctrl.Finish()
		fx.expectInviteObject()
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
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
				return f(fx.mockSpaceView)
			})
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsReader,
		}
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
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
				return f(fx.mockSpaceView)
			})
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsReader,
		}
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
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
				return f(fx.mockSpaceView)
			})
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeDefault,
			Permissions:   list.AclPermissionsReader,
		}
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
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
				return f(fx.mockSpaceView)
			})
		fx.mockAccountService.EXPECT().ProfileInfo().Return(profile, nil)
		fx.mockAccountService.EXPECT().SignData(mock.Anything).Return([]byte("signature"), nil)
		inviteCid, err := newCidFromBytes([]byte("fileCid"))
		require.NoError(t, err)
		inviteKey := crypto.NewAES()
		fx.mockInviteStore.EXPECT().StoreInvite(ctx, mock.Anything).Return(inviteCid, inviteKey, nil)
		inviteFileKeyRaw, err := encode.EncodeKeyToBase58(inviteKey)
		require.NoError(t, err)
		inviteInfo := domain.InviteInfo{
			InviteFileCid: inviteCid.String(),
			InviteFileKey: inviteFileKeyRaw,
			InviteType:    domain.InviteTypeDefault,
			Permissions:   list.AclPermissionsReader,
		}
		fx.mockInviteObject.EXPECT().SetInviteFileInfo(inviteInfo).Return(nil)
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
		returnedInfo := domain.InviteInfo{
			InviteFileCid: "fileCid",
			InviteFileKey: "fileKey",
			InviteType:    domain.InviteTypeAnyone,
			Permissions:   list.AclPermissionsWriter,
		}
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteObject.EXPECT().GetExistingInviteInfo().Return(returnedInfo)
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
			InviteType:      model.InviteType_WithoutApprove,
		}
		expectedView := domain.InviteView{
			InviteType:   domain.InviteTypeAnyone,
			SpaceId:      "spaceId",
			SpaceName:    "spaceName",
			SpaceIconCid: "spaceIconCid",
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
