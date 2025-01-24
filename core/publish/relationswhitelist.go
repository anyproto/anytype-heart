package publish

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var allObjectsRelationsWhiteList = []string{
	bundle.RelationKeyType.String(),
	bundle.RelationKeySpaceId.String(),
	bundle.RelationKeyId.String(),
	bundle.RelationKeyLayout.String(),
	bundle.RelationKeyIsArchived.String(),
	bundle.RelationKeyIsDeleted.String(),
	bundle.RelationKeyName.String(),
}

var documentRelationsWhiteList = append(allObjectsRelationsWhiteList,
	bundle.RelationKeyDescription.String(),
	bundle.RelationKeySnippet.String(),
	bundle.RelationKeyIconImage.String(),
	bundle.RelationKeyIconEmoji.String(),
	bundle.RelationKeyCoverType.String(),
	bundle.RelationKeyCoverId.String(),
)

var todoRelationsWhiteList = append(documentRelationsWhiteList, bundle.RelationKeyDone.String())

var bookmarkRelationsWhiteList = append(documentRelationsWhiteList, bundle.RelationKeyPicture.String())

var derivedObjectsWhiteList = append(allObjectsRelationsWhiteList, bundle.RelationKeyUniqueKey.String())

var relationsWhiteList = append(derivedObjectsWhiteList, bundle.RelationKeyRelationFormat.String())

var relationOptionWhiteList = append(derivedObjectsWhiteList, bundle.RelationKeyRelationOptionColor.String())

var publishingRelationsWhiteList = map[model.ObjectTypeLayout][]string{
	model.ObjectType_basic:      documentRelationsWhiteList,
	model.ObjectType_profile:    documentRelationsWhiteList,
	model.ObjectType_todo:       todoRelationsWhiteList,
	model.ObjectType_set:        documentRelationsWhiteList,
	model.ObjectType_collection: documentRelationsWhiteList,
	model.ObjectType_objectType: derivedObjectsWhiteList,
	model.ObjectType_relation:   relationsWhiteList,
	model.ObjectType_file:       documentRelationsWhiteList,
	model.ObjectType_dashboard:  allObjectsRelationsWhiteList,
	model.ObjectType_image:      documentRelationsWhiteList,
	model.ObjectType_note:       documentRelationsWhiteList,
	model.ObjectType_space:      allObjectsRelationsWhiteList,

	model.ObjectType_bookmark:            bookmarkRelationsWhiteList,
	model.ObjectType_relationOption:      relationOptionWhiteList,
	model.ObjectType_relationOptionsList: relationOptionWhiteList,
	model.ObjectType_participant:         documentRelationsWhiteList,
	model.ObjectType_chat:                allObjectsRelationsWhiteList,
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
