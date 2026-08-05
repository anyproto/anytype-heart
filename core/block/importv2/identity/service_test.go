package identity

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

const spaceId = "spaceId"

type fixture struct {
	*Service
	store *objectstore.StoreFixture
	space *mock_clientspace.MockSpace
}

func newFixture(t *testing.T, updateExisting bool) *fixture {
	store := objectstore.NewStoreFixture(t)
	space := mock_clientspace.NewMockSpace(t)
	return &fixture{
		Service: NewService(space, store.SpaceIndex(spaceId), updateExisting, time.Unix(1700000000, 0)),
		store:   store,
		space:   space,
	}
}

func payloadWithId(id string) treestorage.TreeStorageCreatePayload {
	return treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: id},
	}
}

func relationObject(sourceKey, key, name string, format model.RelationFormat) *importv2.Object {
	return &importv2.Object{
		SourceKey: sourceKey,
		SbType:    coresb.SmartBlockTypeRelation,
		Payload: &importv2.Snapshot{
			Key: key,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyName:           domain.String(name),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(format)),
				bundle.RelationKeyRelationKey:    domain.String(key),
			}),
		},
	}
}

func TestClaim(t *testing.T) {
	t.Run("new object mints a tree payload, taken once by Assign", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.space.EXPECT().CreateTreePayload(mock.Anything, payloadcreator.PayloadCreationParams{
			Time:           time.Unix(1700000000, 0),
			SmartblockType: coresb.SmartBlockTypePage,
		}).Return(payloadWithId("tree1"), nil)
		want := Assignment{Id: "tree1", Payload: payloadWithId("tree1")}

		// when
		err := fx.Claim(context.Background(), importv2.IdentityClaim{
			SourceKey: "docs/page.md", SbType: coresb.SmartBlockTypePage, SourceFilePath: "docs/page.md",
		})
		require.NoError(t, err)
		got, assignErr := fx.Assign("docs/page.md")

		// then
		require.NoError(t, assignErr)
		assert.Equal(t, want, got)
		id, ok := fx.Resolve("docs/page.md")
		assert.True(t, ok)
		assert.Equal(t, "tree1", id)
	})

	t.Run("matches by oldAnytypeID without updateExisting", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:           domain.String("existing1"),
			bundle.RelationKeyOldAnytypeID: domain.String("old1"),
		}})
		want := Assignment{Id: "existing1", IsExisting: true}

		// when
		err := fx.Claim(context.Background(), importv2.IdentityClaim{
			SourceKey: "k", SbType: coresb.SmartBlockTypePage, OldAnytypeID: "old1",
		})
		require.NoError(t, err)
		got, assignErr := fx.Assign("k")

		// then
		require.NoError(t, assignErr)
		assert.Equal(t, want, got)
	})

	t.Run("sourceFilePath match is gated by updateExisting", func(t *testing.T) {
		for _, tc := range []struct {
			name           string
			updateExisting bool
			wantExisting   bool
		}{
			{"enabled matches", true, true},
			{"disabled mints", false, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// given
				fx := newFixture(t, tc.updateExisting)
				fx.store.AddObjects(t, spaceId, []objectstore.TestObject{{
					bundle.RelationKeyId:             domain.String("existing1"),
					bundle.RelationKeySourceFilePath: domain.String("docs/page.md"),
				}})
				if !tc.wantExisting {
					fx.space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).Return(payloadWithId("tree1"), nil)
				}

				// when
				err := fx.Claim(context.Background(), importv2.IdentityClaim{
					SourceKey: "docs/page.md", SbType: coresb.SmartBlockTypePage, SourceFilePath: "docs/page.md",
				})
				require.NoError(t, err)
				got, assignErr := fx.Assign("docs/page.md")

				// then
				require.NoError(t, assignErr)
				assert.Equal(t, tc.wantExisting, got.IsExisting)
			})
		}
	})

	t.Run("duplicate source key is rejected", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).Return(payloadWithId("tree1"), nil).Once()
		claim := importv2.IdentityClaim{SourceKey: "k", SbType: coresb.SmartBlockTypePage}
		require.NoError(t, fx.Claim(context.Background(), claim))

		// when
		err := fx.Claim(context.Background(), claim)

		// then
		assert.ErrorContains(t, err, "duplicate source key")
	})
}

