package editor

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	fileobject2 "github.com/anyproto/anytype-heart/core/block/editor/fileobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/fileobject/fileblocks"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/reconciler"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// required relations for files beside the bundle.RequiredInternalRelations
var fileRequiredRelations = append(pageRequiredRelations, []domain.RelationKey{
	bundle.RelationKeyFileBackupStatus,
	bundle.RelationKeyFileSyncStatus,
}...)

func (f *ObjectFactory) newFile(spaceId string, sb smartblock.SmartBlock) *File {
	store := f.objectStore.SpaceIndex(spaceId)
	basicComponent := basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService)
	return &File{
		SmartBlock:        sb,
		ChangeReceiver:    sb.(source.ChangeReceiver),
		FileObject:        fileobject2.NewFileObject(sb, f.fileService),
		AllOperations:     basicComponent,
		Text:              stext.NewText(sb, store, f.eventSender),
		fileObjectService: f.fileObjectService,
		reconciler:        f.fileReconciler,
		accountService:    f.accountService,
		fileService:       f.fileService,
	}
}

type File struct {
	smartblock.SmartBlock
	fileobject2.FileObject
	source.ChangeReceiver
	basic.AllOperations
	stext.Text
	fileObjectService fileobject.Service
	reconciler        reconciler.Reconciler
	fileService       files.Service
	accountService    accountService
}

func (f *File) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			if len(ctx.ObjectTypeKeys) > 0 && len(ctx.State.ObjectTypeKeys()) == 0 {
				ctx.State.SetObjectTypeKeys(ctx.ObjectTypeKeys)
			}

			// Other blocks added:
			// - While creating file object, if we use synchronous metadata indexing mode
			// - In background metadata indexer, if we use asynchronous metadata indexing mode
			//
			// See fileobject.Service
			fileblocks.InitEmptyFileState(ctx.State)
		},
	}
}

func (f *File) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (f *File) Init(ctx *smartblock.InitContext) error {
	if ctx.Source.Type() != coresb.SmartBlockTypeFileObject {
		return fmt.Errorf("source type should be a file")
	}

	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, fileRequiredRelations...)

	if ctx.BuildOpts.DisableRemoteLoad {
		ctx.Ctx = context.WithValue(ctx.Ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
	}

	err := f.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	fullId := domain.FullID{SpaceID: f.SpaceID(), ObjectID: f.Id()}

	f.SmartBlock.AddHook(f.reconciler.FileObjectHook(fullId), smartblock.HookBeforeApply)

	creator := ctx.State.LocalDetails().GetString(bundle.RelationKeyCreator)
	myParticipantId := f.accountService.MyParticipantId(f.SpaceID())

	if !ctx.IsNewObject && creator == myParticipantId {
		// Run in a goroutine to prevent deadlocks when filesync updates file status before file is loaded into cache
		go func() {
			err = f.fileObjectService.EnsureFileAddedToSyncQueue(fullId, ctx.State.Details())
			if err != nil {
				log.Errorf("failed to ensure file added to sync queue: %v", err)
			}
		}()
		f.AddHook(func(applyInfo smartblock.ApplyInfo) error {
			go func() {
				err = f.fileObjectService.EnsureFileAddedToSyncQueue(fullId, applyInfo.State.Details())
				if err != nil {
					log.Errorf("failed to ensure file added to sync queue: %v", err)
				}
			}()
			return nil
		}, smartblock.HookOnStateRebuild)
	}

	if !ctx.IsNewObject {
		fileId := domain.FullFileId{
			FileId:  domain.FileId(ctx.State.Details().GetString(bundle.RelationKeyFileId)),
			SpaceId: f.SpaceID(),
		}
		// Migrate file to the new file index. The old file index was in the separate database. Now all file info is stored
		// in the object store directly
		if len(ctx.State.Details().GetStringList(bundle.RelationKeyFileVariantIds)) == 0 {
			infos, err := f.fileService.GetFileVariants(ctx.Ctx, fileId, ctx.State.GetFileInfo().EncryptionKeys)
			if err != nil {
				log.Errorf("get infos for indexing: %v", err)
			}
			if len(infos) > 0 {
				err = filemodels.InjectVariantsToDetails(infos, ctx.State)
				if err != nil {
					return fmt.Errorf("inject variants: %w", err)
				}
			}
		}
	}

	return nil
}

func (f *File) InjectVirtualBlocks(objectId string, view *model.ObjectView) {
	if view.Type != model.SmartBlockType_FileObject {
		return
	}

	var details *domain.Details
	for _, det := range view.Details {
		if det.Id == objectId {
			details = domain.NewDetailsFromProto(det.Details)
			break
		}
	}
	if details == nil {
		return
	}

	st := state.NewDoc(objectId, nil).NewState()
	st.SetDetails(details)
	fileblocks.InitEmptyFileState(st)
	if err := fileblocks.AddFileBlocks(st, details, objectId); err != nil {
		log.Errorf("failed to inject virtual file blocks: %v", err)
		return
	}

	view.Blocks = st.Blocks()
}
