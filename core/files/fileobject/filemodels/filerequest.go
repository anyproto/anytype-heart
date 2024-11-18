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
	FileId                domain.FileId
	EncryptionKeys        map[string]string
	ObjectOrigin          objectorigin.ObjectOrigin
	ImageKind             model.ImageKind
	AdditionalDetails     *types.Struct
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

	st.SetDetailAndBundledRelation(bundle.RelationKeyFileSourceChecksum, pbtypes.String(infos[0].Source))
	for _, info := range infos {
		variantIds = append(variantIds, info.Hash)
		checksums = append(checksums, info.Checksum)
		mills = append(mills, info.Mill)
		widths = append(widths, int(pbtypes.GetInt64(info.Meta, "width")))
		keys = append(keys, keysInfo[info.Path])
		options = append(options, info.Opts)
		paths = append(paths, info.Path)
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantIds, pbtypes.StringList(variantIds))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantPaths, pbtypes.StringList(paths))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantChecksums, pbtypes.StringList(checksums))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantMills, pbtypes.StringList(mills))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantWidths, pbtypes.IntList(widths...))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantKeys, pbtypes.StringList(keys))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantOptions, pbtypes.StringList(options))
	return nil
}

func GetFileInfosFromDetails(details *types.Struct) []*storage.FileInfo {
	variantsList := pbtypes.GetStringList(details, bundle.RelationKeyFileVariantIds.String())
	sourceChecksum := pbtypes.GetString(details, bundle.RelationKeyFileSourceChecksum.String())
	infos := make([]*storage.FileInfo, 0, len(variantsList))
	for i, variantId := range variantsList {
		var meta *types.Struct
		widths := pbtypes.GetIntList(details, bundle.RelationKeyFileVariantWidths.String())
		if widths[i] > 0 {
			meta = &types.Struct{
				Fields: map[string]*types.Value{
					"width": pbtypes.Int64(int64(widths[i])),
				},
			}
		}
		info := &storage.FileInfo{
			Name:   pbtypes.GetString(details, bundle.RelationKeyName.String()),
			Size_:  pbtypes.GetInt64(details, bundle.RelationKeySizeInBytes.String()),
			Source: sourceChecksum,
			Media:  pbtypes.GetString(details, bundle.RelationKeyFileMimeType.String()),

			Hash:     variantId,
			Checksum: pbtypes.GetStringList(details, bundle.RelationKeyFileVariantChecksums.String())[i],
			Mill:     pbtypes.GetStringList(details, bundle.RelationKeyFileVariantMills.String())[i],
			Meta:     meta,
			Path:     pbtypes.GetStringList(details, bundle.RelationKeyFileVariantPaths.String())[i],
			Key:      pbtypes.GetStringList(details, bundle.RelationKeyFileVariantKeys.String())[i],
			Opts:     pbtypes.GetStringList(details, bundle.RelationKeyFileVariantOptions.String())[i],
		}
		infos = append(infos, info)
	}
	return infos
}
