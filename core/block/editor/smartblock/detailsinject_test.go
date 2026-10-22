package smartblock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestSmartBlock_getDetailsFromStore(t *testing.T) {
	id := "id"
	t.Run("details are in the store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"id":     domain.String(id),
			"number": domain.Float64(2.18281828459045),
			"🔥":      domain.StringList([]string{"Jeanne d'Arc", "Giordano Bruno", "Capocchio"}),
		})

		err := fx.store.UpdateObjectDetails(context.Background(), id, details)
		require.NoError(t, err)

		// when
		detailsFromStore, err := fx.getDetailsFromStore()

		// then
		assert.NoError(t, err)
		assert.Equal(t, details, detailsFromStore)
	})

	t.Run("no details in the store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		// when
		details, err := fx.getDetailsFromStore()

		// then
		assert.NoError(t, err)
		assert.NotNil(t, details)
	})
}

func TestSmartBlock_injectBackLinks(t *testing.T) {
	backLinks := []string{"1", "2", "3"}
	id := "id"

	t.Run("update back links", func(t *testing.T) {
		// given
		newBackLinks := []string{"4", "5"}
		fx := newFixture(id, t)

		ctx := context.Background()
		err := fx.store.UpdateObjectLinks(ctx, "4", []string{id})
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks(ctx, "5", []string{id})
		require.NoError(t, err)

		st := state.NewDoc("", nil).NewState()
		st.SetDetailAndBundledRelation(bundle.RelationKeyBacklinks, domain.StringList(backLinks))

		// when
		fx.updateBackLinks(st)

		// then
		assert.Equal(t, newBackLinks, st.CombinedDetails().GetStringList(bundle.RelationKeyBacklinks))
	})

	t.Run("back links were found in object store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		ctx := context.Background()
		err := fx.store.UpdateObjectLinks(ctx, "1", []string{id})
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks(ctx, "2", []string{id})
		require.NoError(t, err)
		err = fx.store.UpdateObjectLinks(ctx, "3", []string{id})
		require.NoError(t, err)

		// fx.store.EXPECT().GetInboundLinksById(id).Return(backLinks, nil)
		st := state.NewDoc("", nil).NewState()

		// when
		fx.updateBackLinks(st)

		// then
		details := st.CombinedDetails()
		assert.NotNil(t, details.GetStringList(bundle.RelationKeyBacklinks))
		assert.Equal(t, backLinks, details.GetStringList(bundle.RelationKeyBacklinks))
	})

	t.Run("back links were not found in object store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("", nil).NewState()

		// when
		fx.updateBackLinks(st)

		// then
		assert.Len(t, st.CombinedDetails().GetStringList(bundle.RelationKeyBacklinks), 0)
	})
}

func TestSmartBlock_updatePendingDetails(t *testing.T) {
	id := "id"

	t.Run("no pending details", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		var hasPendingDetails bool
		details := domain.NewDetails()

		// when
		_, result := fx.appendPendingDetails(details)

		// then
		assert.Equal(t, hasPendingDetails, result)
		assert.Zero(t, details.Len())
	})

	t.Run("found pending details", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		details := domain.NewDetails()

		err := fx.store.UpdatePendingLocalDetails(id, func(det *domain.Details) (*domain.Details, error) {
			det.Set(bundle.RelationKeyIsDeleted, domain.Bool(false))
			return det, nil
		})
		require.NoError(t, err)

		// when
		got, _ := fx.appendPendingDetails(details)

		// then
		want := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:        domain.String(id),
			bundle.RelationKeyIsDeleted: domain.Bool(false),
		})
		assert.Equal(t, want, got)
	})

	t.Run("failure on retrieving pending details from the store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		details := domain.NewDetails()

		// when
		_, hasPendingDetails := fx.appendPendingDetails(details)

		// then
		assert.False(t, hasPendingDetails)
	})
}

