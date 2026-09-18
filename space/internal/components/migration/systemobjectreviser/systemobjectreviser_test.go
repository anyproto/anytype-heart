package systemobjectreviser

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	mock_space "github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

func TestMigration_Run(t *testing.T) {
	t.Run("migrate relations with different revisions", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		store.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeySpaceId:        domain.String("space1"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyId:             domain.String("id1"),
				bundle.RelationKeyIsHidden:       domain.Bool(true), // bundle = false
				bundle.RelationKeyRevision:       domain.Int64(1),   // bundle = 3
				bundle.RelationKeyUniqueKey:      domain.String(bundle.RelationKeyBacklinks.URL()),
				bundle.RelationKeySourceObject:   domain.String(bundle.RelationKeyBacklinks.BundledURL()),
			},
		})
		fixer := &Migration{}
		ctx := context.Background()
		log := logger.NewNamed("test")

		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space1").Maybe()
		spc.EXPECT().DoCtx(ctx, "id1", mock.Anything).Return(nil).Times(1)
		spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		})

		// when
		migrated, toMigrate, err := fixer.Run(ctx, log, store.SpaceIndex("space1"), spc)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 1, migrated)
		assert.Equal(t, 1, toMigrate)
	})
	t.Run("a space this account may only read is skipped after the first refusal", func(t *testing.T) {
		// given — three revisable relations in a space the ACL will not let
		// this account write
		store := objectstore.NewStoreFixture(t)
		var objects []objectstore.TestObject
		for i, key := range []domain.RelationKey{
			bundle.RelationKeyBacklinks, bundle.RelationKeyRelationKey, bundle.RelationKeyRelationOptionColor,
		} {
			objects = append(objects, objectstore.TestObject{
				bundle.RelationKeySpaceId:        domain.String("space1"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyId:             domain.String(fmt.Sprintf("id%d", i)),
				bundle.RelationKeyIsHidden:       domain.Bool(true),
				bundle.RelationKeyRevision:       domain.Int64(0),
				bundle.RelationKeyUniqueKey:      domain.String(key.URL()),
				bundle.RelationKeySourceObject:   domain.String(key.BundledURL()),
			})
		}
		store.AddObjects(t, "space1", objects)

		fixer := &Migration{}
		ctx := context.Background()
		log := logger.NewNamed("test")

		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space1").Maybe()
		spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		}).Maybe()
		// the ACL refuses every write, and the migration must ask exactly ONCE:
		// a refusal is the space's answer, not the object's, and each attempt
		// leaves the loaded document carrying a value that will never persist
		spc.EXPECT().DoCtx(ctx, mock.Anything, mock.Anything).
			Return(list.ErrInsufficientPermissions).Times(1)

		// when
		toMigrate, migrated, err := fixer.Run(ctx, log, store.SpaceIndex("space1"), spc)

		// then — reported as a skip, not a failure: a reader cannot revise
		// the system objects of a space they do not own
		assert.NoError(t, err)
		assert.Equal(t, 0, migrated)
		assert.Equal(t, 1, toMigrate, "the pass stops at the first refusal")
	})
}

