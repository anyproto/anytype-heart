package objectid

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

func TestDerivedObject_GetIDAndPayload(t *testing.T) {
	t.Run("try to recreate deleted object", func(t *testing.T) {
		// given
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		deriveObject := newDerivedObject(newExistingObject(sf), service, sf)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyUniqueKey: domain.String("key"),
					}),
					Key: "oldKey",
				},
				SbType: coresb.SmartBlockTypePage,
			},
		}
		space := mock_clientspace.NewMockSpace(t)
		service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		space.EXPECT().DeriveTreePayload(context.Background(), mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "newId"},
		}, nil)

		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, "oldKey")
		assert.Nil(t, err)
		sf.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyUniqueKey: domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyId:        domain.String("oldId"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})

		// when
		id, _, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.NotEqual(t, deriveObject.GetInternalKey(sn.Snapshot.SbType), "key")
		assert.Equal(t, "newId", id)
	})
	t.Run("existing object", func(t *testing.T) {
		// given
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		deriveObject := newDerivedObject(newExistingObject(sf), service, sf)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyName:           domain.String("IMPORTED NAME"),
						bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
						bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
					}),
				},
				SbType: coresb.SmartBlockTypeRelation,
			},
		}

		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, "oldKey")
		assert.Nil(t, err)
		sf.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyId:             domain.String("oldId"),
				bundle.RelationKeyName:           domain.String("name"),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String("spaceId"),
			},
		})

		// when
		id, _, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "oldId", id)
	})
}

func TestDerivedObject_GetIDAndPayload_ObjectType(t *testing.T) {
	const bundledProjectId = "bundledProjectId"

	// newObjectTypeSnapshot builds an imported object type snapshot with the given unique key
	// and name; an empty unique key stands for a legacy export that predates unique keys.
	newObjectTypeSnapshot := func(uniqueKey, name string) *common.Snapshot {
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String(name),
		})
		if uniqueKey != "" {
			details.SetString(bundle.RelationKeyUniqueKey, uniqueKey)
		}
		return &common.Snapshot{
			Id: "type-project",
			Snapshot: &common.SnapshotModel{
				SbType: coresb.SmartBlockTypeObjectType,
				Data:   &common.StateSnapshot{Key: "pmProject", Details: details},
			},
		}
	}

	// newFixture returns a store already holding the bundled Project type, as every space does.
	newFixture := func(t *testing.T) (*objectstore.StoreFixture, *derivedObject) {
		sf := objectstore.NewStoreFixture(t)
		sf.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(bundledProjectId),
				bundle.RelationKeyUniqueKey:      domain.String("ot-project"),
				bundle.RelationKeyName:           domain.String("Project"),
				bundle.RelationKeySourceObject:   domain.String("_otproject"),
				bundle.RelationKeyRevision:       domain.Int64(3),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String("spaceId"),
			},
		})
		service := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		service.EXPECT().Get(mock.Anything, "spaceId").Return(space, nil).Maybe()
		space.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "freshTypeId"},
		}, nil).Maybe()
		return sf, newDerivedObject(newExistingObject(sf), service, sf)
	}

	t.Run("type with own unique key is not merged into a same-named type", func(t *testing.T) {
		// given
		_, deriveObject := newFixture(t)
		sn := newObjectTypeSnapshot("ot-pmProject", "Project")

		// when
		id, payload, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "freshTypeId", id)
		assert.NotNil(t, payload.RootRawChange, "a new type must be created, not merged into the bundled one")
	})

	t.Run("type is merged into an existing type with the same unique key", func(t *testing.T) {
		// given
		_, deriveObject := newFixture(t)
		sn := newObjectTypeSnapshot("ot-project", "Project renamed by the user")

		// when
		id, payload, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundledProjectId, id)
		assert.Nil(t, payload.RootRawChange)
	})

	t.Run("legacy type without unique key is merged by name", func(t *testing.T) {
		// given
		_, deriveObject := newFixture(t)
		sn := newObjectTypeSnapshot("", "Project")

		// when
		id, payload, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, bundledProjectId, id)
		assert.Nil(t, payload.RootRawChange)
	})

	t.Run("type with neither unique key nor name gets a new id", func(t *testing.T) {
		// given
		_, deriveObject := newFixture(t)
		sn := newObjectTypeSnapshot("", "")

		// when
		id, payload, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "freshTypeId", id)
		assert.NotNil(t, payload.RootRawChange)
	})
}

