package v2service

// typeops_test.go covers the op channel of PATCH types/{type}. The bug it
// exists for is reproduced in typeoptions_test.go: three agents sent a
// one-element property_definitions array to add one field and each lost the
// four the type already had. An op says which of the two it means.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const typeOpsTypeId = "type-plant"

// newTypeOpsFixture builds a Plant type that already lists three fields —
// the shape the benchmark ran on.
func newTypeOpsFixture(t *testing.T) *v2Fixture {
	fx := newV2Fixture(t)
	// `water_needs` is SPACE-MINTED: its stored relation key is a bson id and
	// its served key is a slug. That divergence is the axis this whole channel
	// exists for (typeops.go's header says so), and while every fixture
	// property had key == apiObjectKey, any code that conflated the two passed
	// every assertion here. Keeping one property divergent is what makes these
	// tests able to fail.
	for _, p := range []struct {
		id, key, served, name string
		format                model.RelationFormat
	}{
		{"rel-location", "location", "location", "Location", model.RelationFormat_status},
		{"rel-sun", "sun_needs", "sun_needs", "Sun Needs", model.RelationFormat_status},
		{"rel-water", "6a8f2c1d9e4b7a3f5c2d8e10", "water_needs", "Water Needs", model.RelationFormat_longtext},
	} {
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String(p.id),
			bundle.RelationKeyRelationKey:    domain.String(p.key),
			bundle.RelationKeyApiObjectKey:   domain.String(p.served),
			bundle.RelationKeyName:           domain.String(p.name),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(p.format)),
		})
	}
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:           domain.String(typeOpsTypeId),
		bundle.RelationKeyUniqueKey:    domain.String("ot-plant"),
		bundle.RelationKeyApiObjectKey: domain.String("plant"),
		bundle.RelationKeyName:         domain.String("Plant"),
		bundle.RelationKeyRecommendedRelations: domain.StringList(
			[]string{"rel-location", "rel-sun", "rel-water"}),
	})
	return fx
}

// captureTypeDetails wires ObjectSetDetails and returns what the write sent,
// keyed by detail key.
func (fx *v2Fixture) captureTypeDetails() *map[string][]string {
	captured := map[string][]string{}
	fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
			for _, detail := range req.Details {
				captured[detail.Key] = pbtypes.GetStringListValue(detail.Value)
			}
			return &pb.RpcObjectSetDetailsResponse{
				Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL},
			}
		}).Maybe()
	return &captured
}

// typeReadWithViews is the type's live read: one dataview block carrying the
// views the prune works on.
func typeReadWithViews(views ...*model.BlockContentDataviewView) apicore.ObjectRead {
	return apicore.ObjectRead{
		SbType: model.SmartBlockType_STType,
		Heads:  []string{"headX"},
		Snapshot: &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"id": pbtypes.String(typeOpsTypeId),
			}},
			ObjectTypes: []string{"ot-objectType"},
			Blocks: []*model.Block{
				{Id: typeOpsTypeId, ChildrenIds: []string{state.DataviewBlockID},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: state.DataviewBlockID, Content: &model.BlockContentOfDataview{
					Dataview: &model.BlockContentDataview{
						Views: views,
						// a real dataview carries a link per column — it is the
						// format cache the views read. Without them a view that
						// GROUPS by a column produced a document the validator
						// refused, so nothing here could exercise grouping.
						RelationLinks: relationLinksFor(views),
					}}},
			},
		},
	}
}

// relationLinksFor builds the link set a real dataview carries: one per
// distinct column across its views, with the format the fixture's properties
// declare.
func relationLinksFor(views []*model.BlockContentDataviewView) []*model.RelationLink {
	formats := map[string]model.RelationFormat{
		"location":     model.RelationFormat_status,
		"sun_needs":    model.RelationFormat_status,
		waterStoredKey: model.RelationFormat_longtext,
		"water_needs":  model.RelationFormat_longtext,
		"name":         model.RelationFormat_longtext,
	}
	var links []*model.RelationLink
	seen := map[string]bool{}
	for _, view := range views {
		for _, rel := range view.Relations {
			if rel == nil || seen[rel.Key] {
				continue
			}
			seen[rel.Key] = true
			format, known := formats[rel.Key]
			if !known {
				format = model.RelationFormat_longtext
			}
			links = append(links, &model.RelationLink{Key: rel.Key, Format: format})
		}
	}
	return links
}

func viewWithColumns(id, name string, keys ...string) *model.BlockContentDataviewView {
	view := &model.BlockContentDataviewView{Id: id, Name: name}
	for _, key := range keys {
		view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{Key: key, IsVisible: true})
	}
	return view
}

// expectTypeViewEdit wires the read the prune plans from and the mutation it
// applies, and returns the state the mutation committed.
func (fx *v2Fixture) expectTypeViewEdit(read apicore.ObjectRead) **state.State {
	var captured *state.State
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, typeOpsTypeId).Return(read, nil).Maybe()
	fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, typeOpsTypeId, mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
			st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
			if err != nil {
				return nil, err
			}
			if err := apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st}); err != nil {
				return nil, err
			}
			captured = st
			return []string{"headY"}, nil
		}).Maybe()
	return &captured
}

