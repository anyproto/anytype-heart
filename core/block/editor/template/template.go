package template

import (
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	simpleDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

var WithObjectTypes = func(otypes []string) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypes()) == 0 {
			s.SetObjectTypes(otypes)
		}
	}
}

var WithForcedObjectTypes = func(otypes []string) StateTransformer {
	return func(s *state.State) {
		if slice.SortedEquals(s.ObjectTypes(), otypes) {
			return
		}
		s.SetObjectTypes(otypes)
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

var WithRelations = func(rels []bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		var links []*model.RelationLink
		for _, relKey := range rels {
			if s.HasRelation(relKey.String()) {
				continue
			}
			rel := bundle.MustGetRelation(relKey)
			links = append(links, &model.RelationLink{Format: rel.Format, Key: rel.Key})
		}
		s.AddRelationLinks(links...)
	}
}

var WithRequiredRelations = func() StateTransformer {
	return WithRelations(bundle.RequiredInternalRelations)
}

var WithObjectTypesAndLayout = func(otypes []string, layout model.ObjectTypeLayout) StateTransformer {
	return func(s *state.State) {
		if len(s.ObjectTypes()) == 0 {
			s.SetObjectTypes(otypes)
		} else {
			otypes = s.ObjectTypes()
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
			s.SetDetailAndBundledRelation(key, value)
		}
	}
}

var WithForcedDetail = func(key bundle.RelationKey, value *types.Value) StateTransformer {
	return func(s *state.State) {
		if s.Details() == nil || s.Details().Fields == nil || s.Details().Fields[key.String()] == nil || !s.Details().Fields[key.String()].Equal(value) {
			s.SetDetailAndBundledRelation(key, value)
		}
	}
}

// MigrateRelationValue moves a relation value from the old key to the new key.
// In case new key already exists, it does nothing
// In case old key does not exist, it does nothing
var MigrateRelationValue = func(from bundle.RelationKey, to bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		if s.Details().GetFields() == nil {
			return
		}
		if s.Details().GetFields()[to.String()] == nil {
			if val := s.Details().GetFields()[from.String()]; val != nil {
				s.SetDetailAndBundledRelation(to, val)
				s.RemoveDetail(from.String())
				s.RemoveRelation(from.String())
			}
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
var WithDefaultFeaturedRelations = func(s *state.State) {
	if !pbtypes.HasField(s.Details(), bundle.RelationKeyFeaturedRelations.String()) {
		var fr = []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String()}
		layout, _ := s.Layout()
		switch layout {
		case model.ObjectType_basic, model.ObjectType_note:
			fr = []string{bundle.RelationKeyType.String()}
		case model.ObjectType_set:
			fr = []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeySetOf.String()}
		case model.ObjectType_collection:
			fr = []string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String()}
		}
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(fr))
	}
}

var WithAddedFeaturedRelation = func(key bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		var featRels = pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())
		if slice.FindPos(featRels, key.String()) > -1 {
			return
		} else {
			s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(append(featRels, key.String())))
		}
	}
}

var WithCreatorRemovedFromFeaturedRelations = StateTransformer(func(s *state.State) {
	fr := pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String())

	if slice.FindPos(fr, bundle.RelationKeyCreator.String()) != -1 {
		frc := make([]string, len(fr))
		copy(frc, fr)

		frc = slice.Remove(frc, bundle.RelationKeyCreator.String())
		s.SetDetail(bundle.RelationKeyFeaturedRelations.String(), pbtypes.StringList(frc))
	}
})

