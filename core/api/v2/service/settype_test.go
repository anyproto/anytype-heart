package v2service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// typeChangeRefusedProduction mirrors the adapter's checkRestriction output
// for the TypeChange axis, the way blocksRefusedProduction does for Blocks.
func typeChangeRefusedProduction() error {
	return fmt.Errorf("%w: this object's type cannot be changed through the API",
		fmt.Errorf("%w: %s", restriction.ErrRestricted, model.Restrictions_TypeChange.String()))
}

// addLayoutTypes seeds the installed page and task types (page family) and
// a space-minted "Shelf" type whose instances are collections — the layout
// a page can never convert to.
func (fx *v2Fixture) addLayoutTypes(t *testing.T) {
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                domain.String("type-page"),
		bundle.RelationKeyUniqueKey:         domain.String("ot-page"),
		bundle.RelationKeyApiObjectKey:      domain.String("page"),
		bundle.RelationKeyName:              domain.String("Page"),
		bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_basic)),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                domain.String("type-task"),
		bundle.RelationKeyUniqueKey:         domain.String("ot-task"),
		bundle.RelationKeyApiObjectKey:      domain.String("task"),
		bundle.RelationKeyName:              domain.String("Task"),
		bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_todo)),
	})
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                domain.String("type-shelf"),
		bundle.RelationKeyUniqueKey:         domain.String("ot-shelf"),
		bundle.RelationKeyApiObjectKey:      domain.String("shelf"),
		bundle.RelationKeyName:              domain.String("Shelf"),
		bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_collection)),
	})
}

// installTask is what the real InstallBundledType leaves behind: a readable
// type row for task.
func (fx *v2Fixture) installTask(t *testing.T) {
	fx.addType(t, testSpaceId, objectstore.TestObject{
		bundle.RelationKeyId:                domain.String("type-task"),
		bundle.RelationKeyUniqueKey:         domain.String("ot-task"),
		bundle.RelationKeyApiObjectKey:      domain.String("task"),
		bundle.RelationKeyName:              domain.String("Task"),
		bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_todo)),
	})
}

// typedMutation is what expectMutateTyped records: the keys the editor hook
// was handed, and the layout the state reported at each call — the value
// the editor's own conversion would read.
type typedMutation struct {
	keys    []domain.TypeKey
	layouts []model.ObjectTypeLayout
	state   *state.State
}

// expectMutateTyped is expectMutate with the editor's type-change hook
// wired the way the adapter wires it: the hook records the call and writes
// the type keys, standing in for basic.SetObjectTypesInState.
func (fx *v2Fixture) expectMutateTyped(read apicore.ObjectRead, newHeads ...string) *typedMutation {
	rec := &typedMutation{}
	fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
	fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
			st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
			if err != nil {
				return nil, err
			}
			edit := apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st,
				SetObjectType: func(st *state.State, key domain.TypeKey) error {
					layout, _ := st.Layout()
					rec.keys = append(rec.keys, key)
					rec.layouts = append(rec.layouts, layout)
					st.SetObjectTypeKeys([]domain.TypeKey{key})
					return nil
				}}
			if err := apply(edit); err != nil {
				return nil, err
			}
			rec.state = st
			return newHeads, nil
		})
	return rec
}

