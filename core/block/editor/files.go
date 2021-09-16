package editor

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewFiles() *Files {
	return &Files{
		SmartBlock: smartblock.New(),
	}
}

type Files struct {
	smartblock.SmartBlock
}

func detectFileType(mime string) model.BlockContentFileType {
	if strings.HasPrefix(mime, "image") {
		return model.BlockContentFile_Image
	}
	if strings.HasPrefix(mime, "video") {
		return model.BlockContentFile_Video
	}
	if strings.HasPrefix(mime, "audio") {
		return model.BlockContentFile_Audio
	}
	return model.BlockContentFile_File
}

func (p *Files) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_File {
		return fmt.Errorf("source type should be a file")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	doc, err := ctx.Source.ReadDoc(nil, true)
	if err != nil {
		return err
	}
	d := doc.Details()
	fileType := detectFileType(pbtypes.GetString(d, bundle.RelationKeyFileMimeType.String()))

	var blocks []*model.Block
	blocks = append(blocks, &model.Block{
		Id: "file",
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:    pbtypes.GetString(d, bundle.RelationKeyName.String()),
				Mime:    pbtypes.GetString(d, bundle.RelationKeyFileMimeType.String()),
				Hash:    p.Id(),
				Type:    detectFileType(pbtypes.GetString(d, bundle.RelationKeyFileMimeType.String())),
				Size_:   int64(pbtypes.GetFloat64(d, bundle.RelationKeySizeInBytes.String())),
				State:   model.BlockContentFile_Done,
				AddedAt: int64(pbtypes.GetFloat64(d, bundle.RelationKeyFileMimeType.String())),
			},
		}})

	switch fileType {
	case model.BlockContentFile_Image:
		if pbtypes.GetInt64(d, bundle.RelationKeyWidthInPixels.String()) != 0 {
			blocks = append(blocks, &model.Block{
				Id: "rel1",
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: bundle.RelationKeyWidthInPixels.String(),
					},
				},
			})
		}

		if pbtypes.GetInt64(d, bundle.RelationKeyHeightInPixels.String()) != 0 {
			blocks = append(blocks, &model.Block{
				Id: "rel2",
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: bundle.RelationKeyHeightInPixels.String(),
					},
				},
			})
		}

		if pbtypes.GetString(d, bundle.RelationKeyCamera.String()) != "" {
			blocks = append(blocks, &model.Block{
				Id: "rel3",
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: bundle.RelationKeyCamera.String(),
					},
				},
			})
		}

		if pbtypes.GetInt64(d, bundle.RelationKeySizeInBytes.String()) != 0 {
			blocks = append(blocks, &model.Block{
				Id: "rel4",
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: bundle.RelationKeySizeInBytes.String(),
					},
				},
			})
		}

		if pbtypes.GetInt64(d, bundle.RelationKeySizeInBytes.String()) != 0 {
			blocks = append(blocks, &model.Block{
				Id: "rel5",
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: bundle.RelationKeySizeInBytes.String(),
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

	return template.ApplyTemplate(p, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyFile.URL()}),
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithRootBlocks(blocks),
		template.WithAllBlocksEditsRestricted,
	)
}
