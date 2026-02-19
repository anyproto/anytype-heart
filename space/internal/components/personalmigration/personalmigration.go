package personalmigration

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const (
	CName = "common.components.personalmigration"
)

var log = logger.NewNamed(CName)

type Runner interface {
	app.ComponentRunnable
	WaitProfile(ctx context.Context) error
}

func New() Runner {
	return &runner{}
}

type fileObjectGetter interface {
	app.Component
	DoFileWaitLoad(ctx context.Context, objectId string, proc func(object fileobject.FileObject) error) error
	Create(ctx context.Context, spaceId string, req filemodels.CreateRequest) (id string, object *domain.Details, err error)
}

type runner struct {
	spaceLoader      spaceloader.SpaceLoader
	techSpace        techspace.TechSpace
	fileObjectGetter fileObjectGetter

	ctx                context.Context
	cancel             context.CancelFunc
	spc                clientspace.Space
	loadErr            error
	waitMigrateProfile chan struct{}
	waitMigrate        chan struct{}
	started            bool

	app.ComponentRunnable
}

func (r *runner) Name() string {
	return CName
}

func (r *runner) Init(a *app.App) error {
	r.spaceLoader = app.MustComponent[spaceloader.SpaceLoader](a)
	r.techSpace = app.MustComponent[techspace.TechSpace](a)
	r.fileObjectGetter = app.MustComponent[fileObjectGetter](a)

	r.waitMigrateProfile = make(chan struct{})
	r.waitMigrate = make(chan struct{})
	return nil
}

func (r *runner) Run(context.Context) error {
	r.started = true
	r.ctx, r.cancel = context.WithCancel(context.Background())
	go r.runMigrations()
	return nil
}

func (r *runner) Close(context.Context) error {
	if r.started {
		r.cancel()
	}
	<-r.waitMigrate
	return nil
}

func (r *runner) migrateProfile() (hasIcon bool, oldIcon string, err error) {
	defer close(r.waitMigrateProfile)
	shouldMigrateProfile := true
	err = r.techSpace.DoAccountObject(r.ctx, func(accountObject techspace.AccountObject) error {
		res, err := accountObject.GetAnalyticsId()
		if err != nil {
			return err
		}
		if res != "" {
			shouldMigrateProfile = false
			hasIcon, err = accountObject.IsIconMigrated()
			return err
		}
		return nil
	})
	if !shouldMigrateProfile && hasIcon {
		return
	}
	space := r.spc
	ids := space.DerivedIDs()
	var details *domain.Details
	err = space.DoCtx(r.ctx, ids.Profile, func(sb smartblock.SmartBlock) error {
		details = sb.Details().CopyOnlyKeys(
			bundle.RelationKeyName,
			bundle.RelationKeyDescription,
			bundle.RelationKeyIconOption,
		)
		oldIcon = sb.Details().GetString(bundle.RelationKeyIconImage)
		return nil
	})
	if err != nil {
		return
	}
	if !shouldMigrateProfile {
		return
	}
	var analyticsId string
	err = space.DoCtx(r.ctx, ids.Workspace, func(sb smartblock.SmartBlock) error {
		analyticsId = sb.NewState().GetSetting(state.SettingsAnalyticsId).GetStringValue()
		return nil
	})
	if err != nil {
		return
	}
	err = r.techSpace.DoAccountObject(r.ctx, func(accountObject techspace.AccountObject) error {
		err = accountObject.SetAnalyticsId(analyticsId)
		if err != nil {
			return err
		}
		return accountObject.SetProfileDetails(details)
	})
	return
}

func (r *runner) migrateIcon(oldIcon string) (err error) {
	if oldIcon == "" {
		err = r.techSpace.DoAccountObject(r.ctx, func(accountObject techspace.AccountObject) error {
			return accountObject.MigrateIconImage("")
		})
		return
	}
	err = r.fileObjectGetter.DoFileWaitLoad(r.ctx, oldIcon, func(_ fileobject.FileObject) error {
		return nil
	})
	if err != nil {
		return
	}
	var fileInfo state.FileInfo
	err = r.spc.DoCtx(r.ctx, oldIcon, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		fileInfo = st.GetFileInfo()
		return nil
	})
	if err != nil {
		return err
	}
	id, _, err := r.fileObjectGetter.Create(r.ctx, r.spc.Id(), filemodels.CreateRequest{
		FileId:         fileInfo.FileId,
		EncryptionKeys: fileInfo.EncryptionKeys,
		ObjectOrigin:   objectorigin.None(),
	})
	if err != nil {
		return
	}
	err = r.techSpace.DoAccountObject(r.ctx, func(accountObject techspace.AccountObject) error {
		return accountObject.MigrateIconImage(id)
	})
	return
}

func (r *runner) runMigrations() {
	defer close(r.waitMigrate)
	r.spc, r.loadErr = r.spaceLoader.WaitLoad(r.ctx)
	if r.loadErr != nil {
		log.Error("failed to load space", zap.Error(r.loadErr))
		return
	}
	hasIcon, iconId, err := r.migrateProfile()
	if err != nil {
		log.Error("failed to migrate profile", zap.String("spaceId", r.spc.Id()))
		return
	}
	if !hasIcon {
		err = r.migrateIcon(iconId)
		if err != nil {
			log.Error("failed to migrate icon", zap.String("spaceId", r.spc.Id()))
		}
	}
}

func (r *runner) WaitProfile(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.waitMigrateProfile:
		return nil
	}
}