func TestPatchObjectSetType(t *testing.T) {
	ctx := context.Background()

	t.Run("set_type goes through the editor hook and demands the type axis alone", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		read := editRead(t, editBaseDoc)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
		var got apicore.EditNeeds
		var hooked []domain.TypeKey
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
				got = needs
				st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
				if err != nil {
					return nil, err
				}
				edit := apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st,
					SetObjectType: func(st *state.State, key domain.TypeKey) error {
						hooked = append(hooked, key)
						st.SetObjectTypeKeys([]domain.TypeKey{key})
						return nil
					}}
				if err := apply(edit); err != nil {
					return nil, err
				}
				return []string{"headB"}, nil
			})

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		require.NoError(t, err)
		assert.Equal(t, apicore.EditNeeds{TypeChange: true}, got, "set_type edits neither blocks nor properties directly")
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, hooked, "the editor path, not a bare key write")
		assert.Empty(t, result.Warnings, "a committed change is not a dry run")
	})

	t.Run("the receipt reports the type change", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		require.NoError(t, err)
		require.NotNil(t, result.TypeChanged, "diff_stats cannot show a type change; the receipt must")
		assert.Equal(t, &v2model.TypeChange{From: "page", To: "task"}, result.TypeChanged)
	})

	t.Run("a commit without an editor behind the state is an error, never a bare key write", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		captured := fx.expectMutate(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "type")
		assert.Nil(t, *captured, "nothing committed")
	})

	t.Run("the type term resolves like create's: slug, name and did-you-mean", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		rec := fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"Task"}`), "", false, false)

		require.NoError(t, err, "a display name is an accepted spelling")
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, rec.keys)
	})

	t.Run("the current type is a no-op with no receipt entry", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		rec := fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"page"}`), "", false, false)

		require.NoError(t, err)
		assert.Empty(t, rec.keys, "nothing to convert, so the editor is not asked")
		assert.Nil(t, result.TypeChanged)
		assert.Empty(t, result.Warnings)
	})

	t.Run("a bundled type the space has not installed is installed before the lock", func(t *testing.T) {
		fx := newV2Fixture(t)
		// no live types at all: task resolves through the bundle alone
		installed := false
		fx.creatorMock.EXPECT().InstallBundledType(mock.Anything, testSpaceId, bundle.TypeKeyTask).
			RunAndReturn(func(context.Context, string, domain.TypeKey) error {
				installed = true
				fx.installTask(t) // the install makes the type readable, as the real one does
				return nil
			}).Once()
		read := editRead(t, editBaseDoc)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
		var hooked []domain.TypeKey
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
				require.True(t, installed, "the install is its own object create and must not run under the object lock")
				st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
				if err != nil {
					return nil, err
				}
				return []string{"headB"}, apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st,
					SetObjectType: func(st *state.State, key domain.TypeKey) error {
						hooked = append(hooked, key)
						st.SetObjectTypeKeys([]domain.TypeKey{key})
						return nil
					}})
			})

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		require.NoError(t, err)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, hooked)
	})

	t.Run("a set_type the service can refuse up front installs nothing", func(t *testing.T) {
		fx := newV2Fixture(t)
		// no InstallBundledType and no mutator expectation: either call
		// would fail the test
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task","typo":true}`), "", false, false)
		require.Error(t, err, "an unknown member is refused before anything is installed")

		_, err = fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"collection"}`), "", false, false)
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status, "a page cannot become a collection: refused before the install")
		assert.Contains(t, apiErr.Message, "collection layout")
	})

	t.Run("a refused set_type creates no option either", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("type-task"),
			bundle.RelationKeyUniqueKey:     domain.String("ot-task"),
			bundle.RelationKeyName:          domain.String("Task"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		fx.addRelation(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:             domain.String("rel-severity"),
			bundle.RelationKeyRelationKey:    domain.String("severity"),
			bundle.RelationKeyName:           domain.String("Severity"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		})
		// no ObjectCreateRelationOption, no install, no mutator expectation:
		// the removed type must be refused before the option is minted
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"severity":["Brand new"]}}`, `{"op":"set_type","type":"task"}`), "", false, true)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[1].type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Message, "removed")
	})

	t.Run("a return to the original type later in the batch still installs it", func(t *testing.T) {
		fx := newV2Fixture(t)
		// the object IS a page, but the space has no page row (never
		// installed); [task, page] must install page for the second op —
		// read from the original snapshot alone, the second op looks like
		// a no-op and the editor would fail under the lock
		fx.installTask(t)
		fx.creatorMock.EXPECT().InstallBundledType(mock.Anything, testSpaceId, bundle.TypeKeyPage).
			RunAndReturn(func(context.Context, string, domain.TypeKey) error {
				fx.addType(t, testSpaceId, objectstore.TestObject{
					bundle.RelationKeyId:                domain.String("type-page"),
					bundle.RelationKeyUniqueKey:         domain.String("ot-page"),
					bundle.RelationKeyApiObjectKey:      domain.String("page"),
					bundle.RelationKeyName:              domain.String("Page"),
					bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_basic)),
				})
				return nil
			}).Once()
		rec := fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`, `{"op":"set_type","type":"page"}`), "", false, false)

		require.NoError(t, err)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask, bundle.TypeKeyPage}, rec.keys)
	})

	t.Run("an install the store reports a moment later is waited for", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.creatorMock.EXPECT().InstallBundledType(mock.Anything, testSpaceId, bundle.TypeKeyTask).
			RunAndReturn(func(context.Context, string, domain.TypeKey) error {
				go func() {
					time.Sleep(80 * time.Millisecond)
					fx.installTask(t)
				}()
				return nil
			}).Once()
		read := editRead(t, editBaseDoc)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
		taskKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyTask.String())
		require.NoError(t, err)
		var hooked []domain.TypeKey
		fx.mutatorMock.EXPECT().MutateObject(mock.Anything, testSpaceId, "obj1", mock.Anything, mock.Anything).
			RunAndReturn(func(ctx context.Context, spaceId, objectId string, needs apicore.EditNeeds, apply func(apicore.ObjectEdit) error) ([]string, error) {
				// the editor's own lookup under the lock: it must already succeed
				_, lookupErr := fx.objectStore.SpaceIndex(testSpaceId).GetObjectByUniqueKey(taskKey)
				require.NoError(t, lookupErr, "the mutator must not start before the installed type is readable")
				st, err := state.NewDocFromSnapshot(objectId, &pb.ChangeSnapshot{Data: read.Snapshot})
				if err != nil {
					return nil, err
				}
				return []string{"headB"}, apply(apicore.ObjectEdit{SbType: read.SbType, Heads: read.Heads, State: st,
					SetObjectType: func(st *state.State, key domain.TypeKey) error {
						hooked = append(hooked, key)
						st.SetObjectTypeKeys([]domain.TypeKey{key})
						return nil
					}})
			})

		_, err = fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		require.NoError(t, err)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask}, hooked)
	})

	t.Run("a read-only space is a 403 up front, not a wait", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.creatorMock.EXPECT().InstallBundledType(mock.Anything, testSpaceId, bundle.TypeKeyTask).Return(apicore.ErrSpaceReadOnly).Once()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)
		started := time.Now()

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusForbidden, apiErr.Status)
		assert.Less(t, time.Since(started), time.Second, "no readability wait for a space that cannot be written")
	})

	t.Run("an install that never becomes readable is an error, not a 500 under the lock", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.creatorMock.EXPECT().InstallBundledType(mock.Anything, testSpaceId, bundle.TypeKeyTask).Return(nil).Once()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)
		// no mutator expectation: the wait fails first
		previous := v2InstalledTypeVisibleTimeout
		v2InstalledTypeVisibleTimeout = 100 * time.Millisecond
		defer func() { v2InstalledTypeVisibleTimeout = previous }()

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not readable yet")
	})

	t.Run("a probe pass never edits the read it was built from", func(t *testing.T) {
		read := editRead(t, editBaseDoc)
		edit, err := editFromRead("obj1", read)
		require.NoError(t, err)
		blocksBefore := len(read.Snapshot.Blocks)
		root := read.Snapshot.Blocks[0]
		childrenBefore := append([]string(nil), root.ChildrenIds...)

		edit.State.Unlink("blockHeading1")

		assert.Equal(t, childrenBefore, root.ChildrenIds, "the state must own copies of the blocks")
		assert.Len(t, read.Snapshot.Blocks, blocksBefore)
	})

	t.Run("a removed bundled type is refused, not resurrected by the install", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:            domain.String("type-task"),
			bundle.RelationKeyUniqueKey:     domain.String("ot-task"),
			bundle.RelationKeyName:          domain.String("Task"),
			bundle.RelationKeyIsUninstalled: domain.Bool(true),
		})
		// no InstallBundledType expectation (reinstalling is exactly the
		// bug) and no mutator expectation (the refusal precedes the lock)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)

		for _, dryRun := range []bool{false, true} {
			_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
				patchBody(`{"op":"set_type","type":"task"}`), "", dryRun, false)

			apiErr := v2Err(t, err)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			require.NotEmpty(t, apiErr.Issues)
			assert.Equal(t, "ops[0].type", apiErr.Issues[0].Path)
			assert.Contains(t, apiErr.Issues[0].Message, "removed")
		}
	})

	t.Run("the receipt spells the types the way the caller's reads do", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		fx.addType(t, testSpaceId, objectstore.TestObject{
			bundle.RelationKeyId:                domain.String("type-plan"),
			bundle.RelationKeyUniqueKey:         domain.String("ot-plan"),
			bundle.RelationKeyApiObjectKey:      domain.String("project_plan"),
			bundle.RelationKeyName:              domain.String("Project Plan"),
			bundle.RelationKeyRecommendedLayout: domain.Int64(int64(model.ObjectType_basic)),
		})
		fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(CtxWithNameKeys(ctx), testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"Project Plan"}`), "", false, false)

		require.NoError(t, err)
		assert.Equal(t, &v2model.TypeChange{From: "Page", To: "Project Plan"}, result.TypeChanged)
	})

	t.Run("set_type is a view-rebuilding op", func(t *testing.T) {
		assert.True(t, v2OpRebuildsView["set_type"], "the conversion adds and moves blocks; the next op must re-render")
	})

	t.Run("an installed type is not reinstalled, and a dry run installs nothing", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		// no InstallBundledType expectation: a call would fail the test
		fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")
		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", false, false)
		require.NoError(t, err)

		bare := newV2Fixture(t)
		bare.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)
		_, err = bare.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", true, false)
		require.NoError(t, err)
	})

	t.Run("an unknown type is a did-you-mean 400 at the op's type", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		fx.expectMutateTyped(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"tsak"}`), "", false, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "task")
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefListTypes(testSpaceId))
	})

	t.Run("template is not a type an object can become", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutateTyped(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"template"}`), "", false, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.NewRef(v2model.OpCreateTemplate, "space_id", testSpaceId))
	})

	t.Run("a layout the object cannot convert to is refused with the other types it can take", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		fx.expectMutateTyped(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"shelf"}`), "", false, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		assert.Contains(t, apiErr.Message, "basic")
		assert.Contains(t, apiErr.Message, "collection")
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "task", "the hint names types whose layout the object can take")
		assert.NotContains(t, apiErr.Issues[0].Hint, "shelf")
		assert.NotContains(t, apiErr.Issues[0].Hint, "page", "the type it already has is not an alternative")
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefListTypes(testSpaceId))
	})

	t.Run("the source layout is the object's resolved layout, not its type's default", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		read := editRead(t, editBaseDoc)
		// a page whose layout override made it a todo: the editor converts
		// FROM todo, so the refusal must say so — on a dry run as well,
		// where the state is rebuilt from this read
		read.Snapshot.Details.Fields[bundle.RelationKeyResolvedLayout.String()] = pbtypes.Int64(int64(model.ObjectType_todo))
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil)

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"shelf"}`), "", true, false)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "todo layout")
		assert.NotContains(t, apiErr.Message, "basic layout")
	})

	t.Run("two type changes in one batch each convert from the previous one's layout", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		rec := fx.expectMutateTyped(editRead(t, editBaseDoc), "headB")

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`, `{"op":"set_type","type":"page"}`), "", false, false)

		require.NoError(t, err)
		assert.Equal(t, []domain.TypeKey{bundle.TypeKeyTask, bundle.TypeKeyPage}, rec.keys)
		assert.Equal(t, []model.ObjectTypeLayout{model.ObjectType_basic, model.ObjectType_todo}, rec.layouts,
			"the second conversion must start from the layout the first one produced")
		assert.Equal(t, &v2model.TypeChange{From: "page", To: "page"}, result.TypeChanged)
	})

	t.Run("a type-change-restricted object is a permanent 403 at the op", func(t *testing.T) {
		fx := newV2Fixture(t)
		read := editRead(t, editSetDoc)
		read.TypeChangeRefused = typeChangeRefusedProduction()
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(read, nil).Maybe()
		// no mutator expectation: the refusal must come from the read verdict

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"name":"Still fine"}}`, `{"op":"set_type","type":"collection"}`), "", false, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusForbidden, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "/ops/1", apiErr.Issues[0].Path, "the op that needs the axis, not the batch")
		assert.Contains(t, apiErr.Issues[0].Hint, "do not retry")
	})

	t.Run("set_properties with a type key steers to set_type", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutateTyped(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","set":{"type":"task"}}`), "", false, false)

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.NotEmpty(t, apiErr.Issues)
		assert.Equal(t, "ops[0].set.type", apiErr.Issues[0].Path)
		assert.Contains(t, apiErr.Issues[0].Hint, "set_type")
		assert.Contains(t, apiErr.Issues[0].SeeAlso, v2model.RefGetOpSchema("set_type"))
	})

	t.Run("unsetting the type is refused as such, without a set_type recipe", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.expectMutateTyped(editRead(t, editBaseDoc))

		_, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_properties","unset":["type"]}`), "", false, false)

		apiErr := v2Err(t, err)
		require.NotEmpty(t, apiErr.Issues)
		assert.Contains(t, apiErr.Issues[0].Message, "cannot be unset")
		assert.NotContains(t, apiErr.Issues[0].Hint, `"op":"set_type"`)
	})

	t.Run("a dry run validates the change and says the conversion is not simulated", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addLayoutTypes(t)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, testSpaceId, "obj1").Return(editRead(t, editBaseDoc), nil)
		// no mutator expectation: a dry run never commits

		result, err := fx.PatchObject(ctx, testSpaceId, "obj1",
			patchBody(`{"op":"set_type","type":"task"}`), "", true, false)

		require.NoError(t, err)
		assert.True(t, result.DryRun)
		assert.Equal(t, &v2model.TypeChange{From: "page", To: "task"}, result.TypeChanged)
		require.Len(t, result.Warnings, 1)
		assert.Equal(t, "ops[0]", result.Warnings[0].Path)
		assert.Contains(t, result.Warnings[0].Message, "not simulated")
	})

	t.Run("the op is served with its schema and about sentence", func(t *testing.T) {
		fx := newV2Fixture(t)

		entry, err := fx.SchemaOp("set_type")

		require.NoError(t, err)
		assert.Contains(t, string(entry.Schema), `"type":{`)
		assert.Contains(t, string(entry.Schema), "basic, todo, note, profile and bookmark")
		assert.NotContains(t, string(entry.Schema), "set to collection", "unreachable through the API: queries refuse the change")
		assert.NotContains(t, string(entry.Schema), "done property", "the converter links the property; no value is served")
		assert.Contains(t, string(entry.Example), `"op":"set_type"`)
	})
}

var _ = types.Struct{}
