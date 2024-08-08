package template

import (
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	simpleDataview "github.com/anyproto/anytype-heart/core/block/simple/dataview"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

// duplicate constants stored at core/block/editor/state/state.go
// it can't be reused from here as it would create a circular dependency
// after refactoring these templates we need to find a better place for these constants and IsEmpty method
const (
	HeaderLayoutId      = "header"
	TitleBlockId        = "title"
	DescriptionBlockId  = "description"
	DataviewBlockId     = "dataview"
	FeaturedRelationsId = "featuredRelations"
)

var log = logging.Logger("anytype-state-template")

type StateTransformer func(s *state.State)

var WithEmpty = StateTransformer(func(s *state.State) {
	if s.Exists(s.RootId()) {
		return
	}

	s.Add(simple.New(&model.Block{
		Id: s.RootId(),
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
	}))

})

var WithForcedObjectTypes = func(otypes []domain.TypeKey) StateTransformer {
	return func(s *state.State) {
		if slice.SortedEquals(s.ObjectTypeKeys(), otypes) {
			return
		}
		s.SetObjectTypeKeys(otypes)
	}
}

// WithNoObjectTypes is a special case used only for Archive
var WithNoObjectTypes = func() StateTransformer {
	return func(s *state.State) {
		s.SetNoObjectType(true)
	}
}

var WithNoDuplicateLinks = func() StateTransformer {
	return func(s *state.State) {
		var m = make(map[string]struct{})
		var l *model.BlockContentLink
		for _, b := range s.Blocks() {
			if l = b.GetLink(); l == nil {
				continue
			}
			if _, exists := m[l.TargetBlockId]; exists {
				s.Unlink(b.Id)
				continue
			}
			m[l.TargetBlockId] = struct{}{}
		}
	}
}

var WithRelations = func(rels []domain.RelationKey) StateTransformer {
	return func(s *state.State) {
		s.AddBundledRelationLinks(rels...)
	}
}

var WithRequiredRelations = func(s *state.State) {
	WithRelations(bundle.RequiredInternalRelations)(s)
}

var WithObjectTypesAndLayout = func(otypes []domain.TypeKey, layout model.ObjectTypeLayout) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypeKeys()) == 0 {
			s.SetObjectTypeKeys(otypes)
		} else {
			otypes = s.ObjectTypeKeys()
		}

		if !pbtypes.HasField(s.Details(), bundle.RelationKeyLayout.String()) {
			s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Float64(float64(layout)))
		}
	}
}

var WithLayout = func(layout model.ObjectTypeLayout) StateTransformer {
	return WithDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(layout)))
}

var WithDetailName = func(name string) StateTransformer {
	return WithDetail(bundle.RelationKeyName, pbtypes.String(name))
}

var WithDetail = func(key domain.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil {
			s.SetDetailAndBundledRelation(key, value)
		}
	}
}

var WithForcedDetail = func(key domain.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil || !s.Details().Fields[key.String()].Equal(value) {
			s.SetDetailAndBundledRelation(key, value)
		}
	}
}

var WithDetailIconEmoji = func(iconEmoji string) StateTransformer {
	return WithDetail(bundle.RelationKeyIconEmoji, pbtypes.String(iconEmoji))
}

var RequireHeader = StateTransformer(func(s *state.State) {
	WithEmpty(s)
	if s.Exists(HeaderLayoutId) {
		parent := s.PickParentOf(HeaderLayoutId)

		// case when Header is not the first block of the root
		if parent == nil || parent.Model().Id != s.RootId() || slice.FindPos(parent.Model().ChildrenIds, HeaderLayoutId) != 0 {
			s.Unlink(HeaderLayoutId)
			root := s.Get(s.RootId())
			root.Model().ChildrenIds = append([]string{HeaderLayoutId}, root.Model().ChildrenIds...)
		}
		return
	}

	s.Add(simple.New(&model.Block{
		Id: HeaderLayoutId,
		Restrictions: &model.BlockRestrictions{
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Header,
			},
		},
	}))

	// todo: rewrite when we add insert position Block_Inner_Leading
	root := s.Get(s.RootId())
	root.Model().ChildrenIds = append([]string{HeaderLayoutId}, root.Model().ChildrenIds...)
})

