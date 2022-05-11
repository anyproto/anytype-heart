package template

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

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

var WithObjectTypes = func(otypes []string) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypes()) == 0 {
			s.SetObjectTypes(otypes)
		}
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

var WithObjectTypeLayoutMigration = func() StateTransformer {
	return func(s *state.State) {
		layout := pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayout.String())

		if layout == float64(model.ObjectType_objectType) {
			return
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedLayout, pbtypes.Float64(layout))
		s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Float64(float64(model.ObjectType_objectType)))
	}
}

var WithObjectTypeRecommendedRelationsMigration = func(relations []*model.Relation) StateTransformer {
	return func(s *state.State) {
		var relIds []string
		ot := bundle.MustGetType(bundle.TypeKeyObjectType)
		rels := ot.GetRelations()

		var objectTypeOnlyRelations []*model.Relation
		for _, rel := range rels {
			var found bool
			for _, requiredRel := range bundle.RequiredInternalRelations {
				if rel.Key == requiredRel.String() {
					found = true
					break
				}
			}
			if !found {
				objectTypeOnlyRelations = append(objectTypeOnlyRelations, pbtypes.CopyRelation(rel))
			}
		}

		for _, rel := range append(relations, s.ExtraRelations()...) {
			// so the idea is that we need to add all relations EXCEPT that only exists in the objectType
			// e.g. we don't need to recommendedRelation and recommendedLayout
			if pbtypes.HasRelation(objectTypeOnlyRelations, rel.Key) {
				continue
			}
			var relId string
			if bundle.HasRelation(rel.Key) {
				relId = addr.BundledRelationURLPrefix + rel.Key
			} else {
				relId = addr.CustomRelationURLPrefix + rel.Key
			}
			if slice.FindPos(relIds, relId) > -1 {
				continue
			}

			relIds = append(relIds, relId)
			var found bool
			for _, exRel := range s.ExtraRelations() {
				if exRel.Key == rel.Key {
					found = true
					break
				}
			}
			if !found {
				s.AddRelation(rel)
			}
		}

		s.SetDetailAndBundledRelation(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(relIds))
	}
}

var WithRelations = func(rels []bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		for _, relKey := range rels {
			if s.HasRelation(relKey.String()) {
				continue
			}
			rel := bundle.MustGetRelation(relKey)
			s.AddRelation(rel)
		}
	}
}

var WithRequiredRelations = func() StateTransformer {
	return WithRelations(bundle.RequiredInternalRelations)
}

var WithMaxCountMigration = func(s *state.State) {
	d := s.Details()
	if d == nil || d.Fields == nil {
		return
	}

	rels := s.ExtraRelations()
	for k, v := range d.Fields {
		rel := pbtypes.GetRelation(rels, k)
		if rel == nil {
			log.Errorf("obj %s relation %s is missing but detail is set", s.RootId(), k)
		} else if rel.MaxCount == 1 {
			if b := v.GetListValue(); b != nil {
				if len(b.Values) > 0 {
					d.Fields[k] = pbtypes.CopyVal(b.Values[0])
				}
			}
		}
	}
}

var WithObjectTypesAndLayout = func(otypes []string) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypes()) == 0 {
			s.SetObjectTypes(otypes)
		} else {
			otypes = s.ObjectTypes()
		}

		d := s.Details()
		if d == nil || d.Fields == nil || d.Fields[bundle.RelationKeyLayout.String()] == nil {
			for _, ot := range otypes {
				t, err := bundle.GetTypeByUrl(ot)
				if err != nil {
					continue
				}
				s.SetDetailAndBundledRelation(bundle.RelationKeyLayout, pbtypes.Float64(float64(t.Layout)))
			}
		}
	}
}

var WithLayout = func(layout model.ObjectTypeLayout) StateTransformer {
	return WithDetail(bundle.RelationKeyLayout, pbtypes.Float64(float64(layout)))
}

var WithDetailName = func(name string) StateTransformer {
	return WithDetail(bundle.RelationKeyName, pbtypes.String(name))
}

var WithCondition = func(condition bool, f StateTransformer) StateTransformer {
	if condition {
		return f
	} else {
		return func(s *state.State) {}
	}
}

var WithDetail = func(key bundle.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil {
			s.SetDetail(key.String(), value)
		}

		if rel := pbtypes.GetRelation(s.ExtraRelations(), key.String()); rel == nil {
			s.SetExtraRelation(bundle.MustGetRelation(key))
		}
	}
}

