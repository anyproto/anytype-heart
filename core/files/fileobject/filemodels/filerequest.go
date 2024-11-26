package filemodels

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type CreateRequest struct {
	FileId            domain.FileId
	EncryptionKeys    map[string]string
	ObjectOrigin      objectorigin.ObjectOrigin
	ImageKind         model.ImageKind
	AdditionalDetails *domain.Details

	FileVariants          []*storage.FileInfo
	AsyncMetadataIndexing bool
}

var (
	ErrObjectNotFound = fmt.Errorf("file object not found")
	ErrEmptyFileId    = fmt.Errorf("empty file id")
)

func InjectVariantsToDetails(infos []*storage.FileInfo, st *state.State) error {
	if len(infos) == 0 {
		return fmt.Errorf("empty info list")
	}
	var (
		variantIds []string
		keys       []string
		widths     []int
		checksums  []string
		mills      []string
		options    []string
		paths      []string
	)

	keysInfo := st.GetFileInfo().EncryptionKeys

	st.SetDetailAndBundledRelation(bundle.RelationKeyFileSourceChecksum, domain.String(infos[0].Source))
	for _, info := range infos {
		variantIds = append(variantIds, info.Hash)
		checksums = append(checksums, info.Checksum)
		mills = append(mills, info.Mill)
		widths = append(widths, int(pbtypes.GetInt64(info.Meta, "width")))
		keys = append(keys, keysInfo[info.Path])
		options = append(options, info.Opts)
		paths = append(paths, info.Path)
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantIds, domain.StringList(variantIds))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantPaths, domain.StringList(paths))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantChecksums, domain.StringList(checksums))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantMills, domain.StringList(mills))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantWidths, domain.Int64List(widths))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantKeys, domain.StringList(keys))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantOptions, domain.StringList(options))
	return nil
}

func GetFileInfosFromDetails(details *domain.Details) []*storage.FileInfo {
	variantsList := details.GetStringList(bundle.RelationKeyFileVariantIds)
	sourceChecksum := details.GetString(bundle.RelationKeyFileSourceChecksum)
	addedAt := details.GetInt64(bundle.RelationKeyAddedDate)
	lastModifiedAt := details.GetInt64(bundle.RelationKeyLastModifiedDate)
	infos := make([]*storage.FileInfo, 0, len(variantsList))
	for i, variantId := range variantsList {
		var meta *types.Struct
		widths := details.GetInt64List(bundle.RelationKeyFileVariantWidths)
		if widths[i] > 0 {
			meta = &types.Struct{
				Fields: map[string]*types.Value{
					"width": pbtypes.Int64(int64(widths[i])),
				},
			}
		}
		info := &storage.FileInfo{
			Name:   details.GetString(bundle.RelationKeyName),
			Size_:  details.GetInt64(bundle.RelationKeySizeInBytes),
			Source: sourceChecksum,
			Media:  details.GetString(bundle.RelationKeyFileMimeType),

			Hash:             variantId,
			Checksum:         details.GetStringList(bundle.RelationKeyFileVariantChecksums)[i],
			Mill:             details.GetStringList(bundle.RelationKeyFileVariantMills)[i],
			Meta:             meta,
			Path:             details.GetStringList(bundle.RelationKeyFileVariantPaths)[i],
			Key:              details.GetStringList(bundle.RelationKeyFileVariantKeys)[i],
			Opts:             details.GetStringList(bundle.RelationKeyFileVariantOptions)[i],
			Added:            addedAt,
			LastModifiedDate: lastModifiedAt,
		}
		infos = append(infos, info)
	}
	return infos
}