func TestSmartBlock_injectCreationInfo(t *testing.T) {
	creator := "Anytype"
	creationDate := int64(1692127254)

	t.Run("both creator and creation date are already set", func(t *testing.T) {
		// given
		src := &sourceStub{
			creator:     creator,
			createdDate: creationDate,
			err:         nil,
		}
		sb := &smartBlock{source: src}
		s := &state.State{}
		s.SetLocalDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyCreator:     domain.String(creator),
			bundle.RelationKeyCreatedDate: domain.Int64(creationDate),
		}))

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.NoError(t, err)
		assert.Equal(t, creator, s.LocalDetails().GetString(bundle.RelationKeyCreator))
		assert.Equal(t, creationDate, s.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate))
	})

	t.Run("both creator and creation date are found", func(t *testing.T) {
		// given
		src := &sourceStub{
			creator:     creator,
			createdDate: creationDate,
			err:         nil,
		}
		sb := smartBlock{source: src}
		s := &state.State{}

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.NoError(t, err)
		assert.Equal(t, creator, s.LocalDetails().GetString(bundle.RelationKeyCreator))
		assert.Equal(t, creationDate, s.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate))
	})

	t.Run("failure on retrieving creation info from source", func(t *testing.T) {
		// given
		srcErr := errors.New("source error")
		src := &sourceStub{err: srcErr}
		sb := smartBlock{source: src}
		s := &state.State{}

		// when
		err := sb.injectCreationInfo(s)

		// then
		assert.True(t, errors.Is(err, srcErr))
		assert.Nil(t, s.LocalDetails())
	})
}

func TestInjectLocalDetails(t *testing.T) {
	t.Run("with no details in store get creation info from source", func(t *testing.T) {
		const id = "id"

		fx := newFixture(id, t)
		fx.source.creator = domain.NewParticipantId("testSpace", "testIdentity")
		fx.source.createdDate = time.Now().Unix()

		st := state.NewDoc("id", nil).NewState()

		err := fx.injectLocalDetails(st)

		require.NoError(t, err)

		assert.Equal(t, fx.source.creator, st.LocalDetails().GetString(bundle.RelationKeyCreator))
		assert.Equal(t, fx.source.createdDate, st.LocalDetails().GetInt64(bundle.RelationKeyCreatedDate))
	})

	// TODO More tests
}

func TestInjectDerivedDetails(t *testing.T) {
	const (
		id      = "id"
		spaceId = "testSpace"
	)
	t.Run("links are updated on injection", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", map[string]simple.Block{
			id:         simple.New(&model.Block{Id: id, ChildrenIds: []string{"dataview", "link"}}),
			"dataview": simple.New(&model.Block{Id: "dataview", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{TargetObjectId: "some_set"}}}),
			"link":     simple.New(&model.Block{Id: "link", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "some_obj"}}}),
		}).NewState()
		st.AddRelationLinks(&model.RelationLink{Key: bundle.RelationKeyAssignee.String(), Format: model.RelationFormat_object})
		st.SetDetail(bundle.RelationKeyAssignee, domain.StringList([]string{"Kirill"}))

		// when
		fx.injectDerivedDetails(st, spaceId, smartblock.SmartBlockTypePage)

		// then
		assert.Len(t, st.LocalDetails().GetStringList(bundle.RelationKeyLinks), 3)
	})
}

func TestInjectLinksDetails_filterOutRelations(t *testing.T) {
	const id = "id"

	t.Run("string relation value is filtered out from links", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc(id, map[string]simple.Block{
			id:      simple.New(&model.Block{Id: id, ChildrenIds: []string{"link1", "link2"}}),
			"link1": simple.New(&model.Block{Id: "link1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj1"}}}),
			"link2": simple.New(&model.Block{Id: "link2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "iconImageId"}}}),
		}).NewState()
		st.SetDetail(bundle.RelationKeyIconImage, domain.String("iconImageId"))

		// when
		fx.injectLinksDetails(st)

		// then
		links := st.LocalDetails().GetStringList(bundle.RelationKeyLinks)
		assert.Contains(t, links, "obj1")
		assert.NotContains(t, links, "iconImageId")
	})

	t.Run("string list relation value is filtered out from links", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc(id, map[string]simple.Block{
			id:      simple.New(&model.Block{Id: id, ChildrenIds: []string{"link1", "link2"}}),
			"link1": simple.New(&model.Block{Id: "link1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj1"}}}),
			"link2": simple.New(&model.Block{Id: "link2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "pictureId"}}}),
		}).NewState()
		st.SetDetail(bundle.RelationKeyPicture, domain.StringList([]string{"pictureId", "otherPic"}))

		// when
		fx.injectLinksDetails(st)

		// then
		links := st.LocalDetails().GetStringList(bundle.RelationKeyLinks)
		assert.Contains(t, links, "obj1")
		assert.NotContains(t, links, "pictureId")
	})

	t.Run("links are preserved when filter relations are set but do not match", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc(id, map[string]simple.Block{
			id:      simple.New(&model.Block{Id: id, ChildrenIds: []string{"link1", "link2"}}),
			"link1": simple.New(&model.Block{Id: "link1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj1"}}}),
			"link2": simple.New(&model.Block{Id: "link2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj2"}}}),
		}).NewState()
		st.SetDetail(bundle.RelationKeyIconImage, domain.String("someImage"))
		st.SetDetail(bundle.RelationKeyCoverId, domain.String("someCover"))

		// when
		fx.injectLinksDetails(st)

		// then
		links := st.LocalDetails().GetStringList(bundle.RelationKeyLinks)
		assert.Contains(t, links, "obj1")
		assert.Contains(t, links, "obj2")
	})
}