func TestAssignDerived(t *testing.T) {
	t.Run("derives a new keyed tree when nothing matches", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, "myrel")
		require.NoError(t, err)
		fx.space.EXPECT().DeriveTreePayload(mock.Anything, payloadcreator.PayloadDerivationParams{Key: uniqueKey}).
			Return(payloadWithId("derived1"), nil)
		want := Assignment{Id: "derived1", Payload: payloadWithId("derived1"), InternalKey: "myrel"}

		// when
		got, err := fx.AssignDerived(context.Background(), relationObject("rel:myrel", "myrel", "My Relation", model.RelationFormat_longtext))

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
		id, ok := fx.Resolve("rel:myrel")
		assert.True(t, ok)
		assert.Equal(t, "derived1", id)
	})

	t.Run("matches existing relation by name and format", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("existingRel"),
			bundle.RelationKeyName:           domain.String("My Relation"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_longtext)),
			bundle.RelationKeyRelationKey:    domain.String("otherkey"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyUniqueKey:      domain.String("rel-existingkey"),
		}})
		want := Assignment{Id: "existingRel", IsExisting: true, InternalKey: "existingkey"}

		// when
		got, err := fx.AssignDerived(context.Background(), relationObject("rel:myrel", "myrel", "My Relation", model.RelationFormat_longtext))

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("repeated definition memoizes to one object per run", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.space.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).Return(payloadWithId("derived1"), nil).Once()
		first, err := fx.AssignDerived(context.Background(), relationObject("rel:a", "myrel", "A", model.RelationFormat_longtext))
		require.NoError(t, err)
		require.False(t, first.IsExisting)

		// when
		second, err := fx.AssignDerived(context.Background(), relationObject("rel:b", "myrel", "A", model.RelationFormat_longtext))

		// then
		require.NoError(t, err)
		assert.True(t, second.IsExisting)
		assert.Equal(t, first.Id, second.Id)
		assert.Nil(t, second.Payload.RootRawChange)
	})

	t.Run("deleted keyed object gets a fresh key instead of resurrection", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, "myrel")
		require.NoError(t, err)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:        domain.String("deleted1"),
			bundle.RelationKeyUniqueKey: domain.String(uniqueKey.Marshal()),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}})
		var derivedWith domain.UniqueKey
		fx.space.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).
			Run(func(_ context.Context, params payloadcreator.PayloadDerivationParams) {
				derivedWith = params.Key
			}).
			Return(payloadWithId("fresh1"), nil)

		// when
		got, err := fx.AssignDerived(context.Background(), relationObject("rel:myrel", "myrel", "My Relation", model.RelationFormat_longtext))

		// then
		require.NoError(t, err)
		assert.Equal(t, "fresh1", got.Id)
		require.NotNil(t, derivedWith)
		assert.NotEqual(t, "myrel", derivedWith.InternalKey())
		assert.Equal(t, derivedWith.InternalKey(), got.InternalKey)
	})
}

func TestFileFutures(t *testing.T) {
	t.Run("resolve waits for completion", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.RegisterFile("docs/img.png")
		_, ok := fx.Resolve("docs/img.png")
		assert.False(t, ok, "unresolved file must not resolve synchronously")

		done := make(chan struct{})
		var gotId string
		var gotErr error
		go func() {
			defer close(done)
			gotId, gotErr = fx.ResolveFile(context.Background(), "docs/img.png")
		}()

		// when
		fx.CompleteFile("docs/img.png", "fileObj1", nil)
		<-done

		// then
		require.NoError(t, gotErr)
		assert.Equal(t, "fileObj1", gotId)
		id, ok := fx.Resolve("docs/img.png")
		assert.True(t, ok)
		assert.Equal(t, "fileObj1", id)
	})

	t.Run("upload failure propagates to waiting references", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.RegisterFile("docs/img.png")
		fx.CompleteFile("docs/img.png", "", assert.AnError)

		// when
		_, err := fx.ResolveFile(context.Background(), "docs/img.png")

		// then
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("use before definition is an invariant issue, not a hang", func(t *testing.T) {
		// given
		fx := newFixture(t, false)

		// when
		_, err := fx.ResolveFile(context.Background(), "never-registered.png")

		// then
		issue := importv2.AsIssue(err, importv2.SeverityFatal, importv2.IssueStoreError)
		assert.Equal(t, importv2.IssueInvariant, issue.Code)
	})

	t.Run("cancelled context unblocks the wait", func(t *testing.T) {
		// given
		fx := newFixture(t, false)
		fx.RegisterFile("docs/img.png")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// when
		_, err := fx.ResolveFile(ctx, "docs/img.png")

		// then
		assert.ErrorIs(t, err, context.Canceled)
	})
}

// TestDerivedAdoptsClaimedSourceKey pins the interaction the notion importer
// relies on when a minted type takes its single database's place: the type is
// a derived-class object carrying a source key that pass 1 already claimed.
func TestDerivedAdoptsClaimedSourceKey(t *testing.T) {
	t.Run("references resolve to the derived object and the claim is not reported missing", func(t *testing.T) {
		// given — pass 1 claimed the database id
		fx := newFixture(t, false)
		fx.space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).Return(payloadWithId("minted1"), nil)
		require.NoError(t, fx.Claim(context.Background(), importv2.IdentityClaim{
			SourceKey: "db1", SbType: coresb.SmartBlockTypePage,
		}))
		require.Equal(t, []string{"db1"}, fx.UnassignedClaims())

		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, "dbtype")
		require.NoError(t, err)
		fx.space.EXPECT().DeriveTreePayload(mock.Anything, payloadcreator.PayloadDerivationParams{Key: uniqueKey}).
			Return(payloadWithId("derived1"), nil)

		// when — pass 2 emits a derived object under that same source key
		got, err := fx.AssignDerived(context.Background(), relationObject("db1", "dbtype", "Db Type", model.RelationFormat_longtext))

		// then — links to the database resolve to the derived object
		require.NoError(t, err)
		assert.Equal(t, "derived1", got.Id)
		id, ok := fx.Resolve("db1")
		require.True(t, ok)
		assert.Equal(t, "derived1", id, "references to the database must follow it to the type")

		// and the claim is satisfied, not reported as a silent gap
		assert.Empty(t, fx.UnassignedClaims(),
			"a claim answered by a derived object must not surface as an invariant error")
	})
}