// viewColumnKeys reads the committed state's view columns back.
func viewColumnKeys(t *testing.T, st *state.State, viewId string) []string {
	t.Helper()
	require.NotNil(t, st, "no state was committed")
	block := st.Pick(state.DataviewBlockID)
	require.NotNil(t, block)
	for _, view := range block.Model().GetDataview().Views {
		if view.Id != viewId {
			continue
		}
		keys := make([]string, 0, len(view.Relations))
		for _, rel := range view.Relations {
			keys = append(keys, rel.Key)
		}
		return keys
	}
	t.Fatalf("view %q is gone", viewId)
	return nil
}

func opsBody(ops ...string) []byte {
	body := `{"ops":[`
	for i, op := range ops {
		if i > 0 {
			body += ","
		}
		body += op
	}
	return []byte(body + `]}`)
}

// TestV2TypeOpsAddKeepsTheFieldsAlreadyThere is the whole point of the
// channel: the call every benchmark agent made, said as an op.
func TestV2TypeOpsAddKeepsTheFieldsAlreadyThere(t *testing.T) {
	t.Run("an add names one field and keeps the rest", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-harvest"),
			bundle.RelationKeyRelationKey:    domain.String("harvest_season"),
			bundle.RelationKeyApiObjectKey:   domain.String("harvest_season"),
			bundle.RelationKeyName:           domain.String("Harvest Season"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)
		want := []string{"rel-location", "rel-sun", "rel-water", "rel-harvest"}

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"harvest_season"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Removed, "an add detaches nothing")
		assert.Equal(t, want, (*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})

	t.Run("a property the type already lists is a no-op", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"sun_needs"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Created)
		assert.Nil(t, result.Removed)
		assert.Equal(t, []string{"rel-location", "rel-sun", "rel-water"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()],
			"the list must come back in the order it went in")
	})

	// The display name is the spelling an agent has; the served api key is
	// the spelling a listing gave it. Both address the same property, which
	// is the rule the view channel broke by matching stored keys alone.
	t.Run("the display name addresses the same property as the api key", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"Sun Needs"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Nil(t, result.Created, "Sun Needs already exists; nothing may be minted for it")
		assert.Equal(t, []string{"rel-location", "rel-sun", "rel-water"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})
}

// TestV2TypeOpsAddMintsAnUnknownName pins the settled answer to the design
// question: add_property mints, exactly as the body does, and says so.
func TestV2TypeOpsAddMintsAnUnknownName(t *testing.T) {
	t.Run("an unknown name is created and reported", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateRelationResponse{
				ObjectId: "rel-minted",
				Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
			}).Maybe()
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)
		want := v2model.PropertyRow{Key: "harvest_season", Name: "Harvest Season", Format: "select"}

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"Harvest Season","format":"select"}`), false, false)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Properties, 1)
		assert.Equal(t, want, result.Created.Properties[0])
		assert.Equal(t, []string{"rel-location", "rel-sun", "rel-water", "rel-minted"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})

	// The mint is the one irreversible half of this op, so the request has to
	// say which format the new property has. Guessing one would make the
	// difference between a text field and a select field a silent default.
	t.Run("a name nothing answers to needs a format", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		minted := 0
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				minted++
				return &pb.RpcObjectCreateRelationResponse{ObjectId: "rel-minted",
					Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL}}
			}).Maybe()

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"Harvest Season"}`), false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.format", apiErr.Issues[0].Path)
		assert.Zero(t, minted, "the request was refused, so it must not have created anything")
	})

	// The format the caller states and the format the property has must be
	// the same thing, or the op silently means something else than it says.
	t.Run("a format that contradicts the existing property is refused", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when: water_needs is text
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"water_needs","format":"select"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.format", apiErr.Issues[0].Path)
	})
}