func TestResolveLayout(t *testing.T) {
	const id = "id"
	t.Run("resolved layout is injected from layout detail", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", nil).NewState()
		st.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_todo))

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_todo), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
	t.Run("failed to get type object id -> fallback to already sey resolvedLayout", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_set))

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_set), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
	t.Run("failed to get type object id and resolvedLayout is not set -> fallback to basic", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", nil).NewState()

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_basic), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
	t.Run("layout is resolved from sb last deps", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyTask.URL()))
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_basic))

		fx.lastDepDetails = map[string]*domain.Details{
			bundle.TypeKeyTask.URL(): domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_todo),
			}),
		}

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_todo), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
	t.Run("layout is resolved from object store", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", nil).NewState()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyProfile.URL()))
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_basic))

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyProfile.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_profile),
		}})

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_profile), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
	t.Run("layout for template is resolved from target type", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc("id", nil).NewState()
		st.SetDetail(bundle.RelationKeyTargetObjectType, domain.String(bundle.TypeKeyTask.URL()))
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyTemplate.URL()))
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_note))
		st.SetObjectTypeKey(bundle.TypeKeyTemplate)

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTemplate.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_profile),
		}, {
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_todo),
		}})

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_todo), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
	t.Run("blocks are not converted, only resolvedLayout is set", func(t *testing.T) {
		// given: a note-shaped object whose type now recommends todo
		fx := newFixture(id, t)

		st := newNoteShapedState(id, "First note block")
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyTask.URL()))
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_note))

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_todo),
		}})

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_todo), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
		assert.Nil(t, st.Pick(state.TitleBlockID))
		assert.Equal(t, []string{state.HeaderLayoutID, "text"}, st.Pick(id).Model().ChildrenIds)
	})
	t.Run("layout is taken from sbType", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		fx.source.sbType = smartblock.SmartBlockTypeIdentity

		st := state.NewDoc(id, nil).NewState()
		st.SetDetails(domain.NewDetails())

		// when
		fx.resolveLayout(st)

		// then
		assert.Equal(t, int64(model.ObjectType_profile), st.LocalDetails().GetInt64(bundle.RelationKeyResolvedLayout))
	})
}