func TestDerivedObject_GetIDAndPayload_ChatDerived(t *testing.T) {
	// newChatSnapshot builds the snapshot cmd/anyblockconvert emits for a
	// `kind: "chat"` document: the envelope's key on Data.Key, and no
	// uniqueKey detail — an authored bundle has no id to carry one.
	newChatSnapshot := func(uniqueKey string) *common.Snapshot {
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName: domain.String("Wiki requests"),
		})
		if uniqueKey != "" {
			details.SetString(bundle.RelationKeyUniqueKey, uniqueKey)
		}
		return &common.Snapshot{
			Id: "chat-wiki-requests",
			Snapshot: &common.SnapshotModel{
				SbType: coresb.SmartBlockTypeChatDerivedObject,
				Data:   &common.StateSnapshot{Key: "wikiRequests", Details: details},
			},
		}
	}

	newFixture := func(t *testing.T) (*objectstore.StoreFixture, *derivedObject) {
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		service.EXPECT().Get(mock.Anything, "spaceId").Return(space, nil).Maybe()
		space.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "derivedChatId"},
		}, nil).Maybe()
		return sf, newDerivedObject(newExistingObject(sf), service, sf)
	}

	t.Run("chat is derived from the key its document carries", func(t *testing.T) {
		// given
		_, deriveObject := newFixture(t)
		sn := newChatSnapshot("")

		// when
		id, payload, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "derivedChatId", id)
		assert.NotNil(t, payload.RootRawChange)
	})

	// What keeps a reinstalled bundle from growing a second copy of every chat
	// is the derivation key, not a store lookup: existingObject resolves a
	// uniqueKey only for relations, options and types, so a chat always reaches
	// DeriveTreePayload — and the same key derives the same id. Pin the key.
	t.Run("derivation is keyed on the chat's own unique key", func(t *testing.T) {
		// given
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		service.EXPECT().Get(mock.Anything, "spaceId").Return(space, nil).Maybe()

		var derivedWith string
		space.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, params payloadcreator.PayloadDerivationParams) (treestorage.TreeStorageCreatePayload, error) {
				derivedWith = params.Key.Marshal()
				return treestorage.TreeStorageCreatePayload{
					RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "derivedChatId"},
				}, nil
			}).Maybe()
		deriveObject := newDerivedObject(newExistingObject(sf), service, sf)

		// when — the document carries only its key, as an authored bundle does
		id, _, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", newChatSnapshot(""), time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "derivedChatId", id)
		assert.Equal(t, "chatDerived-wikiRequests", derivedWith)
	})
}

func TestProvider_ChatDerivedObjectIsImportable(t *testing.T) {
	// The regression this guards: with no provider registered for the type,
	// GetIDAndPayload fell through to "unsupported smartblock to import" and
	// the whole archive failed — one chat document was enough to sink a
	// 60-object bundle, and the error named neither the type nor the document.
	sf := objectstore.NewStoreFixture(t)
	service := mock_space.NewMockService(t)
	space := mock_clientspace.NewMockSpace(t)
	service.EXPECT().Get(mock.Anything, "spaceId").Return(space, nil).Maybe()
	space.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "derivedChatId"},
	}, nil).Maybe()

	// built through the real constructor: the point of the test is that the
	// wiring in NewIDProvider knows the type, so registering one by hand here
	// would assert nothing
	p := NewIDProvider(sf, service, nil, nil)

	sn := &common.Snapshot{
		Id: "chat-wiki-requests",
		Snapshot: &common.SnapshotModel{
			SbType: coresb.SmartBlockTypeChatDerivedObject,
			Data: &common.StateSnapshot{
				Key:     "wikiRequests",
				Details: domain.NewDetails(),
			},
		},
	}

	id, _, err := p.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

	assert.Nil(t, err)
	assert.Equal(t, "derivedChatId", id)
}
