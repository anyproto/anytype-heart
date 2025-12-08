package filemodels

import (
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const OriginalImagePath = "/0/" + schema.LinkImageOriginal + "/"

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

func GetFileInfosFromDetails(details *domain.Details) ([]*storage.FileInfo, error) {
	ext := details.GetString(bundle.RelationKeyFileExt)
	name := details.GetString(bundle.RelationKeyName)
	if ext != "" {
		name = name + "." + ext
	}

	variantsList := details.GetStringList(bundle.RelationKeyFileVariantIds)
	checksumList := details.GetStringList(bundle.RelationKeyFileVariantChecksums)
	millList := details.GetStringList(bundle.RelationKeyFileVariantMills)
	pathList := details.GetStringList(bundle.RelationKeyFileVariantPaths)
	keysList := details.GetStringList(bundle.RelationKeyFileVariantKeys)
	optsList := details.GetStringList(bundle.RelationKeyFileVariantOptions)
	widthList := details.GetInt64List(bundle.RelationKeyFileVariantWidths)
	orignalWidth := details.GetInt64(bundle.RelationKeyWidthInPixels)
	orignalHeight := details.GetInt64(bundle.RelationKeyHeightInPixels)

	if len(variantsList) != len(checksumList) {
		return nil, fmt.Errorf("checksum list mismatch")
	}
	if len(variantsList) != len(millList) {
		return nil, fmt.Errorf("mill list mismatch")
	}
	if len(variantsList) != len(pathList) {
		return nil, fmt.Errorf("path list mismatch")
	}
	if len(variantsList) != len(keysList) {
		return nil, fmt.Errorf("keys list mismatch")
	}
	if len(variantsList) != len(optsList) {
		return nil, fmt.Errorf("opts list mismatch")
	}
	if len(widthList) != len(optsList) {
		return nil, fmt.Errorf("width list mismatch")
	}

	sourceChecksum := details.GetString(bundle.RelationKeyFileSourceChecksum)
	addedAt := details.GetInt64(bundle.RelationKeyAddedDate)
	lastModifiedAt := details.GetInt64(bundle.RelationKeyLastModifiedDate)
	infos := make([]*storage.FileInfo, 0, len(variantsList))

	for i, variantId := range variantsList {
		var meta *types.Struct
		if pathList[i] == OriginalImagePath && orignalWidth > 0 && orignalHeight > 0 {
			meta = &types.Struct{
				Fields: map[string]*types.Value{
					"width":  pbtypes.Int64(orignalWidth),
					"height": pbtypes.Int64(orignalHeight),
				},
			}
		} else if widthList[i] > 0 {
			meta = &types.Struct{
				Fields: map[string]*types.Value{
					"width": pbtypes.Int64(widthList[i]),
				},
			}
		}
		info := &storage.FileInfo{
			Name:   name,
			Size_:  details.GetInt64(bundle.RelationKeySizeInBytes),
			Source: sourceChecksum,
			Media:  details.GetString(bundle.RelationKeyFileMimeType),

			Hash:             variantId,
			Checksum:         checksumList[i],
			Mill:             millList[i],
			Meta:             meta,
			Path:             pathList[i],
			Key:              keysList[i],
			Opts:             optsList[i],
			Added:            addedAt,
			LastModifiedDate: lastModifiedAt,
		}
		infos = append(infos, info)
	}
	return infos, nil
}