func TestGetFallbackLayout(t *testing.T) {
	const id = "id"
	t.Run("fallback to layout of bundle type", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc(id, nil).NewState()
		st.SetObjectTypeKey(bundle.TypeKeyTask)
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyTask).Return(bundle.TypeKeyTask.URL(), nil).Maybe()

		// when
		v, layoutIsKnown := fx.getFallbackLayoutValue(st)

		// then
		assert.Equal(t, domain.Int64(int64(model.ObjectType_todo)), v)
		assert.True(t, layoutIsKnown)
	})
	t.Run("fallback to file if sbType=file", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		fx.source.sbType = smartblock.SmartBlockTypeFileObject

		st := state.NewDoc(id, nil).NewState()

		// when
		v, layoutIsKnown := fx.getFallbackLayoutValue(st)

		// then
		assert.Equal(t, domain.Int64(int64(model.ObjectType_file)), v)
		assert.True(t, layoutIsKnown)
	})
	t.Run("fallback to basic if title exists", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc(id, map[string]simple.Block{
			id:                   simple.New(&model.Block{Id: id, ChildrenIds: []string{state.HeaderLayoutID}}),
			state.HeaderLayoutID: simple.New(&model.Block{Id: state.HeaderLayoutID, ChildrenIds: []string{state.TitleBlockID}}),
			state.TitleBlockID:   simple.New(&model.Block{Id: state.TitleBlockID}),
		}).NewState()

		// when
		v, layoutIsKnown := fx.getFallbackLayoutValue(st)

		// then
		assert.Equal(t, domain.Int64(int64(model.ObjectType_basic)), v)
		assert.False(t, layoutIsKnown, "layout of an unknown type is only a guess")
	})
	t.Run("fallback to basic, not note, if no title presented", func(t *testing.T) {
		// given
		fx := newFixture(id, t)

		st := state.NewDoc(id, nil).NewState()

		// when
		v, layoutIsKnown := fx.getFallbackLayoutValue(st)

		// then
		assert.Equal(t, domain.Int64(int64(model.ObjectType_basic)), v)
		assert.False(t, layoutIsKnown, "layout of an unknown type is only a guess")
	})
}

// newNoteShapedState builds the note form: no title block, the name lives in the first text block
func newNoteShapedState(rootId, text string) *state.State {
	return state.NewDoc(rootId, map[string]simple.Block{
		rootId: simple.New(&model.Block{Id: rootId, ChildrenIds: []string{state.HeaderLayoutID, "text"}}),
		"text": simple.New(&model.Block{Id: "text", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: text}}}),
	}).NewState()
}

// newTitledState builds the titled form of the page layouts with the given name
func newTitledState(rootId, name string) *state.State {
	st := state.NewDoc(rootId, map[string]simple.Block{
		rootId: simple.New(&model.Block{Id: rootId}),
	}).NewState()
	template.InitTemplate(st, template.WithTitle)
	st.SetDetail(bundle.RelationKeyName, domain.String(name))
	return st
}

