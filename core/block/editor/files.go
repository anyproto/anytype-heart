package editor

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/stext"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/filestorage"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

func (f *ObjectFactory) newFile(sb smartblock.SmartBlock) *File {
	basicComponent := basic.NewBasic(sb, f.objectStore, f.layoutConverter)
	return &File{
		SmartBlock:    sb,
		AllOperations: basicComponent,
		Text:          stext.NewText(sb, f.objectStore, f.eventSender),
	}
}

type File struct {
	smartblock.SmartBlock
	basic.AllOperations
	stext.Text
}

func (p *File) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			if len(ctx.ObjectTypeKeys) > 0 && len(ctx.State.ObjectTypeKeys()) == 0 {
				ctx.State.SetObjectTypeKeys(ctx.ObjectTypeKeys)
			}

			template.InitTemplate(s,
				template.WithEmpty,
				template.WithTitle,
				template.WithDefaultFeaturedRelations,
				template.WithFeaturedRelations,
				template.WithAllBlocksEditsRestricted,
			)
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
