package editor

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/mill"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	if mill.IsImage(mime) {
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

func (p *Files) SetDetails(ctx session.Context, details []*pb.RpcObjectSetDetailsDetail, showEvent bool) error {
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
	if ctx.Source.Type() != coresb.SmartBlockTypeFile {
		return fmt.Errorf("source type should be a file")
	}

	if ctx.BuildOpts.DisableRemoteLoad {
		ctx.Ctx = context.WithValue(ctx.Ctx, filestorage.CtxKeyRemoteLoadDisabled, true)
	}
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	details := p.NewState().CombinedDetails()
	fileType := detectFileType(pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()))

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
				Type:    detectFileType(pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String())),
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

	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithFeaturedRelations,
		template.WithRootBlocks(blocks),
		template.WithAllBlocksEditsRestricted,
	)
}
