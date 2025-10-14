package publish

import (
	"slices"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var allObjectsRelationsWhiteList = []string{
	bundle.RelationKeyType.String(),
	bundle.RelationKeySpaceId.String(),
	bundle.RelationKeyId.String(),
	bundle.RelationKeyLayout.String(),
	bundle.RelationKeyResolvedLayout.String(),
	bundle.RelationKeyIsArchived.String(),
	bundle.RelationKeyIsDeleted.String(),
	bundle.RelationKeyName.String(),
}

var documentRelationsWhiteList = append(slices.Clone(allObjectsRelationsWhiteList),
	bundle.RelationKeyDescription.String(),
	bundle.RelationKeySnippet.String(),
	bundle.RelationKeyIconImage.String(),
	bundle.RelationKeyIconEmoji.String(),
	bundle.RelationKeyCoverType.String(),
	bundle.RelationKeyCoverId.String(),
	bundle.RelationKeyCoverX.String(),
	bundle.RelationKeyCoverY.String(),
	bundle.RelationKeyCoverScale.String(),
)

var todoRelationsWhiteList = append(slices.Clone(documentRelationsWhiteList), bundle.RelationKeyDone.String())

var bookmarkRelationsWhiteList = append(slices.Clone(documentRelationsWhiteList), bundle.RelationKeyPicture.String(), bundle.RelationKeySource.String())

var derivedObjectsWhiteList = append(slices.Clone(allObjectsRelationsWhiteList), bundle.RelationKeyUniqueKey.String())

var relationsWhiteList = append(slices.Clone(derivedObjectsWhiteList), bundle.RelationKeyRelationFormat.String())

var relationOptionWhiteList = append(slices.Clone(derivedObjectsWhiteList), bundle.RelationKeyRelationOptionColor.String())
var objectTypeWhiteList = append(slices.Clone(derivedObjectsWhiteList),
	bundle.RelationKeyRecommendedRelations.String(),
	bundle.RelationKeyRecommendedFeaturedRelations.String(),
	bundle.RelationKeyRecommendedLayout.String(),
	bundle.RelationKeyLayoutWidth.String(),
	bundle.RelationKeyLayoutAlign.String(),
	bundle.RelationKeyIconName.String(),
	bundle.RelationKeyIconOption.String(),
	bundle.RelationKeyIconImage.String(),
)

var fileRelationsWhiteList = append(
	slices.Clone(documentRelationsWhiteList),
	bundle.RelationKeyFileId.String(),
	bundle.RelationKeyFileExt.String(),
	bundle.RelationKeyFileMimeType.String(),
	bundle.RelationKeySizeInBytes.String(),
	bundle.RelationKeySource.String(),
)

var imageRelationsWhiteList = append(slices.Clone(fileRelationsWhiteList),
	bundle.RelationKeyMediaArtistName.String(),
	bundle.RelationKeyMediaArtistURL.String(),
)

var spacedWhiteList = append(slices.Clone(documentRelationsWhiteList), bundle.RelationKeyIconImage.String(), bundle.RelationKeyIconOption.String())

var publishingRelationsWhiteList = map[model.ObjectTypeLayout][]string{
	model.ObjectType_basic:      documentRelationsWhiteList,
	model.ObjectType_profile:    documentRelationsWhiteList,
	model.ObjectType_todo:       todoRelationsWhiteList,
	model.ObjectType_set:        documentRelationsWhiteList,
	model.ObjectType_collection: documentRelationsWhiteList,
	model.ObjectType_objectType: objectTypeWhiteList,
	model.ObjectType_relation:   relationsWhiteList,
	model.ObjectType_file:       fileRelationsWhiteList,
	model.ObjectType_pdf:        fileRelationsWhiteList,
	model.ObjectType_dashboard:  allObjectsRelationsWhiteList,
	model.ObjectType_image:      imageRelationsWhiteList,
	model.ObjectType_note:       documentRelationsWhiteList,
	model.ObjectType_space:      spacedWhiteList,

	model.ObjectType_bookmark:            bookmarkRelationsWhiteList,
	model.ObjectType_relationOption:      relationOptionWhiteList,
	model.ObjectType_relationOptionsList: relationOptionWhiteList,
	model.ObjectType_participant:         documentRelationsWhiteList,
	model.ObjectType_chatDerived:         allObjectsRelationsWhiteList,
	model.ObjectType_tag:                 documentRelationsWhiteList,
}

func relationsWhiteListToPbModel() []*pb.RpcObjectListExportRelationsWhiteList {
	pbRelationsWhiteList := make([]*pb.RpcObjectListExportRelationsWhiteList, 0, len(publishingRelationsWhiteList))
	for layout, relation := range publishingRelationsWhiteList {
		pbRelationsWhiteList = append(pbRelationsWhiteList, &pb.RpcObjectListExportRelationsWhiteList{
			Layout:           layout,
			AllowedRelations: relation,
		})
	}
	return pbRelationsWhiteList
}
