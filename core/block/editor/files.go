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
		Text:           stext.NewText(sb, f.objectStore, f.eventSender),fileObjectService: f.fileObjectService,
	}
}

type File struct {
	smartblock.SmartBlock
	source.ChangeReceiver
	basic.AllOperations
	stext.Text
	fileObjectService fileobject.Service
}

func (p *File) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
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
			p.fileObjectService.InitEmptyFileState(ctx.State)
		},
	}
}

func (p *File) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (p *File) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != coresb.SmartBlockTypeFileObject {
		return fmt.Errorf("source type should be a file")
	}

	if ctx.BuildOpts.DisableRemoteLoad {
		ctx.Ctx = context.WithValue(ctx.Ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
	}
	return p.SmartBlock.Init(ctx)
}