// TestV2TypeOpsRemovePrunesEveryView is the direction that did not exist: a
// detached property kept showing as a column, which is what made a read of
// the gutted type look correct.
func TestV2TypeOpsRemovePrunesEveryView(t *testing.T) {
	t.Run("the column goes from every view, and the response says so", func(t *testing.T) {
		// given: two views, both showing Sun Needs
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		committed := fx.expectTypeViewEdit(typeReadWithViews(
			viewWithColumns("v-a", "All", "name", "location", "sun_needs"),
			viewWithColumns("v-b", "Grid", "sun_needs", "water_needs"),
		))

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"remove_property","property":"sun_needs"}`), false, false)

		// then: the definitions half
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-location", "rel-water"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])

		// and the report, which a caller cannot infer from the request
		require.NotNil(t, result.Removed)
		require.Len(t, result.Removed.Properties, 1)
		assert.Equal(t, "sun_needs", result.Removed.Properties[0].Key)
		require.NotEmpty(t, result.Warnings)
		assert.Contains(t, result.Warnings[0].Message, "All")
		assert.Contains(t, result.Warnings[0].Message, "Grid")

		// and the view half
		assert.Equal(t, []string{"name", "location"}, viewColumnKeys(t, *committed, "v-a"))
		assert.Equal(t, []string{"water_needs"}, viewColumnKeys(t, *committed, "v-b"))
	})

	// A view that boards by a property is doing more than showing it.
	t.Run("a view that groups by the property is left as it is", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		fx.captureTypeDetails()
		board := viewWithColumns("v-c", "Board", "name", "sun_needs")
		board.GroupRelationKey = "sun_needs"
		committed := fx.expectTypeViewEdit(typeReadWithViews(
			viewWithColumns("v-a", "All", "name", "sun_needs"),
			board,
		))

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"remove_property","property":"sun_needs"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"name"}, viewColumnKeys(t, *committed, "v-a"))
		assert.Equal(t, []string{"name", "sun_needs"}, viewColumnKeys(t, *committed, "v-c"),
			"a board grouped by the property must not lose the column out from under the grouping")

		var warned bool
		for _, warning := range result.Warnings {
			if warning.Path == "/ops" &&
				strings.Contains(warning.Message, "Board") && strings.Contains(warning.Message, "group") {
				warned = true
			}
		}
		assert.True(t, warned, "the views left alone must be named: %v", result.Warnings)
	})

	// A sort and a filter are the same argument as the group.
	t.Run("a view that sorts or filters by the property is left as it is", func(t *testing.T) {
		for name, view := range map[string]*model.BlockContentDataviewView{
			"sorts": func() *model.BlockContentDataviewView {
				v := viewWithColumns("viewOne", "One", "name", "sun_needs")
				v.Sorts = []*model.BlockContentDataviewSort{{RelationKey: "sun_needs"}}
				return v
			}(),
			"filters": func() *model.BlockContentDataviewView {
				v := viewWithColumns("viewOne", "One", "name", "sun_needs")
				v.Filters = []*model.BlockContentDataviewFilter{{NestedFilters: []*model.BlockContentDataviewFilter{
					{RelationKey: "sun_needs"},
				}}}
				return v
			}(),
		} {
			t.Run(name, func(t *testing.T) {
				// given
				fx := newTypeOpsFixture(t)
				fx.captureTypeDetails()
				committed := fx.expectTypeViewEdit(typeReadWithViews(view))

				// when
				_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
					"", opsBody(`{"op":"remove_property","property":"sun_needs"}`), false, false)

				// then: nothing was committed at all, the view being the only one
				require.NoError(t, err)
				assert.Nil(t, *committed, "a view arranged by the property must not be rewritten")
			})
		}
	})

	// A misspelled key is the mistake this op exists to surface.
	t.Run("a property the type does not list is refused", func(t *testing.T) {
		// given: a live property that this type does not recommend
		fx := newTypeOpsFixture(t)
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:           domain.String("rel-elsewhere"),
			bundle.RelationKeyRelationKey:  domain.String("elsewhere"),
			bundle.RelationKeyApiObjectKey: domain.String("elsewhere"),
			bundle.RelationKeyName:         domain.String("Elsewhere"),
		})

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"remove_property","property":"elsewhere"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "elsewhere")
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.property", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "location", "the refusal must say what the type does carry")
	})

	t.Run("a key nothing in the space answers to is refused by name", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"remove_property","property":"sunlight"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "sunlight")
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.property", apiErr.Issues[0].Path)
	})
}

// TestV2TypeOpsMoveReordersTheList covers the third op and the one rule the
// storage forces on it: the sections are four separate lists.
func TestV2TypeOpsMoveReordersTheList(t *testing.T) {
	t.Run("position first makes it the leading field", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"move_property","property":"water_needs","position":"first"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-water", "rel-location", "rel-sun"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})

	t.Run("after places it behind the named field", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"move_property","property":"location","after":"water_needs"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-sun", "rel-water", "rel-location"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})

	t.Run("a move with no destination is refused", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"move_property","property":"location"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		assert.Contains(t, apiErr.Message, "destination")
	})

	// The four sections are four stored lists, so ordering a member of one
	// against a member of another would be a write that changes nothing —
	// the silent no-op this channel exists to end.
	t.Run("ordering across sections is refused, not silently ignored", func(t *testing.T) {
		// given: location is featured, the rest are plain fields
		fx := newV2Fixture(t)
		for _, p := range []struct{ id, key, name string }{
			{"rel-location", "location", "Location"},
			{"rel-sun", "sun_needs", "Sun Needs"},
		} {
			fx.addRelation(t, testSpaceId, objectstore.TestObject{
				bundle.RelationKeyId:           domain.String(p.id),
				bundle.RelationKeyRelationKey:  domain.String(p.key),
				bundle.RelationKeyApiObjectKey: domain.String(p.key),
				bundle.RelationKeyName:         domain.String(p.name),
			})
		}
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                           domain.String(typeOpsTypeId),
			bundle.RelationKeyUniqueKey:                    domain.String("ot-plant"),
			bundle.RelationKeyApiObjectKey:                 domain.String("plant"),
			bundle.RelationKeyName:                         domain.String("Plant"),
			bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-location"}),
			bundle.RelationKeyRecommendedRelations:         domain.StringList([]string{"rel-sun"}),
		})

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"move_property","property":"sun_needs","after":"location"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.after", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "section")
	})

	t.Run("section moves a property between the lists", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectEtagRead(typeOpsTypeId)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"location","section":"featured"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-location"},
			(*captured)[bundle.RelationKeyRecommendedFeaturedRelations.String()])
		assert.Equal(t, []string{"rel-sun", "rel-water"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})
}

// TestV2TypeOpsDryRunReportsTheRealRun: a dry run that hides the destructive
// half is worse than no dry run.
func TestV2TypeOpsDryRunReportsTheRealRun(t *testing.T) {
	body := opsBody(`{"op":"remove_property","property":"sun_needs"}`,
		`{"op":"add_property","property":"Harvest Season","format":"select"}`)

	run := func(t *testing.T, dryRun bool) *v2model.CreateResult {
		t.Helper()
		fx := newTypeOpsFixture(t)
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			Return(&pb.RpcObjectCreateRelationResponse{
				ObjectId: "rel-minted",
				Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
			}).Maybe()
		fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(
			viewWithColumns("v-a", "All", "name", "sun_needs"),
		))
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "", body, dryRun, false)
		require.NoError(t, err)
		return result
	}

	// given / when
	preview := run(t, true)
	real := run(t, false)

	// then
	assert.True(t, preview.DryRun)
	assert.Equal(t, real.Removed, preview.Removed, "the preview must state the removal the real run makes")
	assert.Equal(t, real.Created, preview.Created, "and the property the real run creates")
	assert.Equal(t, real.Warnings, preview.Warnings, "and the views the real run rewrites")
}

// TestV2TypeOpsDryRunWritesNothing is the other half: the preview above is
// only worth reading if it costs nothing.
func TestV2TypeOpsDryRunWritesNothing(t *testing.T) {
	// given
	fx := newTypeOpsFixture(t)
	writes := 0
	fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
			writes++
			return &pb.RpcObjectSetDetailsResponse{
				Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL},
			}
		}).Maybe()
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, typeOpsTypeId).
		Return(typeReadWithViews(viewWithColumns("v-a", "All", "sun_needs")), nil).Maybe()

	// when
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"remove_property","property":"sun_needs"}`), true, false)

	// then: MutateObject is never wired, so reaching it would fail the mock
	require.NoError(t, err)
	assert.Zero(t, writes)
}