func TestConvertLayoutBlocks(t *testing.T) {
	const id = "id"
	withTypeLayout := func(t *testing.T, fx *fixture, st *state.State, layout model.ObjectTypeLayout) {
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyTask.URL()))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(layout),
		}})
	}

	t.Run("type leaves note -> name and title are added", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		withTypeLayout(t, fx, st, model.ObjectType_todo)

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.True(t, converted)
		assert.Equal(t, "First note block", st.Details().GetString(bundle.RelationKeyName))
		assert.NotNil(t, st.PickParentOf(state.TitleBlockID))
	})
	t.Run("type leaves note with a description -> description is added", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		st.SetDetail(bundle.RelationKeyDescription, domain.String("description"))
		withTypeLayout(t, fx, st, model.ObjectType_todo)

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.True(t, converted)
		assert.NotNil(t, st.Pick(state.DescriptionBlockID))
		assert.Contains(t, st.Details().GetStringList(bundle.RelationKeyFeaturedRelations), bundle.RelationKeyDescription.String())
	})
	t.Run("type becomes note -> name moves into a text block and the title is removed", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newTitledState(id, "Hello")
		withTypeLayout(t, fx, st, model.ObjectType_note)

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.True(t, converted)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
		assert.Nil(t, st.PickParentOf(state.TitleBlockID))
		var texts []string
		_ = st.Iterate(func(b simple.Block) bool {
			if txt := b.Model().GetText(); txt != nil {
				texts = append(texts, txt.Text)
			}
			return true
		})
		assert.Contains(t, texts, "Hello")
	})
	t.Run("converting twice -> second time is a no-op", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newTitledState(id, "Hello")
		withTypeLayout(t, fx, st, model.ObjectType_note)
		require.True(t, fx.ConvertLayoutBlocks(st))

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
	})
	t.Run("layout detail wins over the type", func(t *testing.T) {
		// given: the type (as this device sees it) says basic, the object pins note
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		st.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_note))
		withTypeLayout(t, fx, st, model.ObjectType_basic)

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
		assert.Nil(t, st.Pick(state.TitleBlockID))
	})
	t.Run("type is not indexed -> layout is a guess, nothing is converted", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		st.SetObjectTypeKey(domain.TypeKey("teamNote"))
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, domain.TypeKey("teamNote")).Return("typeObjectId", nil).Maybe()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String("typeObjectId"))

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
	})
	t.Run("stored resolvedLayout alone is not used", func(t *testing.T) {
		// given: no layout detail, no type - only a resolvedLayout that may itself be a guess
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		st.SetObjectTypeKey(domain.TypeKey("teamNote"))
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, domain.TypeKey("teamNote")).Return("typeObjectId", nil).Maybe()
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_basic))

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
	})
	t.Run("bundled type is not indexed -> its default layout is not used", func(t *testing.T) {
		// given: Task defaults to todo, but this space's Task may say otherwise and is not indexed
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		st.SetObjectTypeKey(bundle.TypeKeyTask)
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyTask).Return(bundle.TypeKeyTask.URL(), nil).Maybe()
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyTask.URL()))
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(model.ObjectType_note))

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
	})
	t.Run("template follows its target type", func(t *testing.T) {
		// given: a titled template whose target type now recommends note
		fx := newFixture(id, t)
		st := newTitledState(id, "Hello")
		st.SetObjectTypeKey(bundle.TypeKeyTemplate)
		st.SetDetail(bundle.RelationKeyTargetObjectType, domain.String(bundle.TypeKeyTask.URL()))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_note),
		}})

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.True(t, converted)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
	})
	t.Run("type changed in this edit -> the new type's layout is used, not the stale type detail", func(t *testing.T) {
		// given: the edit set the type key to Task (note); the type detail still names Page (basic)
		fx := newFixture(id, t)
		st := newTitledState(id, "Hello")
		st.SetObjectTypeKey(bundle.TypeKeyTask)
		st.SetLocalDetail(bundle.RelationKeyType, domain.String(bundle.TypeKeyPage.URL()))
		fx.space.EXPECT().GetTypeIdByKey(mock.Anything, bundle.TypeKeyTask).Return(bundle.TypeKeyTask.URL(), nil).Maybe()
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyTask.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_note),
		}, {
			bundle.RelationKeyId:                domain.String(bundle.TypeKeyPage.URL()),
			bundle.RelationKeyRecommendedLayout: domain.Int64(model.ObjectType_basic),
		}})

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.True(t, converted)
		assert.Empty(t, st.Details().GetString(bundle.RelationKeyName))
	})
	t.Run("non-page layout -> nothing is converted", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		st := newNoteShapedState(id, "First note block")
		withTypeLayout(t, fx, st, model.ObjectType_set)

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
		assert.Nil(t, st.Pick(state.TitleBlockID))
	})
	t.Run("smartblock type with a fixed layout -> nothing is converted", func(t *testing.T) {
		// given
		fx := newFixture(id, t)
		fx.source.sbType = smartblock.SmartBlockTypeDate
		st := newNoteShapedState(id, "First note block")
		st.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_basic))

		// when
		converted := fx.ConvertLayoutBlocks(st)

		// then
		assert.False(t, converted)
	})
}

func TestLayoutSourceChanged(t *testing.T) {
	newChild := func() *state.State {
		parent := state.NewDoc("id", nil).(*state.State)
		parent.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_basic))
		parent.SetDetail(bundle.RelationKeyTargetObjectType, domain.String("type1"))
		return parent.NewState()
	}
	for _, tc := range []struct {
		name   string
		change func(st *state.State)
		want   bool
	}{
		{"nothing changed", func(st *state.State) {}, false},
		{"unrelated detail changed", func(st *state.State) { st.SetDetail(bundle.RelationKeyName, domain.String("x")) }, false},
		{"layout changed", func(st *state.State) {
			st.SetDetail(bundle.RelationKeyLayout, domain.Int64(model.ObjectType_note))
		}, true},
		{"layout removed", func(st *state.State) { st.RemoveDetail(bundle.RelationKeyLayout) }, true},
		{"template target type changed", func(st *state.State) {
			st.SetDetail(bundle.RelationKeyTargetObjectType, domain.String("type2"))
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			st := newChild()
			tc.change(st)

			// when
			got := LayoutSourceChanged(st)

			// then
			assert.Equal(t, tc.want, got)
		})
	}
}