var WithTitle = StateTransformer(func(s *state.State) {
	RequireHeader(s)

	var (
		align model.BlockAlign
	)
	if pbtypes.HasField(s.Details(), bundle.RelationKeyLayoutAlign.String()) {
		alignN := int32(pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayoutAlign.String()))
		if alignN >= 0 && alignN <= 2 {
			align = model.BlockAlign(alignN)
		}
	}

	blockExists := s.Exists(TitleBlockId)

	if blockExists {
		isAlignOk := s.Pick(TitleBlockId).Model().Align == align
		isFieldOk := len(pbtypes.GetStringList(s.Pick(TitleBlockId).Model().Fields, text.DetailsKeyFieldName)) == 2
		if isFieldOk && isAlignOk {
			return
		}
	}

	s.Set(simple.New(&model.Block{
		Id: TitleBlockId,
		Restrictions: &model.BlockRestrictions{
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Title}},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.StringList([]string{bundle.RelationKeyName.String(), bundle.RelationKeyDone.String()}),
			},
		},
		Align: align,
	}))

	if parent := s.PickParentOf(TitleBlockId); parent != nil {
		if slice.FindPos(parent.Model().ChildrenIds, TitleBlockId) != 0 {
			s.Unlink(TitleBlockId)
			blockExists = false
		}
	}

	if blockExists {
		return
	}
	if err := s.InsertTo(HeaderLayoutId, model.Block_InnerFirst, TitleBlockId); err != nil {
		log.Errorf("template WithTitle failed to insert: %v", err)
	}
})

// WithDefaultFeaturedRelations **MUST** be called before WithDescription
var WithDefaultFeaturedRelations = func(s *state.State) {
	if !pbtypes.HasField(s.Details(), bundle.RelationKeyFeaturedRelations.String()) {
		var fr = []string{bundle.RelationKeyType.String()}
		layout, _ := s.Layout()
		switch layout {
		case model.ObjectType_basic, model.ObjectType_note:
			fr = []string{bundle.RelationKeyType.String(), bundle.RelationKeyBacklinks.String()}
		case model.ObjectType_set:
			fr = []string{bundle.RelationKeyType.String(), bundle.RelationKeySetOf.String(), bundle.RelationKeyBacklinks.String()}
		case model.ObjectType_collection:
			fr = []string{bundle.RelationKeyType.String(), bundle.RelationKeyBacklinks.String()}
		}
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(fr))
	}
}

var WithAddedFeaturedRelation = func(key domain.RelationKey) StateTransformer {
	return func(s *state.State) {
		var featRels = pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
		if slice.FindPos(featRels, key.String()) > -1 {
			return
		} else {
			s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(append(featRels, key.String())))
		}
	}
}

var WithRemovedFeaturedRelation = func(key domain.RelationKey) StateTransformer {
	return func(s *state.State) {
		var featRels = pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
		if slice.FindPos(featRels, key.String()) > -1 {
			s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(slice.RemoveMut(featRels, key.String())))
			return
		}
	}
}

var WithCreatorRemovedFromFeaturedRelations = StateTransformer(func(s *state.State) {
	fr := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())

	if slice.FindPos(fr, bundle.RelationKeyCreator.String()) != -1 {
		frc := make([]string, len(fr))
		copy(frc, fr)

		frc = slice.RemoveMut(frc, bundle.RelationKeyCreator.String())
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(frc))
	}
})

var WithForcedDescription = func(s *state.State) {
	RequireHeader(s)

	var align model.BlockAlign
	if pbtypes.HasField(s.Details(), bundle.RelationKeyLayoutAlign.String()) {
		alignN := int(pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayoutAlign.String()))
		if alignN >= 0 && alignN <= 2 {
			align = model.BlockAlign(alignN)
		}
	}

	header := s.Get(HeaderLayoutId)
	blockExists := s.Exists(DescriptionBlockId) && slices.Contains(header.Model().ChildrenIds, DescriptionBlockId)
	if blockExists && (s.Get(DescriptionBlockId).Model().Align == align) {
		return
	}

	s.Set(simple.New(&model.Block{
		Id: DescriptionBlockId,
		Restrictions: &model.BlockRestrictions{
			Remove: true,
			Drag:   true,
			DropOn: true,
		},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Description}},
		Fields: &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.String("description"),
			},
		},
		Align: align,
	}))

	if blockExists {
		return
	}

	if err := s.InsertTo(TitleBlockId, model.Block_Bottom, DescriptionBlockId); err != nil {
		if err = s.InsertTo(FeaturedRelationsId, model.Block_Top, DescriptionBlockId); err != nil {
			if err = s.InsertTo(HeaderLayoutId, model.Block_Inner, DescriptionBlockId); err != nil {
				log.Errorf("template WithDescription failed to insert: %s", err)
			}
		}
	}
}

var WithDescription = func(s *state.State) {
	RequireHeader(s)

	WithAddedFeaturedRelation(bundle.RelationKeyDescription)(s)
	if !s.Exists(DescriptionBlockId) {
		WithForcedDescription(s)
	}
}

var WithNoTitle = StateTransformer(func(s *state.State) {
	s.Unlink(TitleBlockId)
})

var WithFirstTextBlock = WithFirstTextBlockContent("")

