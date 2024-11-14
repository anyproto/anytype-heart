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

func FileInfosToDetails(infos []*storage.FileInfo, st *state.State) error {
	if len(infos) == 0 {
		return fmt.Errorf("empty info list")
	}
	var (
		variantIds []string
		keys       []string // fill in smartblock?
		widths     []int
		checksums  []string
		mills      []string
		options    []string
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
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantIds, pbtypes.StringList(variantIds))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantChecksums, pbtypes.StringList(checksums))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantMills, pbtypes.StringList(mills))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantWidths, pbtypes.IntList(widths...))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantKeys, pbtypes.StringList(keys))
	st.SetDetailAndBundledRelation(bundle.RelationKeyFileVariantOptions, pbtypes.StringList(options))
	return nil
}
