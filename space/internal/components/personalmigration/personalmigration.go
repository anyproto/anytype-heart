package personalmigration

import (
	"context"
	"errors"
	"io"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileuploader"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

type uploader interface {
	UploadFile(ctx context.Context, spaceId string, fileId string, file files.File) (string, error)
}

type fileObjectGetter interface {
	GetFileIdFromObjectWaitLoad(ctx context.Context, objectId string) (domain.FullFileId, error)
}

type runner struct {
	store            objectstore.ObjectStore
	spaceLoader      spaceloader.SpaceLoader
	techSpace        techspace.TechSpace
	fileObjectGetter fileObjectGetter
	fileGetter       files.Service
	fileUploader     fileuploader.Service

	ctx                context.Context
	cancel             context.CancelFunc
	spc                clientspace.Space
	loadErr            error
	waitLoad           chan struct{}
	waitMigrateProfile chan struct{}
	waitMigrate        chan struct{}
	started            bool
	isPersonal         bool

	app.ComponentRunnable
}

func (r *runner) Name() string {
	return CName
}

func (r *runner) Init(a *app.App) error {
	r.store = app.MustComponent[objectstore.ObjectStore](a)
	r.spaceLoader = app.MustComponent[spaceloader.SpaceLoader](a)
	r.techSpace = app.MustComponent[techspace.TechSpace](a)
	r.fileObjectGetter = app.MustComponent[fileObjectGetter](a)
	r.fileGetter = app.MustComponent[files.Service](a)
	r.fileUploader = app.MustComponent[fileuploader.Service](a)

	r.waitMigrateProfile = make(chan struct{})
	r.waitMigrate = make(chan struct{})
	r.waitLoad = make(chan struct{})
	return nil
}

func (r *runner) Run(context.Context) error {
	r.started = true
	r.ctx, r.cancel = context.WithCancel(context.Background())
	go r.waitSpace()
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

func (r *runner) waitSpace() {
	r.spc, r.loadErr = r.spaceLoader.WaitLoad(r.ctx)
	close(r.waitLoad)
}

func (r *runner) migrateProfile() (hasIcon bool, oldIcon string, err error) {
	defer close(r.waitMigrateProfile)
	space := r.spc
	ids := space.DerivedIDs()
	var details *types.Struct
	err = space.DoCtx(r.ctx, ids.Profile, func(sb smartblock.SmartBlock) error {
		details = pbtypes.CopyStructFields(sb.CombinedDetails(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyDescription.String())
		oldIcon = sb.CombinedDetails().GetFields()[bundle.RelationKeyIconImage.String()].GetStringValue()
		return nil
	})
	if err != nil {
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
		if accountObject.CombinedDetails().GetFields()[bundle.RelationKeyName.String()].GetStringValue() != "" {
			hasIcon = accountObject.CombinedDetails().GetFields()[bundle.RelationKeyIconImage.String()].GetStringValue() != ""
			return nil
		}
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
		return
	}
	fileId, err := r.fileObjectGetter.GetFileIdFromObjectWaitLoad(r.ctx, oldIcon)
	if err != nil {
		return
	}
	image, err := r.fileGetter.ImageByHash(r.ctx, fileId)
	if err != nil {
		return
	}
	file, err := image.GetOriginalFile()
	if err != nil {
		return
	}
	reader, err := file.Reader(r.ctx)
	if err != nil {
		return
	}
	var (
		total []byte
		buf   = make([]byte, 1024)
	)
	for {
		n, err := reader.Read(buf)
		// can we have n bytes and err == EOF?
		total = append(total, buf[:n]...)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			break
		}
	}

	upl := r.fileUploader.NewUploader(r.spc.Id(), objectorigin.None())
	upl.SetType(model.BlockContentFile_Image)
	upl.SetBytes(total)
	res := upl.Upload(r.ctx)
	if res.Err != nil {
		return res.Err
	}
	err = r.techSpace.DoAccountObject(r.ctx, func(accountObject techspace.AccountObject) error {
		return accountObject.SetIconImage(res.FileObjectId)
	})
	return
}

func (r *runner) runMigrations() {
	defer close(r.waitMigrate)
	select {
	case <-r.ctx.Done():
		return
	case <-r.waitLoad:
		if r.loadErr != nil {
			log.Error("failed to load space", zap.Error(r.loadErr))
			return
		}
		break
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
	return
}

func (r *runner) WaitProfile(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.waitMigrateProfile:
		return nil
	}
}