var WithFirstTextBlockContent = func(text string) StateTransformer {
	return func(s *state.State) {
		WithEmpty(s)
		root := s.Pick(s.RootId())
		if root != nil {
			for i, chId := range root.Model().ChildrenIds {
				if child := s.Pick(chId); child != nil {
					if exText := child.Model().GetText(); exText != nil {
						if text != "" && i == len(root.Model().ChildrenIds)-1 && exText.Text != text {
							s.Get(chId).Model().GetText().Text = text
						}
						return
					}
				}
			}
			tb := simple.New(&model.Block{Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Marks: &model.BlockContentTextMarks{}, Text: text},
			}})
			s.Add(tb)
			s.InsertTo("", 0, tb.Model().Id)
		}
	}
}

var WithNoDescription = StateTransformer(func(s *state.State) {
	WithRemovedFeaturedRelation(bundle.RelationKeyDescription)(s)
	s.Unlink(DescriptionBlockId)
})

var WithNameToFirstBlock = StateTransformer(func(s *state.State) {
	RequireHeader(s)

	name, ok := s.Details().Fields[bundle.RelationKeyName.String()]
	if ok && name.GetStringValue() != "" {
		newBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: name.GetStringValue()},
			},
		})
		s.Add(newBlock)

		if err := s.InsertTo(HeaderLayoutId, model.Block_Bottom, newBlock.Model().Id); err != nil {
			log.Errorf("WithNameToFirstBlock failed to insert: %s", err)
		} else {
			s.RemoveDetail(bundle.RelationKeyName.String())
		}
	}
})

var WithFeaturedRelations = StateTransformer(func(s *state.State) {
	RequireHeader(s)

	var align model.BlockAlign
	if pbtypes.HasField(s.Details(), bundle.RelationKeyLayoutAlign.String()) {
		alignN := int(pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayoutAlign.String()))
		if alignN >= 0 && alignN <= 2 {
			align = model.BlockAlign(alignN)
		}
	}

	blockExists := s.Exists(FeaturedRelationsId)
	if blockExists && (s.Get(FeaturedRelationsId).Model().Align == align) {
		return
	}

	s.Set(simple.New(&model.Block{
		Id: FeaturedRelationsId,
		Restrictions: &model.BlockRestrictions{
			Remove: true,
			Drag:   true,
			DropOn: true,
			Edit:   false,
		},
		Content: &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}},
		Align:   align,
	}))

	if blockExists {
		return
	}

	if err := s.InsertTo(HeaderLayoutId, model.Block_Inner, FeaturedRelationsId); err != nil {
		log.Errorf("template FeaturedRelations failed to insert: %v", err)
	}
})

var WithAllBlocksEditsRestricted = StateTransformer(func(s *state.State) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		b.Model().Restrictions = &model.BlockRestrictions{
			Read:   false,
			Edit:   true,
			Remove: true,
			Drag:   true,
			DropOn: true,
		}
		return true
	})
})

var WithDataviewID = func(id string, dataview *model.BlockContentOfDataview, forceViews bool) StateTransformer {
	return func(s *state.State) {
		WithEmpty(s)
		// remove old dataview
		var blockNeedToUpdate bool
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if dvBlock, ok := b.(simpleDataview.Block); !ok {
				return true
			} else {
				if len(dvBlock.Model().GetDataview().Relations) == 0 ||
					!slice.UnsortedEqual(dvBlock.Model().GetDataview().Source, dataview.Dataview.Source) ||
					len(dvBlock.Model().GetDataview().Views) == 0 ||
					forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations) ||
					forceViews && !pbtypes.DataviewViewsEqualSorted(dvBlock.Model().GetDataview().Views, dataview.Dataview.Views) {

					/* log.With("object" s.RootId()).With("name", pbtypes.GetString(s.Details(), "name")).Warnf("dataview needs to be migrated: %v, %v, %v, %v",
					len(dvBlock.Model().GetDataview().Relations) == 0,
					!slice.UnsortedEqual(dvBlock.Model().GetDataview().Source, dataview.Dataview.Source),
					len(dvBlock.Model().GetDataview().Views) == 0,
					forceViews && len(dvBlock.Model().GetDataview().Views[0].Filters) != len(dataview.Dataview.Views[0].Filters) ||
						forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations)) */
					blockNeedToUpdate = true
					return false
				}
			}
			return true
		})

		if blockNeedToUpdate || !s.Exists(id) {
			s.Set(simple.New(&model.Block{Content: dataview, Id: id}))
			if !s.IsParentOf(s.RootId(), id) {
				err := s.InsertTo(s.RootId(), model.Block_Inner, id)
				if err != nil {
					log.Errorf("template WithDataview failed to insert: %v", err)
				}
			}
		}

	}
}