func TestReviseSystemObject(t *testing.T) {
	ctx := context.Background()
	log := logger.NewNamed("test")

	t.Run("system object type is updated if revision is higher", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(bundle.MustGetType(bundle.TypeKeyFile).Revision - 1),
			bundle.RelationKeySourceObject: domain.String("_otfile"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-file"),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system object type is updated if no revision is set", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{ // bundle Audio type revision = 1
			bundle.RelationKeySourceObject: domain.String("_otaudio"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-audio"),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("custom object type is not updated", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyUniqueKey: domain.String("ot-kitty"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("non system bundled object type is updated", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeySourceObject: domain.String("_otcontact"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-contact"),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system object type with same revision is not updated", func(t *testing.T) {
		// given
		objectType := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(bundle.MustGetType(bundle.TypeKeyImage).Revision),
			bundle.RelationKeySourceObject: domain.String("_otimage"),
			bundle.RelationKeyUniqueKey:    domain.String("ot-image"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, objectType)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation is updated if revision is higher", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(bundle.MustGetRelation(bundle.RelationKeyGlobalName).Revision - 1),
			bundle.RelationKeySourceObject: domain.String("_brglobalName"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-globalName"),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		})

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("system relation is updated if no revision is set", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{ // relationMaxCount revision = 1
			bundle.RelationKeySourceObject: domain.String("_brrelationMaxCount"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-relationMaxCount"),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		})

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("custom relation is not updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyUniqueKey: domain.String("rel-custom"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("non system relation without newer bundle revision is not updated", func(t *testing.T) {
		// given bundle audioLyrics revision = 0, so the local object is already up to date
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(1),
			bundle.RelationKeySourceObject: domain.String("_braudioLyrics"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-audioLyrics"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("system relation with same revision is not updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:     domain.Int64(bundle.MustGetRelation(bundle.RelationKeyBacklinks).Revision),
			bundle.RelationKeySourceObject: domain.String("_brbacklinks"),
			bundle.RelationKeyUniqueKey:    domain.String("rel-backlinks"),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("relation with absent maxCount is updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:         domain.Int64(bundle.MustGetRelation(bundle.RelationKeyBacklinks).Revision - 1),
			bundle.RelationKeySourceObject:     domain.String("_brbacklinks"),
			bundle.RelationKeyUniqueKey:        domain.String("rel-backlinks"),
			bundle.RelationKeyRelationMaxCount: domain.Int64(1), // maxCount of bundle backlinks = 0
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		})

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("recommendedRelations list is updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:             domain.Int64(bundle.MustGetType(bundle.TypeKeyImage).Revision - 1),
			bundle.RelationKeySourceObject:         domain.String("_otimage"),
			bundle.RelationKeyUniqueKey:            domain.String("ot-image"),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{"rel-name"}),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.ObjectTypeKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})

	t.Run("relationFormatObjectTypes list is updated", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyRevision:                  domain.Int64(bundle.MustGetRelation(bundle.RelationKeyCreator).Revision - 1),
			bundle.RelationKeySourceObject:              domain.String("_brcreator"),
			bundle.RelationKeyUniqueKey:                 domain.String("rel-creator"),
			bundle.RelationKeyRelationFormatObjectTypes: domain.StringList([]string{}),
		})
		space := mock_space.NewMockSpace(t)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Times(1).Return(nil)
		space.EXPECT().Id().Times(1).Return("")
		space.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return addr.RelationKeyToIdPrefix + key.InternalKey(), nil
		}).Maybe()

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.True(t, toRevise)
	})
}

func TestBuildDiffDetails(t *testing.T) {
	t.Run("new name is applied to system types", func(t *testing.T) {
		diff := buildDiffDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyPluralName: domain.String("Pages"),
			bundle.RelationKeyName:       domain.String("Page"),
		}), domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Page"),
		}), domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, "page"), true)

		assert.Equal(t, "Pages", diff.GetString(bundle.RelationKeyPluralName))
	})

	t.Run("new name is applied to custom types, if name was not modified", func(t *testing.T) {
		diff := buildDiffDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyPluralName: domain.String("Projects"),
			bundle.RelationKeyName:       domain.String("Project"),
		}), domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Project"),
		}), domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, "project"), false)

		assert.Equal(t, "Projects", diff.GetString(bundle.RelationKeyPluralName))
	})

	t.Run("new name is NOT applied to custom types, if name was modified", func(t *testing.T) {
		diff := buildDiffDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyPluralName: domain.String("Projects"),
			bundle.RelationKeyName:       domain.String("Project"),
		}), domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Проект"),
		}), domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, "project"), false)

		assert.False(t, diff.Has(bundle.RelationKeyPluralName))
		assert.False(t, diff.Has(bundle.RelationKeyName))
	})
}