var WithForcedDetail = func(key bundle.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil || !s.Details().Fields[key.String()].Equal(value) {
			s.SetDetail(key.String(), value)
		}

		if rel := pbtypes.GetRelation(s.ExtraRelations(), key.String()); rel == nil {
			s.SetExtraRelation(bundle.MustGetRelation(key))
		}
	}
}

var WithDetailIconEmoji = func(iconEmoji string) StateTransformer {
	return WithDetail(bundle.RelationKeyIconEmoji, pbtypes.String(iconEmoji))
}

var WithHeader = StateTransformer(func(s *state.State) {
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
	WithHeader(s)

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
		log.Errorf("template WithTitle failed to insert: %w", err)
	}
})

// WithDefaultFeaturedRelations **MUST** be called before WithDescription
var WithDefaultFeaturedRelations = StateTransformer(func(s *state.State) {
	if !pbtypes.HasField(s.Details(), bundle.RelationKeyFeaturedRelations.String()) {
		var fr = []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()}
		layout, _ := s.Layout()
		if layout == model.ObjectType_basic || layout == model.ObjectType_note {
			fr = []string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()}
		}
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(fr))
	}
})

var WithDescription = StateTransformer(func(s *state.State) {
	WithHeader(s)

	var align model.BlockAlign
	if pbtypes.HasField(s.Details(), bundle.RelationKeyLayoutAlign.String()) {
		alignN := int(pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayoutAlign.String()))
		if alignN >= 0 && alignN <= 2 {
			align = model.BlockAlign(alignN)
		}
	}

	blockExists := s.Exists(DescriptionBlockId)
	blockShouldExists := slice.FindPos(pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String()), DescriptionBlockId) > -1
	if !blockShouldExists {
		if blockExists {
			s.Unlink(DescriptionBlockId)
		}
		return
	}

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
				log.Errorf("template WithDescription failed to insert: %s", err.Error())
			}
		}
	}
})

var WithNoTitle = StateTransformer(func(s *state.State) {
	WithFirstTextBlock(s)
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
	s.Unlink(DescriptionBlockId)
})

var WithFeaturedRelations = StateTransformer(func(s *state.State) {
	WithHeader(s)

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
		log.Errorf("template FeaturedRelations failed to insert: %w", err)
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

var WithBlockEditRestricted = func(id string) StateTransformer {
	return StateTransformer(func(s *state.State) {
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if b.Model().Id != id {
				return true
			}
			b.Model().Restrictions = &model.BlockRestrictions{
				Read:   false,
				Edit:   true,
				Remove: true,
				Drag:   true,
				DropOn: true,
			}
			return false
		})
	})
}

var WithRootBlocks = func(blocks []*model.Block) StateTransformer {
	return func(s *state.State) {
		WithEmpty(s)

		for _, block := range blocks {
			if block.Id == "" {
				panic("WithRootBlocks arg must contains exact ids for blocks")
			}
			s.Add(simple.New(block))
			err := s.InsertTo(s.RootId(), model.Block_Inner, block.Id)
			if err != nil {
				log.Errorf("template WithDataview failed to insert: %w", err)
			}
		}
	}
}

var WithDataviewRequiredRelation = func(id string, key bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		rel := bundle.MustGetRelation(key)
		b := s.Get(id)
		if b == nil {
			return
		}
		var blockNeedToUpdate bool
		if dvBlock, ok := b.(simpleDataview.Block); !ok {
			log.Errorf("WithDataviewRequiredRelation got not dataview block")
			return
		} else {
			dv := dvBlock.Model().GetDataview()
			if dv == nil {
				return
			}
			if exRel := pbtypes.GetRelation(dv.Relations, key.String()); exRel == nil {
				dv.Relations = append(dv.Relations, rel)
				blockNeedToUpdate = true
			}
			for i, view := range dv.Views {
				if view.Relations == nil {
					continue
				}
				var found bool
				for _, rel := range view.Relations {
					if rel.Key == bundle.RelationKeyDone.String() {
						found = true
						break
					}
				}
				if !found {
					blockNeedToUpdate = true
					dv.Views[i].Relations = append(dv.Views[i].Relations, &model.BlockContentDataviewRelation{Key: key.String(), IsVisible: false})
				}
			}
			if blockNeedToUpdate {
				log.Errorf("add missing done relation for set")
				s.Set(simple.New(&model.Block{Content: &model.BlockContentOfDataview{Dataview: dv}, Id: id}))
			}
		}
	}
}