// TestV2TypeOpsRefuseBeforeMinting: an op batch that cannot succeed must not
// leave a minted property behind.
func TestV2TypeOpsRefuseBeforeMinting(t *testing.T) {
	// given: a valid mint followed by an op that cannot apply
	fx := newTypeOpsFixture(t)
	minted := 0
	fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
			minted++
			return &pb.RpcObjectCreateRelationResponse{ObjectId: "rel-minted",
				Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL}}
		}).Maybe()

	// when
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"add_property","property":"Harvest Season","format":"select"}`,
			`{"op":"remove_property","property":"nothing_answers_to_this"}`), false, false)

	// then
	require.Error(t, err)
	assert.Zero(t, minted, "the second op refuses the batch, so the first must not have created a property")
}

// TestV2TypeOpsDeclaredOptionsNeedConsent: the select vocabulary an op
// declares is created under the same consent as everywhere else, and the
// refusal arrives before the mint.
func TestV2TypeOpsDeclaredOptionsNeedConsent(t *testing.T) {
	body := opsBody(`{"op":"add_property","property":"Harvest Season","format":"select",` +
		`"options":[{"name":"Early summer"}]}`)

	t.Run("without consent the batch is refused and nothing is created", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		created := 0
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				created++
				return &pb.RpcObjectCreateRelationResponse{ObjectId: "rel-minted",
					Error: &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL}}
			}).Maybe()

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "", body, false, false)

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeValidationFailed, apiErr.Code)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/ops/0.options", apiErr.Issues[0].Path,
			"the refusal must point at the op the caller wrote")
		assert.Zero(t, created, "a refused batch must mint nothing")
	})

	t.Run("with consent the options are reported", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "", body, true, true)

		// then
		require.NoError(t, err)
		require.NotNil(t, result.Created)
		require.Len(t, result.Created.Options, 1)
		assert.Equal(t, "Early summer", result.Created.Options[0].Name)
	})
}

// TestV2TypeOpsEnvelopeIsNotAFlatBody: an ops envelope carries none of the
// members that mark a type document, so without the discriminator it reads
// as the flat body and dies on an unknown key.
func TestV2TypeOpsEnvelopeIsNotAFlatBody(t *testing.T) {
	t.Run("the envelope reaches the op channel", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when: a refusal only the op channel can produce
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"remove_property","property":"sunlight"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.property", apiErr.Issues[0].Path,
			"an ops envelope refused at /ops means the flat body claimed it")
	})

	t.Run("one op sent as the whole body is named", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"op":"remove_property","property":"sun_needs"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/op", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, `{"ops":[`)
	})

	t.Run("an ops envelope mixed with body members is refused", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", []byte(`{"ops":[{"op":"remove_property","property":"sun_needs"}],"name":"Plant"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/name", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "ops envelope")
	})

	t.Run("an unknown op names the type op set", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when: a real op, on the wrong resource
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"insert_blocks","markdown":"hi"}`), true, false)

		// then
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops/0.op", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "add_property")
	})

	// POST has no list to edit, and saying so beats calling ops an unknown
	// field among eight the caller did not mean.
	t.Run("the create verb says ops need a type that exists", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)

		// when
		_, err := fx.CreateType(context.Background(), testSpaceId,
			opsBody(`{"op":"add_property","property":"Location","format":"text"}`), true, false)

		// then: the generic unknown-key message also happens to name
		// property_definitions, so the assertion has to be on what only this
		// refusal says — that ops belong to the other verb
		apiErr := v2Err(t, err)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "/ops", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Message, "ops edit a type that already exists")
		assert.Contains(t, apiErr.Issues[0].Hint, "PATCH /v2/spaces/{space_id}/types/{type}")
		assert.NotContains(t, apiErr.Issues[0].Message, "unknown key")
	})
}

// TestTypeOpSectionsMatchTheFormat pins this file's section table against the
// format's own. The op channel writes the four recommended lists directly —
// the ids it keeps are already known, and re-resolving them would make an
// untouched entry's identity depend on the chain agreeing with itself twice —
// so the pairing of section name to detail key is stated twice and has to be
// checked, not assumed.
func TestTypeOpSectionsMatchTheFormat(t *testing.T) {
	t.Run("the detail keys and their order are the type's four lists", func(t *testing.T) {
		// given
		want := typeRecommendedListKeys

		// when
		got := make([]domain.RelationKey, 0, len(v2TypeSections))
		for _, section := range v2TypeSections {
			got = append(got, section.detailKey)
		}

		// then
		assert.Equal(t, want, got)
	})

	t.Run("each section name lands in the detail key this file pairs it with", func(t *testing.T) {
		for _, section := range v2TypeSections {
			// given: one property in exactly that section
			defs := []anyblockjson.TypeProperty{{InternalKey: "probe", Section: section.section}}

			// when: the format resolves it into the four lists
			lists, err := anyblockjson.BuildRecommendedLists(defs, anyblockjson.Options{})

			// then
			require.NoError(t, err)
			landed := ""
			for _, list := range lists {
				if len(list.Ids) == 1 && list.Ids[0] == "probe" {
					landed = list.DetailKey
				}
			}
			assert.Equal(t, string(section.detailKey), landed,
				"section %q", section.section)
		}
	})
}

// waterStoredKey is the bson stored key of the fixture's space-minted
// property. A real view column carries THIS, while a caller addresses the
// property as `water_needs`. Tests that key a column by the served spelling
// cannot tell the two apart, which is how a prune-by-served-key survived.
const waterStoredKey = "6a8f2c1d9e4b7a3f5c2d8e10"

// TestV2TypeOpsPrunesByStoredKey pins the axis this channel exists for. A
// space-minted property stores a bson key and serves a slug; view columns
// carry the stored one. Pruning by the served spelling finds no column,
// removes nothing, and reports success — the worst outcome available here,
// because the response says the removal happened.
func TestV2TypeOpsPrunesByStoredKey(t *testing.T) {
	// given: the column carries the STORED key, as a real one does
	fx := newTypeOpsFixture(t)
	captured := fx.captureTypeDetails()
	committed := fx.expectTypeViewEdit(typeReadWithViews(
		viewWithColumns("v1", "All", "name", "location", waterStoredKey),
	))

	// when: the caller addresses it by the only spelling they have
	result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"remove_property","property":"water_needs"}`), false, false)

	// then: detached from the definitions
	require.NoError(t, err)
	assert.Equal(t, []string{"rel-location", "rel-sun"},
		(*captured)[bundle.RelationKeyRecommendedRelations.String()])

	// and the column is actually gone, which only holds if the prune used the
	// stored key
	assert.Equal(t, []string{"name", "location"}, viewColumnKeys(t, *committed, "v1"))

	// and the report names the property the way the caller does
	require.NotNil(t, result.Removed)
	require.Len(t, result.Removed.Properties, 1)
	assert.Equal(t, "water_needs", result.Removed.Properties[0].Key)
}

