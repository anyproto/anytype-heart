package smartblock

import (
	"slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectlink"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/slice"
)

var relationsToSkipLinksIndexing = []domain.RelationKey{
	bundle.RelationKeyId,
	bundle.RelationKeyLinks,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyMentions,
	bundle.RelationKeyCreator,
	bundle.RelationKeyLastModifiedBy,
	bundle.RelationKeyType,
	bundle.RelationKeyFileId,
	bundle.RelationKeyFeaturedRelations,
	bundle.RelationKeyCreatedInContext,
	bundle.RelationKeySourceObject,
	bundle.RelationKeyChatId,
}

// relationsToFilterOutForLinks contains relations that we index but filter-out when injecting "links" details
var relationsToFilterOutForLinks = []domain.RelationKey{
	bundle.RelationKeyIconImage,
	bundle.RelationKeyPicture,
	bundle.RelationKeyFileId,
	bundle.RelationKeyCoverId,
}

func (sb *smartBlock) updateBackLinks(s *state.State) {
	backLinks, err := sb.spaceIndex.GetInboundLinksById(sb.Id())
	if err != nil {
		log.With("objectID", sb.Id()).Errorf("failed to get inbound links from object store: %s", err)
		return
	}
	s.SetDetailAndBundledRelation(bundle.RelationKeyBacklinks, domain.StringList(backLinks))
}

func (sb *smartBlock) injectLinksDetails(s *state.State) {
	links := objectlink.DependentObjectIDs(s, sb.Space(), sb.formatFetcher, objectlink.Flags{
		Blocks:                   true,
		Details:                  true,
		Relations:                sb.includeRelationObjectsAsDependents,
		Types:                    false,
		Collection:               !internalflag.NewFromState(s).Has(model.InternalFlag_collectionDontIndexLinks),
		DataviewBlockOnlyTarget:  true,
		NoSystemRelations:        true,
		NoHiddenBundledRelations: true,
		NoImages:                 false,
		RoundDateIdsToDay:        true,
	})
	links = slice.Filter(links, func(link string) bool {
		if link == sb.Id() {
			return false
		}

		for _, rel := range relationsToFilterOutForLinks {
			if !s.Details().Has(rel) {
				continue
			}
			val := s.Details().Get(rel)
			if val.IsString() {
				if val.String() != link {
					return false
				}
				continue
			}
			if val.IsStringList() && slices.Contains(val.StringList(), link) {
				return false
			}
		}
		return true
	})
	s.SetLocalDetail(bundle.RelationKeyLinks, domain.StringList(links))
}

func (sb *smartBlock) injectMentions(s *state.State) {
	mentions := objectlink.DependentObjectIDs(s, sb.Space(), sb.formatFetcher, objectlink.Flags{
		Blocks:                   true,
		Details:                  false,
		Relations:                false,
		Types:                    false,
		Collection:               false,
		DataviewBlockOnlyTarget:  true,
		NoSystemRelations:        true,
		NoHiddenBundledRelations: true,
		NoImages:                 true,
	})
	mentions = slice.RemoveMut(mentions, sb.Id())
	s.SetDetailAndBundledRelation(bundle.RelationKeyMentions, domain.StringList(mentions))
}

func isBacklinksChanged(msgs []simple.EventMessage) bool {
	for _, msg := range msgs {
		if amend, ok := msg.Msg.Value.(*pb.EventMessageValueOfObjectDetailsAmend); ok {
			for _, detail := range amend.ObjectDetailsAmend.Details {
				if detail.Key == bundle.RelationKeyBacklinks.String() {
					return true
				}
			}
		}
	}
	return false
}