var WithForcedDescription = StateTransformer(func(s *state.State) {
	WithHeader(s)

	var align model.BlockAlign
	if pbtypes.HasField(s.Details(), bundle.RelationKeyLayoutAlign.String()) {
		alignN := int(pbtypes.GetFloat64(s.Details(), bundle.RelationKeyLayoutAlign.String()))
		if alignN >= 0 && alignN <= 2 {
			align = model.BlockAlign(alignN)
		}
	}

	blockExists := s.Exists(DescriptionBlockId)
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

var WithDescription = StateTransformer(func(s *state.State) {
	WithHeader(s)

	blockShouldExists := slice.FindPos(pbtypes.GetStringList(s.Details(), bundle.RelationKeyFeaturedRelations.String()), DescriptionBlockId) > -1
	if !blockShouldExists {
		if s.Exists(DescriptionBlockId) {
			s.Unlink(DescriptionBlockId)
		}
		return
	}

	WithForcedDescription(s)
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
var WithDataviewRelationMigrationRelation = func(id string, source string, from bundle.RelationKey, to bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		rel := bundle.MustGetRelation(to)
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
			if len(dv.Source) != 1 || dv.Source[0] != source {
				return
			}
			var alreadyExists bool
			for _, r := range dv.Relations {
				if r.Key == to.String() {
					alreadyExists = true
				}
			}

			if !alreadyExists {
				for i, r := range dv.Relations {
					if r.Key == from.String() {
						blockNeedToUpdate = true
						dv.Relations[i] = rel
						break
					}
				}
			}

			for _, view := range dv.Views {
				if view.Relations == nil {
					continue
				}

				var alreadyExists bool
				for _, r := range view.Relations {
					if r.Key == to.String() {
						alreadyExists = true
					}
				}
				if !alreadyExists {
					for i, er := range view.Relations {
						if er.Key == from.String() {
							blockNeedToUpdate = true
							view.Relations[i] = &model.BlockContentDataviewRelation{
								Key:             rel.Key,
								IsVisible:       true,
								Width:           er.Width,
								DateIncludeTime: er.DateIncludeTime,
								TimeFormat:      er.TimeFormat,
								DateFormat:      er.DateFormat,
							}
							break
						}
					}
				}

				for i, f := range view.Filters {
					if f.RelationKey == from.String() {
						blockNeedToUpdate = true
						view.Filters[i].RelationKey = rel.Key
						break
					}
				}
			}
			if blockNeedToUpdate {
				s.Set(simple.New(&model.Block{Content: &model.BlockContentOfDataview{Dataview: dv}, Id: id}))
			}
		}
	}
}

var WithDataviewAddIDsToFilters = func(id string) StateTransformer {
	return func(s *state.State) {
		b := s.Get(id)
		if b == nil {
			return
		}
		dv := b.Model().GetDataview()
		if dv == nil {
			return
		}

		for _, view := range dv.Views {
			for _, f := range view.Filters {
				if f.Id == "" {
					f.Id = bson.NewObjectId().Hex()
				}
			}
		}
	}
}

var WithDataviewAddIDsToSorts = func(id string) StateTransformer {
	return func(s *state.State) {
		b := s.Get(id)
		if b == nil {
			return
		}
		dv := b.Model().GetDataview()
		if dv == nil {
			return
		}

		for _, view := range dv.Views {
			for _, s := range view.Sorts {
				if s.Id == "" {
					s.Id = bson.NewObjectId().Hex()
				}
			}
		}
	}
}

var WithDataviewRequiredRelation = func(id string, key bundle.RelationKey) StateTransformer {
	return func(s *state.State) {
		found := false
		for _, r := range bundle.SystemRelations {
			if r.String() == key.String() {
				found = true
				break
			}
		}
		rel := bundle.MustGetRelation(key)
		if rel == nil {
			return
		}
		if !found {
			log.Errorf("WithDataviewRequiredRelation got not system relation %s; ignore", key)
			return
		}
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
			if slice.FindPos(pbtypes.GetRelationListKeys(dv.RelationLinks), key.String()) == -1 {
				dv.RelationLinks = append(dv.RelationLinks, &model.RelationLink{Key: key.String(), Format: rel.Format})
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
				s.Set(simple.New(&model.Block{Content: &model.BlockContentOfDataview{Dataview: dv}, Id: id}))
			}
		}
	}
}

var WithDataviewID = func(id string, dataview model.BlockContentOfDataview, forceViews bool) StateTransformer {
	return func(s *state.State) {
		WithEmpty(s)
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

					/* log.With("thread", s.RootId()).With("name", pbtypes.GetString(s.Details(), "name")).Warnf("dataview needs to be migrated: %v, %v, %v, %v",
					len(dvBlock.Model().GetDataview().Relations) == 0,
					!slice.UnsortedEquals(dvBlock.Model().GetDataview().Source, dataview.Dataview.Source),
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

func InitTemplate(s *state.State, templates ...StateTransformer) (err error) {
	for _, template := range templates {
		template(s)
	}

	return
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
	bundle.RelationKeyNotes.String(),
	bundle.RelationKeyQuote.String(),
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
			s.AddBundledRelations(bundle.RelationKey(k))
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
		log.Errorf("insert relation blocks: %w", err)
		return
	}
}