// TestV2TypeOpsCompose covers batches of more than one op. Every composition
// defect found in review came from the plan reading its op LOG rather than its
// net effect, and nothing exercised two ops together.
func TestV2TypeOpsCompose(t *testing.T) {
	t.Run("remove then add the same property keeps its columns", func(t *testing.T) {
		// given: the natural re-sectioning batch
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		committed := fx.expectTypeViewEdit(typeReadWithViews(
			viewWithColumns("v1", "All", "name", "location", "sun_needs"),
		))

		// when
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(
				`{"op":"remove_property","property":"sun_needs"}`,
				`{"op":"add_property","property":"sun_needs","section":"featured"}`,
			), false, false)
		// the batch nets to zero removals, so the prune never runs and no view
		// edit is committed at all — which is the point: the column is safe
		// because nothing went looking for it.
		_ = committed

		// then: the type still lists it, so nothing was detached
		require.NoError(t, err)
		assert.Contains(t, (*captured)[bundle.RelationKeyRecommendedFeaturedRelations.String()], "rel-sun")
		assert.Nil(t, result.Removed, "it is still on the type, so nothing was removed")

		// and no view edit was committed, because there was nothing to prune
		assert.Nil(t, *committed, "a net-zero batch must not touch the views")
	})

	t.Run("add then remove a new property creates nothing", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name")))
		minted := 0
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				minted++
				return &pb.RpcObjectCreateRelationResponse{
					Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
					ObjectId: "rel-new",
				}
			}).Maybe()

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(
				`{"op":"add_property","property":"Harvest Season","format":"select"}`,
				`{"op":"remove_property","property":"Harvest Season"}`,
			), false, false)

		// then: the batch nets to nothing, so it must leave no relation behind
		require.NoError(t, err)
		assert.Zero(t, minted, "a property added and removed in one batch must not be created")
	})

	t.Run("adding the same new property twice lists it once", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name")))
		fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
				return &pb.RpcObjectCreateRelationResponse{
					Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
					ObjectId: "rel-new",
				}
			}).Maybe()

		// when: the second op names the same property, into another section
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(
				`{"op":"add_property","property":"Harvest Season","format":"select"}`,
				`{"op":"add_property","property":"Harvest Season","section":"featured"}`,
			), false, false)

		// then: one relation, in exactly one of the four lists
		require.NoError(t, err)
		seen := 0
		for _, key := range []domain.RelationKey{
			bundle.RelationKeyRecommendedRelations,
			bundle.RelationKeyRecommendedFeaturedRelations,
			bundle.RelationKeyRecommendedHiddenRelations,
			bundle.RelationKeyRecommendedFileRelations,
		} {
			for _, id := range (*captured)[key.String()] {
				if id == "rel-new" {
					seen++
				}
			}
		}
		assert.Equal(t, 1, seen, "one relation must not land in two lists, nor twice in one")
	})
}

