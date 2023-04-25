package editor

import (
	"context"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewFiles(sb smartblock.SmartBlock) *Files {
	return &Files{
		SmartBlock: sb,
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
	if strings.HasPrefix(mime, "application/pdf") {
		return model.BlockContentFile_PDF
	}

	return model.BlockContentFile_File
}

func (p *Files) SetDetails(ctx *session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) error {
	st := p.NewStateCtx(ctx)
	det := pbtypes.CopyStruct(st.Details())
	for _, d := range details {
		if d.Key == bundle.RelationKeyFileSyncStatus.String() {
			det.Fields[d.Key] = d.Value
		}
	}
	st.SetDetails(det)
	return p.Apply(st)
}

func (p *Files) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_File {
		return fmt.Errorf("source type should be a file")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	var loadCtx = ctx.Ctx
	if ctx.BuildOpts.DisableRemoteLoad {
		loadCtx = context.WithValue(loadCtx, filestorage.CtxKeyRemoteLoadDisabled, true)
	}
	doc, err := ctx.Source.ReadDoc(loadCtx, nil, true)
	if err != nil {
		return err
	}
	d := doc.CombinedDetails()
	fileType := detectFileType(pbtypes.GetString(d, bundle.RelationKeyFileMimeType.String()))

	fname := pbtypes.GetString(d, bundle.RelationKeyName.String())
	ext := pbtypes.GetString(d, bundle.RelationKeyFileExt.String())

	if ext != "" && !strings.HasSuffix(fname, "."+ext) {
		fname = fname + "." + ext
	}

	var blocks []*model.Block
	blocks = append(blocks, &model.Block{
		Id: "file",
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:    fname,
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
		if pbtypes.GetString(d, bundle.RelationKeyMediaArtistName.String()) != "" {
			blocks = append(blocks, &model.Block{
				Id: "rel6",
				Content: &model.BlockContentOfRelation{
					Relation: &model.BlockContentRelation{
						Key: bundle.RelationKeyMediaArtistName.String(),
					},
				},
			})
		}
		if pbtypes.GetString(d, bundle.RelationKeyMediaArtistURL.String()) != "" {
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

	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyFile.URL()}, model.ObjectType_file),
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithRootBlocks(blocks),
		template.WithAllBlocksEditsRestricted,
	)
}