func TestReviseNonSystemBundledRelation(t *testing.T) {
	ctx := context.Background()
	log := logger.NewNamed("test")

	newSpaceApplyingTo := func(t *testing.T, sb *smarttest.SmartTest) *mock_space.MockSpace {
		spc := mock_space.NewMockSpace(t)
		spc.EXPECT().Id().Return("space1").Maybe()
		spc.EXPECT().DeriveObjectID(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, key domain.UniqueKey) (string, error) {
			return key.Marshal(), nil
		}).Maybe()
		spc.EXPECT().DoCtx(mock.Anything, sb.Id(), mock.Anything).RunAndReturn(
			func(_ context.Context, _ string, apply func(smartblock.SmartBlock) error) error {
				return apply(sb)
			}).Times(1)
		return spc
	}

	t.Run("relation still carrying the previous bundled name gets the new bundled name", func(t *testing.T) {
		// given bundle audioGenre was renamed "Genre" -> "Audio genre" with revision 1
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:           domain.String("audioGenreId"),
			bundle.RelationKeyName:         domain.String("Genre"),
			bundle.RelationKeySourceObject: domain.String(bundle.RelationKeyAudioGenre.BundledURL()),
			bundle.RelationKeyUniqueKey:    domain.String(bundle.RelationKeyAudioGenre.URL()),
		})
		sb := smarttest.New("audioGenreId")
		sb.Doc.(*state.State).SetDetail(bundle.RelationKeyName, domain.String("Genre"))
		space := newSpaceApplyingTo(t, sb)
		want := map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:     domain.String("Audio genre"),
			bundle.RelationKeyRevision: domain.Int64(bundle.MustGetRelation(bundle.RelationKeyAudioGenre).Revision),
		}

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		require.NoError(t, err)
		assert.True(t, toRevise)
		for key, value := range want {
			assert.Equal(t, value, sb.Details().Get(key), key)
		}
		// nothing beyond name and revision is applied to a non-system relation
		assert.False(t, sb.Details().Has(bundle.RelationKeyRecommendedFeaturedRelations))
	})

	t.Run("relation renamed by the user keeps the user's name", func(t *testing.T) {
		// given the local name matches neither the previous nor the current bundled name
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:           domain.String("audioGenreId"),
			bundle.RelationKeyName:         domain.String("My genre"),
			bundle.RelationKeySourceObject: domain.String(bundle.RelationKeyAudioGenre.BundledURL()),
			bundle.RelationKeyUniqueKey:    domain.String(bundle.RelationKeyAudioGenre.URL()),
		})
		sb := smarttest.New("audioGenreId")
		sb.Doc.(*state.State).SetDetail(bundle.RelationKeyName, domain.String("My genre"))
		space := newSpaceApplyingTo(t, sb)
		want := map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:     domain.String("My genre"),
			bundle.RelationKeyRevision: domain.Int64(bundle.MustGetRelation(bundle.RelationKeyAudioGenre).Revision),
		}

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		require.NoError(t, err)
		assert.True(t, toRevise)
		for key, value := range want {
			assert.Equal(t, value, sb.Details().Get(key), key)
		}
	})

	t.Run("relation with recorded bundle revision is not revised again", func(t *testing.T) {
		// given
		rel := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:           domain.String("audioGenreId"),
			bundle.RelationKeyName:         domain.String("My genre"),
			bundle.RelationKeyRevision:     domain.Int64(bundle.MustGetRelation(bundle.RelationKeyAudioGenre).Revision),
			bundle.RelationKeySourceObject: domain.String(bundle.RelationKeyAudioGenre.BundledURL()),
			bundle.RelationKeyUniqueKey:    domain.String(bundle.RelationKeyAudioGenre.URL()),
		})
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		// when
		toRevise, err := reviseObject(ctx, log, space, rel)

		// then
		assert.NoError(t, err)
		assert.False(t, toRevise)
	})

	t.Run("bundled name is applied only via the previous-names table", func(t *testing.T) {
		assert.True(t, canApplyBundledRelationName(bundle.RelationKeyAudioGenre, "Genre"))
		assert.True(t, canApplyBundledRelationName(bundle.RelationKeyAudioGenre, ""))
		assert.False(t, canApplyBundledRelationName(bundle.RelationKeyAudioGenre, "Genre 2"))
		assert.True(t, canApplyBundledRelationName(bundle.RelationKeyHeaderRelationsLayout, "Header relations layout"))
		// a relation with no recorded rename history never gets its name overwritten
		assert.False(t, canApplyBundledRelationName(bundle.RelationKeyAudioLyrics, "Lyrics"))
	})

	t.Run("previous-names table agrees with the bundle", func(t *testing.T) {
		for key, previousNames := range previousBundledRelationNames {
			relation, err := bundle.GetRelation(key)
			require.NoError(t, err, key)
			// without a revision bump the rename never reaches existing spaces
			assert.GreaterOrEqual(t, relation.Revision, int64(1), key)
			// the table is only for non-system relations: the system path applies names unconditionally
			assert.False(t, bundle.IsSystemRelation(key), key)
			assert.NotContains(t, previousNames, relation.Name, key)
		}
	})

	t.Run("only revision and name are revisable on non-system relations", func(t *testing.T) {
		// given a local object diverging from the bundle in name, hidden flag and readonly value
		diff := buildDiffDetails(
			(&relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyAudioGenre)}).ToDetails(),
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:                  domain.String("Genre"),
				bundle.RelationKeyIsHidden:              domain.Bool(true),
				bundle.RelationKeyRelationReadonlyValue: domain.Bool(true),
			}),
			domain.MustUniqueKey(coresb.SmartBlockTypeRelation, bundle.RelationKeyAudioGenre.String()),
			false)

		// then only name and revision made it into the diff
		assert.Equal(t, "Audio genre", diff.GetString(bundle.RelationKeyName))
		assert.Equal(t, bundle.MustGetRelation(bundle.RelationKeyAudioGenre).Revision, diff.GetInt64(bundle.RelationKeyRevision))
		assert.False(t, diff.Has(bundle.RelationKeyIsHidden))
		assert.False(t, diff.Has(bundle.RelationKeyRelationReadonlyValue))
	})
}