// TestV2TypeOpsBundledFormatConflict covers the arm the format check used to
// miss: a bundled property this space has NOT installed. The check lived
// inside the installed arm, so a declared format that disagreed with the
// bundled one fell through and minted a relation derived from the bundled key
// with the wrong format — resolvable on every device and undoable by no v2
// call. The whole-type body refuses the same request.
func TestV2TypeOpsBundledFormatConflict(t *testing.T) {
	// given: audioAlbum is bundled (text) and not installed in this space
	fx := newTypeOpsFixture(t)
	minted := 0
	fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
			minted++
			return &pb.RpcObjectCreateRelationResponse{
				Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
				ObjectId: "rel-bad",
			}
		}).Maybe()

	// when: a format that contradicts the bundled one
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"add_property","property":"audioAlbum","format":"number"}`), true, false)

	// then
	require.Error(t, err, "a format that disagrees with the bundled one must be refused")
	apiErr := v2Err(t, err)
	assert.Contains(t, apiErr.Message, "format conflict")
	assert.Zero(t, minted, "and nothing may be created on the bundled key")
}

// TestV2TypeOpsRefusalsAreDistinguishable covers two mistakes that must not
// read the same. "this type does not list it" is a caller who named a real
// property on the wrong type; "nothing answers to it" is a typo. Both were
// satisfied by one assertion, so either message could have drifted into the
// other without a test noticing.
func TestV2TypeOpsRefusalsAreDistinguishable(t *testing.T) {
	// given: `priority` is a real bundled property, and this type does not list it
	fx := newTypeOpsFixture(t)

	// when
	_, listedErr := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"remove_property","property":"priority"}`), true, false)
	_, unknownErr := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"remove_property","property":"no_such_property_at_all"}`), true, false)

	// then: both refuse
	require.Error(t, listedErr)
	require.Error(t, unknownErr)
	listed := v2Err(t, listedErr).Message
	unknown := v2Err(t, unknownErr).Message

	// and they say different things — a caller who misspelled should not be
	// told to go look at the type, and vice versa
	assert.NotEqual(t, listed, unknown,
		"a property that exists but is not on this type is a different mistake from a typo")
}

// TestV2TypeOpsEmptySectionIsWritten pins the one write that looks like a
// no-op and is not. Emptying a section must send an EMPTY list for it: skipping
// the detail leaves the old ids stored, so the property stays in that section
// with no op able to clear it.
func TestV2TypeOpsEmptySectionIsWritten(t *testing.T) {
	// given: a type whose only featured field is sun_needs
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-sun"),
		bundle.RelationKeyRelationKey:    domain.String("sun_needs"),
		bundle.RelationKeyApiObjectKey:   domain.String("sun_needs"),
		bundle.RelationKeyName:           domain.String("Sun Needs"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                           domain.String(typeOpsTypeId),
		bundle.RelationKeyUniqueKey:                    domain.String("ot-plant"),
		bundle.RelationKeyApiObjectKey:                 domain.String("plant"),
		bundle.RelationKeyName:                         domain.String("Plant"),
		bundle.RelationKeyRecommendedFeaturedRelations: domain.StringList([]string{"rel-sun"}),
	})
	captured := fx.captureTypeDetails()
	fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name", "sun_needs")))

	// when: the only member of that section leaves
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
		"", opsBody(`{"op":"remove_property","property":"sun_needs"}`), false, false)

	// then: the section is written EMPTY, not skipped
	require.NoError(t, err)
	featured, written := (*captured)[bundle.RelationKeyRecommendedFeaturedRelations.String()]
	assert.True(t, written, "the emptied section must still be written")
	assert.Empty(t, featured)
}

// TestV2TypeOpsAddPlacement covers add_property's placement, which had no
// coverage in either branch: deleting both placement calls passed the suite.
func TestV2TypeOpsAddPlacement(t *testing.T) {
	t.Run("position first leads the section", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name")))

		// when: an already-listed property moves to the front via add_property
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"water_needs","position":"first"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-water", "rel-location", "rel-sun"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})

	t.Run("after anchors on an existing member", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name")))

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant",
			"", opsBody(`{"op":"add_property","property":"water_needs","after":"location"}`), false, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-location", "rel-water", "rel-sun"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])
	})
}

// TestV2TypeOpsIfMatch covers the concurrency precondition. The ops path is a
// server-side read-modify-write of all four lists across several RPCs, so a
// change landing in that window is otherwise silently reverted — and a
// reverted featured list cascades to every object of the type.
func TestV2TypeOpsIfMatch(t *testing.T) {
	t.Run("a stale etag is refused before anything is planned", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name", "sun_needs")))
		wrote := false
		fx.mwMock.EXPECT().ObjectSetDetails(mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
				wrote = true
				return &pb.RpcObjectSetDetailsResponse{
					Error: &pb.RpcObjectSetDetailsResponseError{Code: pb.RpcObjectSetDetailsResponseError_NULL},
				}
			}).Maybe()

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "\"deadbeef\"",
			opsBody(`{"op":"remove_property","property":"sun_needs"}`), false, false)

		// then
		require.Error(t, err)
		assert.Equal(t, http.StatusConflict, v2Err(t, err).Status)
		assert.False(t, wrote, "a stale precondition must refuse before the write")
	})

	t.Run("an absent etag is last-write-wins", func(t *testing.T) {
		// given: the documented advisory default
		fx := newTypeOpsFixture(t)
		fx.captureTypeDetails()
		fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v1", "All", "name", "sun_needs")))

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"remove_property","property":"sun_needs"}`), false, false)

		// then
		require.NoError(t, err)
	})
}