var WithDataview = func(dataview *model.BlockContentOfDataview, forceViews bool) StateTransformer {
	return WithDataviewID(DataviewBlockId, dataview, forceViews)
}

var WithNoRootLink = func(targetBlockId string) StateTransformer {
	return func(s *state.State) {
		var linkBlockId string
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if b, ok := b.(*link.Link); !ok {
				return true
			} else {
				if b.Model().GetLink().TargetBlockId == targetBlockId {
					linkBlockId = b.Id
					return false
				}

				return true
			}
		})

		if linkBlockId == "" {
			return
		}

		s.Unlink(linkBlockId)

		return
	}
}

func WithBlockField(blockId, fieldName string, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if b := s.Get(blockId); b != nil {
			fields := b.Model().Fields
			if fields == nil || fields.Fields == nil {
				fields = &types.Struct{Fields: map[string]*types.Value{}}
			}
			fields.Fields[fieldName] = value
			b.Model().Fields = fields
		}
	}
}

func InitTemplate(s *state.State, templates ...StateTransformer) {
	for _, template := range templates {
		template(s)
	}
}

var WithLinkFieldsMigration = func(s *state.State) {
	const linkMigratedKey = "_link_migrated"
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := b.(*link.Link); !ok {
			return true
		} else {
			if b.Model().GetFields().GetFields() != nil && !pbtypes.GetBool(b.Model().GetFields(), linkMigratedKey) {

				b = s.Get(b.Model().Id)
				link := b.(*link.Link).GetLink()

				if cardStyle, ok := b.Model().GetFields().Fields["style"]; ok {
					link.CardStyle = model.BlockContentLinkCardStyle(cardStyle.GetNumberValue())
				}

				if iconSize, ok := b.Model().GetFields().Fields["iconSize"]; ok {
					if int(iconSize.GetNumberValue()) == 1 {
						link.IconSize = model.BlockContentLink_SizeSmall
					} else if int(iconSize.GetNumberValue()) == 2 {
						link.IconSize = model.BlockContentLink_SizeMedium
					}
				}

				if description, ok := b.Model().GetFields().Fields["description"]; ok {
					link.Description = model.BlockContentLinkDescription(description.GetNumberValue())
				}

				featuredRelations := map[string]string{"withCover": "cover", "withName": "name", "withType": "type"}
				for key, relName := range featuredRelations {
					if rel, ok := b.Model().GetFields().Fields[key]; ok {
						if rel.GetBoolValue() {
							link.Relations = append(link.Relations, relName)
						}
					}
				}

				b.Model().Fields.Fields[linkMigratedKey] = pbtypes.Bool(true)
			}

			return true
		}
	})

	return
}

var bookmarkRelationKeys = []string{
	bundle.RelationKeySource.String(),
	bundle.RelationKeyTag.String(),
}

var oldBookmarkRelationBlocks = []string{
	bundle.RelationKeyUrl.String(),
	bundle.RelationKeyPicture.String(),
	bundle.RelationKeyCreatedDate.String(),
}

var oldBookmarkRelations = []string{
	bundle.RelationKeyUrl.String(),
}

func makeRelationBlock(k string) *model.Block {
	return &model.Block{
		Id: k,
		Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{
				Key: k,
			},
		},
	}
}

var WithBookmarkBlocks = func(s *state.State) {
	if !s.HasRelation(bundle.RelationKeySource.String()) && s.HasRelation(bundle.RelationKeyUrl.String()) {
		s.SetDetailAndBundledRelation(bundle.RelationKeySource, s.Details().Fields[bundle.RelationKeyUrl.String()])
	}

	for _, oldRel := range oldBookmarkRelationBlocks {
		s.Unlink(oldRel)
	}

	for _, oldRel := range oldBookmarkRelations {
		s.RemoveRelation(oldRel)
	}

	fr := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())

	if slice.FindPos(fr, bundle.RelationKeyCreatedDate.String()) == -1 {
		fr = append(fr, bundle.RelationKeyCreatedDate.String())
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(fr))
	}

	for _, k := range bookmarkRelationKeys {
		if !s.HasRelation(k) {
			s.AddBundledRelationLinks(domain.RelationKey(k))
		}
	}

	for _, rk := range bookmarkRelationKeys {
		if b := s.Pick(rk); b != nil {
			if ok := s.Unlink(b.Model().Id); !ok {
				log.Errorf("can't unlink block %s", b.Model().Id)
				return
			}
			continue
		}

		ok := s.Add(simple.New(makeRelationBlock(rk)))
		if !ok {
			log.Errorf("can't add block %s", rk)
			return
		}
	}

	if err := s.InsertTo(s.RootId(), model.Block_InnerFirst, bookmarkRelationKeys...); err != nil {
		log.Errorf("insert relation blocks: %v", err)
		return
	}
}
