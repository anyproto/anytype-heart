package personalmigration

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/personalmigration/mock_personalmigration"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader/mock_spaceloader"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	*runner
	a             *app.App
	spaceLoader   *mock_spaceloader.MockSpaceLoader
	getter        *mock_personalmigration.MockfileObjectGetter
	techSpace     *mock_techspace.MockTechSpace
	accountObject *mock_techspace.MockAccountObject
	space         *mock_clientspace.MockSpace
	smartBlock    *smarttest.SmartTest
}

var ctx = context.Background()

func newFixture(t *testing.T) *fixture {
	a := &app.App{}
	fx := &fixture{
		runner:        New().(*runner),
		a:             a,
		spaceLoader:   mock_spaceloader.NewMockSpaceLoader(t),
		getter:        mock_personalmigration.NewMockfileObjectGetter(t),
		techSpace:     mock_techspace.NewMockTechSpace(t),
		accountObject: mock_techspace.NewMockAccountObject(t),
		space:         mock_clientspace.NewMockSpace(t),
		smartBlock:    smarttest.New("Workspace"),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.spaceLoader)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.techSpace)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.getter)).
		Register(fx)
	fx.spaceLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.space, nil)
	return fx
}

func (fx *fixture) run(t *testing.T) {
	require.NoError(t, fx.a.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, fx.a.Close(ctx))
	})
	<-fx.waitMigrate
}

func TestRunner_Run(t *testing.T) {
	t.Run("full migration", func(t *testing.T) {
		fx := newFixture(t)
		st := fx.smartBlock.NewState()
		st.SetSetting(state.SettingsAnalyticsId, pbtypes.String("analyticsId"))
		fileInfo := state.FileInfo{
			FileId: "fileId",
			EncryptionKeys: map[string]string{
				"path": "key",
			},
		}
		st.SetFileInfo(fileInfo)
		st.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:        domain.String("name"),
			bundle.RelationKeyDescription: domain.String("description"),
			bundle.RelationKeyIconImage:   domain.String("iconImage"),
		}))
		err := fx.smartBlock.Apply(st)
		require.NoError(t, err)
		fx.accountObject.EXPECT().GetAnalyticsId().Return("", nil)
		fx.techSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, f func(techspace.AccountObject) error) error {
			return f(fx.accountObject)
		})
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Profile:   "Profile",
			Workspace: "Workspace",
		})
		fx.space.EXPECT().DoCtx(mock.Anything, "Profile", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.space.EXPECT().DoCtx(mock.Anything, "Workspace", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.accountObject.EXPECT().SetAnalyticsId("analyticsId").Return(nil)
		fx.accountObject.EXPECT().SetProfileDetails(mock.Anything).Return(nil)
		fx.getter.EXPECT().DoFileWaitLoad(mock.Anything, "iconImage", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(fileobject.FileObject) error) error {
			return nil
		}).Return(nil)
		fx.space.EXPECT().DoCtx(mock.Anything, "iconImage", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.space.EXPECT().Id().Return("spaceId")
		fx.getter.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return("iconMigratedId", nil, nil)
		fx.accountObject.EXPECT().MigrateIconImage("iconMigratedId").Return(nil)
		fx.run(t)
	})
	t.Run("migrate only profile without icon", func(t *testing.T) {
		fx := newFixture(t)
		st := fx.smartBlock.NewState()
		st.SetSetting(state.SettingsAnalyticsId, pbtypes.String("analyticsId"))
		st.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:        domain.String("name"),
			bundle.RelationKeyDescription: domain.String("description"),
		}))
		err := fx.smartBlock.Apply(st)
		require.NoError(t, err)
		fx.accountObject.EXPECT().GetAnalyticsId().Return("", nil)
		fx.techSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, f func(techspace.AccountObject) error) error {
			return f(fx.accountObject)
		})
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Profile:   "Profile",
			Workspace: "Workspace",
		})
		fx.space.EXPECT().DoCtx(mock.Anything, "Profile", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.space.EXPECT().DoCtx(mock.Anything, "Workspace", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.accountObject.EXPECT().SetAnalyticsId("analyticsId").Return(nil)
		fx.accountObject.EXPECT().SetProfileDetails(mock.Anything).Return(nil)
		fx.accountObject.EXPECT().MigrateIconImage("").Return(nil)
		fx.run(t)
	})
	t.Run("already migrated but not icon", func(t *testing.T) {
		fx := newFixture(t)
		st := fx.smartBlock.NewState()
		st.SetSetting(state.SettingsAnalyticsId, pbtypes.String("analyticsId"))
		fileInfo := state.FileInfo{
			FileId: "fileId",
			EncryptionKeys: map[string]string{
				"path": "key",
			},
		}
		st.SetFileInfo(fileInfo)
		st.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:        domain.String("name"),
			bundle.RelationKeyDescription: domain.String("description"),
			bundle.RelationKeyIconImage:   domain.String("iconImage"),
		}))
		err := fx.smartBlock.Apply(st)
		require.NoError(t, err)
		fx.accountObject.EXPECT().GetAnalyticsId().Return("analyticsId", nil)
		fx.accountObject.EXPECT().IsIconMigrated().Return(false, nil)
		fx.techSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, f func(techspace.AccountObject) error) error {
			return f(fx.accountObject)
		})
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{
			Profile:   "Profile",
			Workspace: "Workspace",
		})
		fx.space.EXPECT().DoCtx(mock.Anything, "Profile", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.getter.EXPECT().DoFileWaitLoad(mock.Anything, "iconImage", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(fileobject.FileObject) error) error {
			return nil
		}).Return(nil)
		fx.space.EXPECT().DoCtx(mock.Anything, "iconImage", mock.Anything).RunAndReturn(func(ctx2 context.Context, s string, f func(smartblock.SmartBlock) error) error {
			return f(fx.smartBlock)
		}).Times(1)
		fx.space.EXPECT().Id().Return("spaceId")
		fx.getter.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return("iconMigratedId", nil, nil)
		fx.accountObject.EXPECT().MigrateIconImage("iconMigratedId").Return(nil)
		fx.run(t)
	})
	t.Run("already migrated fully", func(t *testing.T) {
		fx := newFixture(t)
		fx.accountObject.EXPECT().GetAnalyticsId().Return("analyticsId", nil)
		fx.accountObject.EXPECT().IsIconMigrated().Return(true, nil)
		fx.techSpace.EXPECT().DoAccountObject(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, f func(techspace.AccountObject) error) error {
			return f(fx.accountObject)
		})
		fx.run(t)
	})
}
