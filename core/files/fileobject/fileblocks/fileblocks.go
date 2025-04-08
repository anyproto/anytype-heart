package fileblocks

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	fileblock "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func InitEmptyFileState(st *state.State) {
	template.InitTemplate(st,
		template.WithEmpty,
		template.WithTitle,
		template.WithFeaturedRelationsBlock,
		template.WithAllBlocksEditsRestricted,
	)
}

func AddFileBlocks(st *state.State, details *domain.Details, objectId string) error {
	name := details.GetString(bundle.RelationKeyName)
	fileType := fileblock.DetectTypeByMIME(name, details.GetString(bundle.RelationKeyFileMimeType))

	if fileType == model.BlockContentFile_Image {
		st.SetDetailAndBundledRelation(bundle.RelationKeyIconImage, domain.String(objectId))
	}

	blocks := buildFileBlocks(details, objectId, name, fileType)

	for _, b := range blocks {
		if st.Exists(b.Id) {
			st.Set(simple.New(b))
		} else {
			st.Add(simple.New(b))
			err := st.InsertTo(st.RootId(), model.Block_Inner, b.Id)
			if err != nil {
				return fmt.Errorf("failed to insert file block: %w", err)
			}
		}
	}
	template.WithAllBlocksEditsRestricted(st)
	return nil
}

func buildFileBlocks(details *domain.Details, objectId, name string, fileType model.BlockContentFileType) []*model.Block {
	var blocks []*model.Block
	blocks = append(blocks, &model.Block{
		Id: "file",
		Content: &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:           name,
				Mime:           details.GetString(bundle.RelationKeyFileMimeType),
				TargetObjectId: objectId,
				Type:           fileType,
				Size_:          details.GetInt64(bundle.RelationKeySizeInBytes),
				State:          model.BlockContentFile_Done,
				AddedAt:        details.GetInt64(bundle.RelationKeyAddedDate),
			},
		}}, makeFileInfoBlock(), makeRelationBlock(bundle.RelationKeyDescription), makeRelationBlock(bundle.RelationKeyFileExt))

	switch fileType {
	case model.BlockContentFile_Image:
		for _, relKey := range []domain.RelationKey{
			bundle.RelationKeyWidthInPixels,
			bundle.RelationKeyHeightInPixels,
			bundle.RelationKeyCamera,
			bundle.RelationKeyMediaArtistName,
			bundle.RelationKeyMediaArtistURL,
		} {
			if notEmpty(details, relKey) {
				blocks = append(blocks, makeRelationBlock(relKey))
			}
		}
	case model.BlockContentFile_Audio:
		for _, relKey := range []domain.RelationKey{
			bundle.RelationKeyArtist,
			bundle.RelationKeyAudioAlbum,
			bundle.RelationKeyAudioAlbumTrackNumber,
			bundle.RelationKeyAudioGenre,
			bundle.RelationKeyAudioLyrics,
			bundle.RelationKeyReleasedYear,
		} {
			if notEmpty(details, relKey) {
				blocks = append(blocks, makeRelationBlock(relKey))
			}
		}
	case model.BlockContentFile_Video:
		for _, relKey := range []domain.RelationKey{
			bundle.RelationKeyWidthInPixels,
			bundle.RelationKeyHeightInPixels,
			bundle.RelationKeyCamera,
			bundle.RelationKeyCameraIso,
			bundle.RelationKeyAperture,
			bundle.RelationKeyExposure,
		} {
			if notEmpty(details, relKey) {
				blocks = append(blocks, makeRelationBlock(relKey))
			}
		}
	}

	for _, relKey := range []domain.RelationKey{
		bundle.RelationKeySizeInBytes,
		bundle.RelationKeyOrigin,
		bundle.RelationKeyImportType,
		bundle.RelationKeyAddedDate,
	} {
		if details.GetInt64(relKey) != 0 {
			blocks = append(blocks, makeRelationBlock(relKey))
		}
	}

	return blocks
}

func makeFileInfoBlock() *model.Block {
	return &model.Block{
		Id: "info",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "File Information",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{{
						Range: &model.Range{
							From: 0,
							To:   16,
						},
						Type: model.BlockContentTextMark_Bold,
					}},
				},
			},
		},
	}
}

func notEmpty(details *domain.Details, relKey domain.RelationKey) bool {
	return details.GetInt64(relKey) != 0 || details.GetString(relKey) != ""
}

func makeRelationBlock(relationKey domain.RelationKey) *model.Block {
	return &model.Block{
		Id: relationKey.String(),
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: relationKey.String(),
			},
		},
	}
}
