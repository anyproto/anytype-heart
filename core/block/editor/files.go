package editor

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	fileblock "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (f *ObjectFactory) newFile(sb smartblock.SmartBlock) *File {
	basicComponent := basic.NewBasic(sb, f.objectStore, f.layoutConverter)
	return &File{
		SmartBlock:      sb,
		DetailsSettable: basicComponent,
	}
}

type File struct {
	smartblock.SmartBlock
	basic.DetailsSettable
}

func (p *File) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			if len(ctx.ObjectTypeKeys) > 0 && len(ctx.State.ObjectTypeKeys()) == 0 {
				ctx.State.SetObjectTypeKeys(ctx.ObjectTypeKeys)
			}

			details := ctx.State.CombinedDetails()
			fileType := fileblock.DetectTypeByMIME(pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()))

			fname := pbtypes.GetString(details, bundle.RelationKeyName.String())
			ext := pbtypes.GetString(details, bundle.RelationKeyFileExt.String())

			if ext != "" && !strings.HasSuffix(fname, "."+ext) {
				fname = fname + "." + ext
			}

			var blocks []*model.Block
			blocks = append(blocks, &model.Block{
				Id: "file",
				Content: &model.BlockContentOfFile{
					File: &model.BlockContentFile{
						Name:    fname,
						Mime:    pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()),
						Hash:    p.Id(),
						Type:    fileType,
						Size_:   int64(pbtypes.GetFloat64(details, bundle.RelationKeySizeInBytes.String())),
						State:   model.BlockContentFile_Done,
						AddedAt: int64(pbtypes.GetFloat64(details, bundle.RelationKeyFileMimeType.String())),
					},
				}})

			switch fileType {
			case model.BlockContentFile_Image:
				if pbtypes.GetInt64(details, bundle.RelationKeyWidthInPixels.String()) != 0 {
					blocks = append(blocks, &model.Block{
						Id: "rel1",
						Content: &model.BlockContentOfRelation{
							Relation: &model.BlockContentRelation{
								Key: bundle.RelationKeyWidthInPixels.String(),
							},
						},
					})
				}

				if pbtypes.GetInt64(details, bundle.RelationKeyHeightInPixels.String()) != 0 {
					blocks = append(blocks, &model.Block{
						Id: "rel2",
						Content: &model.BlockContentOfRelation{
							Relation: &model.BlockContentRelation{
								Key: bundle.RelationKeyHeightInPixels.String(),
							},
						},
					})
				}

				if pbtypes.GetString(details, bundle.RelationKeyCamera.String()) != "" {
					blocks = append(blocks, &model.Block{
						Id: "rel3",
						Content: &model.BlockContentOfRelation{
							Relation: &model.BlockContentRelation{
								Key: bundle.RelationKeyCamera.String(),
							},
						},
					})
				}

				if pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()) != 0 {
					blocks = append(blocks, &model.Block{
						Id: "rel4",
						Content: &model.BlockContentOfRelation{
							Relation: &model.BlockContentRelation{
								Key: bundle.RelationKeySizeInBytes.String(),
							},
						},
					})
				}
				if pbtypes.GetString(details, bundle.RelationKeyMediaArtistName.String()) != "" {
					blocks = append(blocks, &model.Block{
						Id: "rel6",
						Content: &model.BlockContentOfRelation{
							Relation: &model.BlockContentRelation{
								Key: bundle.RelationKeyMediaArtistName.String(),
							},
						},
					})
				}
				if pbtypes.GetString(details, bundle.RelationKeyMediaArtistURL.String()) != "" {
					blocks = append(blocks, &model.Block{
						Id: "rel7",
						Content: &model.BlockContentOfRelation{
							Relation: &model.BlockContentRelation{
								Key: bundle.RelationKeyMediaArtistURL.String(),
							},
						},
					})
				}
			default:
				blocks = append(blocks,
					[]*model.Block{
						{
							Id: "rel4",
							Content: &model.BlockContentOfRelation{
								Relation: &model.BlockContentRelation{
									Key: bundle.RelationKeySizeInBytes.String(),
								},
							},
						},
					}...)
			}

			template.InitTemplate(s,
				template.WithEmpty,
				template.WithTitle,
				template.WithDefaultFeaturedRelations,
				template.WithFeaturedRelations,
				template.WithRootBlocks(blocks),
				template.WithAllBlocksEditsRestricted,
			)
		},
	}
}

func (p *File) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (p *File) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != coresb.SmartBlockTypeFile {
		return fmt.Errorf("source type should be a file")
	}

	if ctx.BuildOpts.DisableRemoteLoad {
		ctx.Ctx = context.WithValue(ctx.Ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
	}
	return p.SmartBlock.Init(ctx)
}
