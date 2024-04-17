package editor

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/filestorage"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func (f *ObjectFactory) newFile(sb smartblock.SmartBlock) *File {
	basicComponent := basic.NewBasic(sb, f.objectStore, f.layoutConverter)
	return &File{
		SmartBlock:     sb,
		ChangeReceiver: sb.(source.ChangeReceiver),
		AllOperations:  basicComponent,
		Text:           stext.NewText(sb, f.objectStore, f.eventSender), fileObjectService: f.fileObjectService,
	}
}

type File struct {
	smartblock.SmartBlock
	source.ChangeReceiver
	basic.AllOperations
	stext.Text
	fileObjectService fileobject.Service
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
			f.fileObjectService.InitEmptyFileState(ctx.State)
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

	if ctx.BuildOpts.DisableRemoteLoad {
		ctx.Ctx = context.WithValue(ctx.Ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
	}

	err := f.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	if !ctx.IsNewObject {
		err = f.fileObjectService.EnsureFileAddedToSyncQueue(domain.FullID{ObjectID: f.Id(), SpaceID: f.SpaceID()}, ctx.State.Details())
		if err != nil {
			log.Errorf("failed to ensure file added to sync queue: %v", err)
		}
		f.AddHook(func(applyInfo smartblock.ApplyInfo) error {
			return f.fileObjectService.EnsureFileAddedToSyncQueue(domain.FullID{ObjectID: f.Id(), SpaceID: f.SpaceID()}, applyInfo.State.Details())
		}, smartblock.HookOnStateRebuild)
	}
	return nil
}