// TestV2TypeOpsViewFamily covers the read/write asymmetry this closes.
// GET /types/{key} serves the type's dataview WITH its views, so a caller
// reads a view off the type and reasonably edits it on the same resource.
// Before, that returned `unknown op "insert_view" on a type` — the resource
// showed you a view and called the op that changes it unknown.
func TestV2TypeOpsViewFamily(t *testing.T) {
	t.Run("insert_view runs on the type's own dataview", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		committed := fx.expectTypeViewEdit(typeReadWithViews(
			viewWithColumns("v-a", "All", "name", "sun_needs"),
		))

		// when: the op the object channel takes, sent to the type
		result, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"insert_view","name":"By sun","set":{"type":"kanban","group_by":"sun_needs"}}`),
			false, false)

		// then
		require.NoError(t, err, "a type shows its views, so it must accept view ops")
		require.NotNil(t, *committed)
		var inserted *model.BlockContentDataviewView
		for _, v := range (*committed).Pick(state.DataviewBlockID).Model().GetDataview().Views {
			if v.Name == "By sun" {
				inserted = v
			}
		}
		require.NotNil(t, inserted, "the view must exist on the type's dataview")

		// and it is the view that was asked for, not merely a view with the
		// right name: the arrangement is what the caller wanted
		assert.Equal(t, model.BlockContentDataviewView_Kanban, inserted.Type)
		assert.Equal(t, "sun_needs", inserted.GroupRelationKey)

		// and the minted view id comes back, since the payload has no id slot
		require.NotEmpty(t, result.CreatedViews, "a server-minted view id must be reported")
	})

	t.Run("a property op and a view op compose, property first", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)
		captured := fx.captureTypeDetails()
		committed := fx.expectTypeViewEdit(typeReadWithViews(
			viewWithColumns("v-a", "All", "name", "location", "sun_needs"),
		))

		// when: one batch, both halves
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(
				`{"op":"remove_property","property":"sun_needs"}`,
				`{"op":"insert_view","name":"Plain","set":{"type":"list"}}`,
			), false, false)

		// then: the property left the lists
		require.NoError(t, err)
		assert.Equal(t, []string{"rel-location", "rel-water"},
			(*captured)[bundle.RelationKeyRecommendedRelations.String()])

		// and the view exists — the prune ran first and did not undo it
		var names []string
		for _, v := range (*committed).Pick(state.DataviewBlockID).Model().GetDataview().Views {
			names = append(names, v.Name)
		}
		assert.Contains(t, names, "Plain")
	})

	t.Run("a non-view object op is still refused", func(t *testing.T) {
		// given: widening to the view family must not open the whole object set
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"insert_blocks","blocks":[{"type":"paragraph","text":"x"}]}`), true, false)

		// then
		require.Error(t, err)
		assert.Contains(t, v2Err(t, err).Message, "unknown op")
	})
}

// TestV2TypeOpsMintIsAddressableInTheSameBatch is the design doc's own example
// batch (APIV2_TYPE_OPS.md): add a property, then move it by the key the same
// response reports under `created`. A mint's entry must therefore carry the
// spelling it will ACTUALLY get, not the display name the add op used.
func TestV2TypeOpsMintIsAddressableInTheSameBatch(t *testing.T) {
	// given
	fx := newTypeOpsFixture(t)
	captured := fx.captureTypeDetails()
	fx.expectTypeViewEdit(typeReadWithViews(viewWithColumns("v-a", "All", "name")))
	fx.mwMock.EXPECT().ObjectCreateRelation(mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
			return &pb.RpcObjectCreateRelationResponse{
				Error:    &pb.RpcObjectCreateRelationResponseError{Code: pb.RpcObjectCreateRelationResponseError_NULL},
				ObjectId: "rel-harvest",
				Key:      "harvest_season",
			}
		}).Maybe()

	// when: the doc's batch verbatim — display name to create, api key to move
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
		opsBody(
			`{"op":"add_property","property":"Harvest Season","format":"select"}`,
			`{"op":"move_property","property":"harvest_season","position":"first"}`,
		), false, false)

	// then
	require.NoError(t, err, "the key the response reports must address the property it reports")
	assert.Equal(t, "rel-harvest", (*captured)[bundle.RelationKeyRecommendedRelations.String()][0],
		"position first must have moved it to the front")
}