var WithDataviewID = func(id string, dataview model.BlockContentOfDataview, forceViews bool) StateTransformer {
	return func(s *state.State) {
		// remove old dataview
		var blockNeedToUpdate bool
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if dvBlock, ok := b.(simpleDataview.Block); !ok {
				return true
			} else {
				if len(dvBlock.Model().GetDataview().Relations) == 0 ||
					!slice.UnsortedEquals(dvBlock.Model().GetDataview().Source, dataview.Dataview.Source) ||
					len(dvBlock.Model().GetDataview().Views) == 0 ||
					forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations) ||
					forceViews && !pbtypes.DataviewViewsEqualSorted(dvBlock.Model().GetDataview().Views, dataview.Dataview.Views) {

					log.With("thread", s.RootId()).With("name", pbtypes.GetString(s.Details(), "name")).Warnf("dataview needs to be migrated: %v, %v, %v, %v",
						len(dvBlock.Model().GetDataview().Relations) == 0,
						!slice.UnsortedEquals(dvBlock.Model().GetDataview().Source, dataview.Dataview.Source),
						len(dvBlock.Model().GetDataview().Views) == 0,
						forceViews && len(dvBlock.Model().GetDataview().Views[0].Filters) != len(dataview.Dataview.Views[0].Filters) ||
							forceViews && len(dvBlock.Model().GetDataview().Relations) != len(dataview.Dataview.Relations))
					blockNeedToUpdate = true
					return false
				}
			}
			return true
		})

		if blockNeedToUpdate || !s.Exists(id) {
			s.Set(simple.New(&model.Block{Content: &dataview, Id: id}))
			if !s.IsParentOf(s.RootId(), id) {
				err := s.InsertTo(s.RootId(), model.Block_Inner, id)
				if err != nil {
					log.Errorf("template WithDataview failed to insert: %w", err)
				}
			}
		}

	}
}

var WithDataview = func(dataview model.BlockContentOfDataview, forceViews bool) StateTransformer {
	return WithDataviewID(DataviewBlockId, dataview, forceViews)
}

var WithChildrenSorter = func(blockId string, sort func(blockIds []string)) StateTransformer {
	return func(s *state.State) {
		b := s.Get(blockId)
		sort(b.Model().ChildrenIds)

		s.Set(b)
		return
	}
}

var WithRootLink = func(targetBlockId string, style model.BlockContentLinkStyle) StateTransformer {
	return func(s *state.State) {
		var exists bool
		s.Iterate(func(b simple.Block) (isContinue bool) {
			if b, ok := b.(*link.Link); !ok {
				return true
			} else {
				if b.Model().GetLink().TargetBlockId == targetBlockId {
					exists = true
					return false
				}

				return true
			}
		})

		if exists {
			return
		}

		linkBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: targetBlockId,
					Style:         style,
				},
			},
		})

		s.Add(linkBlock)
		if err := s.InsertTo(s.RootId(), model.Block_Inner, linkBlock.Model().Id); err != nil {
			log.Errorf("can't insert link in template: %w", err)
		}

		return
	}
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

func InitTemplate(s *state.State, templates ...StateTransformer) (err error) {
	for _, template := range templates {
		template(s)
	}

	return
}

var WithLink = func(s *state.State) {
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := b.(*link.Link); !ok {
			return true
		} else {
			if b.Model().Fields != nil {
				link := s.Get(b.Model().Id).(*link.Link).GetLink()

				if cardStyle, ok := b.Model().GetFields().Fields["style"]; ok {
					link.CardStyle = model.BlockContentLinkCardStyle(cardStyle.GetNumberValue())
				}

				if iconSize, ok := b.Model().GetFields().Fields["iconSize"]; ok {
					if int(iconSize.GetNumberValue()) < 2 {
						link.IconSize = model.BlockContentLink_Small
					} else {
						link.IconSize = model.BlockContentLink_Medium
					}
				}

				if description, ok := b.Model().GetFields().Fields["description"]; ok {
					link.Description = model.BlockContentLinkDescription(description.GetNumberValue())
				}

				featuredRelations := map[string]string{"withCover": "cover", "withIcon": "icon", "withName": "name", "withType": "type"}
				for key, relName := range featuredRelations {
					if rel, ok := b.Model().GetFields().Fields[key]; ok {
						if rel.GetBoolValue() {
							link.Relations = append(link.Relations, relName)
						}
					}
				}

				b.Model().Fields = nil
			}

			return true
		}
	})

	return
}