// TestV2TypeOpsMintFormatIsConsistent covers the waiver hole. A second op
// naming a property the batch is MINTING must obey the same format rule as one
// naming an existing property — otherwise a select vocabulary attaches to a
// property being created as text, and options are irreversible.
func TestV2TypeOpsMintFormatIsConsistent(t *testing.T) {
	t.Run("a contradicting format on a pending mint is refused", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(
				`{"op":"add_property","property":"Harvest Season","format":"select"}`,
				`{"op":"add_property","property":"Harvest Season","format":"number"}`,
			), true, false)

		// then
		require.Error(t, err)
		assert.Contains(t, v2Err(t, err).Message, "format conflict")
	})

	t.Run("a vocabulary cannot attach to a property the batch mints as text", func(t *testing.T) {
		// given
		fx := newTypeOpsFixture(t)

		// when: consent granted, so only the format rule can refuse this
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(
				`{"op":"add_property","property":"Tags","format":"text"}`,
				`{"op":"add_property","property":"Tags","options":[{"name":"Red"}]}`,
			), true, true)

		// then
		require.Error(t, err, "options belong to a select property, and this one is text")
		assert.Contains(t, v2Err(t, err).Message, "select format")
	})
}

// TestV2TypeOpsFileSectionSurvivesRemoveAndAdd closes the one-way guard's
// bypass: remove_property drops the entry, so a later add of the same property
// re-appended it into an authorable section and performed the irreversible
// move the guard refuses when it is stated directly.
func TestV2TypeOpsFileSectionSurvivesRemoveAndAdd(t *testing.T) {
	fx := newV2Fixture(t)
	fx.addRelation(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-photo"),
		bundle.RelationKeyRelationKey:    domain.String("photo"),
		bundle.RelationKeyApiObjectKey:   domain.String("photo"),
		bundle.RelationKeyName:           domain.String("Photo"),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_file)),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                       domain.String(typeOpsTypeId),
		bundle.RelationKeyUniqueKey:                domain.String("ot-plant"),
		bundle.RelationKeyApiObjectKey:             domain.String("plant"),
		bundle.RelationKeyName:                     domain.String("Plant"),
		bundle.RelationKeyRecommendedFileRelations: domain.StringList([]string{"rel-photo"}),
	})

	// when: the laundered move — remove it, then add it back somewhere else
	_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
		opsBody(
			`{"op":"remove_property","property":"photo"}`,
			`{"op":"add_property","property":"photo","section":"featured"}`,
		), true, false)

	// then
	require.Error(t, err, "a removal must not launder a move this channel refuses outright")
	assert.Contains(t, v2Err(t, err).Message, "file field")
}

// TestV2TypeOpNamesMatchTheDispatch pins the two halves of the type channel's
// op set to each other. `v2TypeOpNames` is declarative — it feeds the schema
// index and the refusal text — while the accepted set is the switch in
// planTypeOps. Nothing connected them, so a name could be advertised with no
// case behind it (served and unreachable), or a case could accept an op the
// surface never mentions. A mutation removing the view family from the list
// left the whole package green, which is how this gap was found.
func TestV2TypeOpNamesMatchTheDispatch(t *testing.T) {
	// a minimal, well-formed body per op — enough to reach the dispatch
	bodies := map[string]string{
		"add_property":    `{"op":"add_property","property":"location"}`,
		"remove_property": `{"op":"remove_property","property":"location"}`,
		"move_property":   `{"op":"move_property","property":"location","position":"first"}`,
		"insert_view":     `{"op":"insert_view","name":"X"}`,
		"update_view":     `{"op":"update_view","view":"v-a","set":{"name":"Y"}}`,
		"move_view":       `{"op":"move_view","view":"v-a","position":"first"}`,
		"delete_view":     `{"op":"delete_view","view":"v-a"}`,
	}

	t.Run("every advertised op reaches a dispatch arm", func(t *testing.T) {
		for _, op := range v2TypeOpNames {
			body, covered := bodies[op]
			require.True(t, covered, "%s is advertised but this test has no body for it", op)

			fx := newTypeOpsFixture(t)
			fx.captureTypeDetails()
			fx.expectTypeViewEdit(typeReadWithViews(
				viewWithColumns("v-a", "All", "name", "location"),
				viewWithColumns("v-b", "Grid", "name"),
			))

			// then: it may fail on its own merits, but never as UNKNOWN —
			// that is the signal it has no arm
			_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "", opsBody(body), true, false)
			if err != nil {
				assert.NotContains(t, v2Err(t, err).Message, "unknown op",
					"%s is advertised on this endpoint but the dispatch refuses it", op)
			}
		}
	})

	t.Run("an op the list does not name is refused", func(t *testing.T) {
		// given: a real object-channel op that is not in the type's set
		fx := newTypeOpsFixture(t)

		// when
		_, err := fx.UpdateType(context.Background(), testSpaceId, "plant", "",
			opsBody(`{"op":"set_properties","set":{"name":"X"}}`), true, false)

		// then
		require.Error(t, err)
		assert.Contains(t, v2Err(t, err).Message, "unknown op",
			"the accepted set must be exactly what the surface advertises")
	})
}
