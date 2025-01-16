package publish

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/anyproto/anytype-publish-server/publishclient/publishapi"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	files2 "github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/core/identity/mock_identity"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/core/inviteservice/mock_inviteservice"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const (
	spaceId  = "spaceId"
	objectId = "objectId"
	id       = "identity"
)

type mockPublishClient struct {
	t               *testing.T
	expectedUrl     string
	expectedErr     error
	expectedRequest *publishapi.PublishRequest
	expectedSpace   string
	expectedInvite  string
	expectedObject  string
	expectedPbFiles map[string]struct{}
}

func (m *mockPublishClient) Init(a *app.App) (err error) {
	return
}

func (m *mockPublishClient) Name() (name string) {
	return ""
}

func (m *mockPublishClient) ResolveUri(ctx context.Context, uri string) (publish *publishapi.Publish, err error) {
	return
}

func (m *mockPublishClient) GetPublishStatus(ctx context.Context, spaceId, objectId string) (publish *publishapi.Publish, err error) {
	return
}

func (m *mockPublishClient) Publish(ctx context.Context, req *publishapi.PublishRequest) (uploadUrl string, err error) {
	m.expectedRequest = req
	return m.expectedUrl, m.expectedErr
}

func (m *mockPublishClient) UnPublish(ctx context.Context, req *publishapi.UnPublishRequest) (err error) {
	return
}

func (m *mockPublishClient) ListPublishes(ctx context.Context, spaceId string) (publishes []*publishapi.Publish, err error) {
	return
}

func (m *mockPublishClient) UploadDir(ctx context.Context, uploadUrl, dir string) (err error) {
	assert.NoError(m.t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			m.checkIndexFile(path, info)
		}
		return nil
	}))
	return
}

func (m *mockPublishClient) checkIndexFile(path string, info os.FileInfo) {
	assert.Equal(m.t, info.Name(), indexFileName)
	file, err := os.Open(path)
	assert.NoError(m.t, err)
	defer file.Close()
	reader, err := gzip.NewReader(file)
	assert.NoError(m.t, err)
	defer reader.Close()
	fileContent, err := ioutil.ReadAll(reader)
	assert.NoError(m.t, err)
	uberSnapshot := &PublishingUberSnapshot{}
	err = json.Unmarshal(fileContent, uberSnapshot)
	assert.NoError(m.t, err)
	assert.Equal(m.t, m.expectedInvite, uberSnapshot.Meta.InviteLink)
	assert.Equal(m.t, m.expectedSpace, uberSnapshot.Meta.SpaceId)
	assert.Equal(m.t, m.expectedObject, uberSnapshot.Meta.RootPageId)
	for fileName := range m.expectedPbFiles {
		_, ok := uberSnapshot.PbFiles[fileName]
		assert.True(m.t, ok)
	}
}

func TestPublish(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		isPersonal := true
		spaceService, err := prepaeSpaceService(t, isPersonal)

		objectTypeId := "customObjectType"
		expectedUri := "test"
		expected := fmt.Sprintf(defaultUrlTemplate, id) + "/" + expectedUri
		publishClient := &mockPublishClient{
			t:              t,
			expectedUrl:    expected,
			expectedObject: objectId,
			expectedInvite: "",
			expectedSpace:  spaceId,
			expectedPbFiles: map[string]struct{}{
				filepath.Join("objects", objectId+".pb"):   {},
				filepath.Join("types", objectTypeId+".pb"): {},
			},
		}

		identityService := mock_identity.NewMockService(t)
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{}))

		exp := prepareExporter(t, objectTypeId, spaceService)

		svc := &service{
			spaceService:         spaceService,
			exportService:        exp,
			publishClientService: publishClient,
			identityService:      identityService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, false)

		// then
		assert.NoError(t, err)
		assert.Equal(t, expected, publish.Cid)
		assert.Equal(t, "{\"heads\":[\"heads\"]}", publishClient.expectedRequest.Version)
		assert.Equal(t, objectId, publishClient.expectedRequest.ObjectId)
		assert.Equal(t, spaceId, publishClient.expectedRequest.SpaceId)
		assert.Equal(t, expectedUri, publishClient.expectedRequest.Uri)
	})
	t.Run("success with space sharing", func(t *testing.T) {
		// given
		isPersonal := false
		spaceService, err := prepaeSpaceService(t, isPersonal)

		objectTypeId := "customObjectType"
		expectedUri := "test"
		expected := fmt.Sprintf(defaultUrlTemplate, id) + "/" + expectedUri

		inviteFileCid := "inviteFileCid"
		inviteFileKey := "inviteFileKey"
		expectedInvite := fmt.Sprintf(inviteLinkUrlTemplate, inviteFileCid, inviteFileKey)
		publishClient := &mockPublishClient{
			t:              t,
			expectedUrl:    expected,
			expectedObject: objectId,
			expectedInvite: expectedInvite,
			expectedSpace:  spaceId,
			expectedPbFiles: map[string]struct{}{
				filepath.Join("objects", objectId+".pb"):   {},
				filepath.Join("types", objectTypeId+".pb"): {},
			},
		}

		identityService := mock_identity.NewMockService(t)
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{}))

		exp := prepareExporter(t, objectTypeId, spaceService)

		inviteService := mock_inviteservice.NewMockInviteService(t)
		inviteService.EXPECT().GetCurrent(context.Background(), "spaceId").Return(domain.InviteInfo{
			InviteFileCid: inviteFileCid,
			InviteFileKey: inviteFileKey,
		}, nil)

		svc := &service{
			spaceService:         spaceService,
			exportService:        exp,
			publishClientService: publishClient,
			identityService:      identityService,
			inviteService:        inviteService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, true)

		// then
		assert.NoError(t, err)
		assert.Equal(t, expected, publish.Cid)
		assert.Equal(t, "{\"heads\":[\"heads\"]}", publishClient.expectedRequest.Version)
		assert.Equal(t, objectId, publishClient.expectedRequest.ObjectId)
		assert.Equal(t, spaceId, publishClient.expectedRequest.SpaceId)
		assert.Equal(t, expectedUri, publishClient.expectedRequest.Uri)
	})
	t.Run("success with space sharing - invite not exists", func(t *testing.T) {
		isPersonal := false
		spaceService, err := prepaeSpaceService(t, isPersonal)

		objectTypeId := "customObjectType"
		expectedUri := "test"
		expected := fmt.Sprintf(defaultUrlTemplate, id) + "/" + expectedUri

		publishClient := &mockPublishClient{
			t:              t,
			expectedUrl:    expected,
			expectedObject: objectId,
			expectedInvite: "",
			expectedSpace:  spaceId,
			expectedPbFiles: map[string]struct{}{
				filepath.Join("objects", objectId+".pb"):   {},
				filepath.Join("types", objectTypeId+".pb"): {},
			},
		}

		identityService := mock_identity.NewMockService(t)
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{}))

		exp := prepareExporter(t, objectTypeId, spaceService)

		inviteService := mock_inviteservice.NewMockInviteService(t)
		inviteService.EXPECT().GetCurrent(context.Background(), "spaceId").Return(domain.InviteInfo{}, inviteservice.ErrInviteNotExists)

		svc := &service{
			spaceService:         spaceService,
			exportService:        exp,
			publishClientService: publishClient,
			identityService:      identityService,
			inviteService:        inviteService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, true)

		// then
		assert.NoError(t, err)
		assert.Equal(t, expected, publish.Cid)
		assert.Equal(t, "{\"heads\":[\"heads\"]}", publishClient.expectedRequest.Version)
		assert.Equal(t, objectId, publishClient.expectedRequest.ObjectId)
		assert.Equal(t, spaceId, publishClient.expectedRequest.SpaceId)
		assert.Equal(t, expectedUri, publishClient.expectedRequest.Uri)
	})
	t.Run("success for member", func(t *testing.T) {
		// given
		isPersonal := false
		spaceService, err := prepaeSpaceService(t, isPersonal)

		objectTypeId := "customObjectType"
		expectedUri := "test"

		identityService := mock_identity.NewMockService(t)
		globalName := "name"
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyGlobalName: domain.String(globalName),
		}))
		expectedUrl := fmt.Sprintf(memberUrlTemplate, globalName) + "/" + expectedUri

		inviteFileCid := "inviteFileCid"
		inviteFileKey := "inviteFileKey"
		expectedInvite := fmt.Sprintf(inviteLinkUrlTemplate, inviteFileCid, inviteFileKey)

		publishClient := &mockPublishClient{
			t:              t,
			expectedUrl:    expectedUrl,
			expectedObject: objectId,
			expectedInvite: expectedInvite,
			expectedSpace:  spaceId,
			expectedPbFiles: map[string]struct{}{
				filepath.Join("objects", objectId+".pb"):   {},
				filepath.Join("types", objectTypeId+".pb"): {},
			},
		}

		exp := prepareExporter(t, objectTypeId, spaceService)

		inviteService := mock_inviteservice.NewMockInviteService(t)
		inviteService.EXPECT().GetCurrent(context.Background(), "spaceId").Return(domain.InviteInfo{
			InviteFileCid: inviteFileCid,
			InviteFileKey: inviteFileKey,
		}, nil)

		svc := &service{
			spaceService:         spaceService,
			exportService:        exp,
			publishClientService: publishClient,
			identityService:      identityService,
			inviteService:        inviteService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, true)

		// then
		assert.NoError(t, err)
		assert.Equal(t, expectedUrl, publish.Cid)
		assert.Equal(t, "{\"heads\":[\"heads\"]}", publishClient.expectedRequest.Version)
		assert.Equal(t, objectId, publishClient.expectedRequest.ObjectId)
		assert.Equal(t, spaceId, publishClient.expectedRequest.SpaceId)
		assert.Equal(t, expectedUri, publishClient.expectedRequest.Uri)
	})
	t.Run("internal error", func(t *testing.T) {
		// given
		isPersonal := true
		spaceService, err := prepaeSpaceService(t, isPersonal)

		objectTypeId := "customObjectType"
		expectedUri := "test"

		identityService := mock_identity.NewMockService(t)
		globalName := "name"
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyGlobalName: domain.String(globalName),
		}))

		publishClient := &mockPublishClient{
			t:              t,
			expectedObject: objectId,
			expectedSpace:  spaceId,
			expectedPbFiles: map[string]struct{}{
				filepath.Join("objects", objectId+".pb"):   {},
				filepath.Join("types", objectTypeId+".pb"): {},
			},
			expectedErr: fmt.Errorf("internal error"),
		}

		exp := prepareExporter(t, objectTypeId, spaceService)

		svc := &service{
			spaceService:         spaceService,
			exportService:        exp,
			publishClientService: publishClient,
			identityService:      identityService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, true)

		// then
		assert.Error(t, err)
		assert.Equal(t, "", publish.Cid)
		assert.Equal(t, "{\"heads\":[\"heads\"]}", publishClient.expectedRequest.Version)
		assert.Equal(t, objectId, publishClient.expectedRequest.ObjectId)
		assert.Equal(t, spaceId, publishClient.expectedRequest.SpaceId)
		assert.Equal(t, expectedUri, publishClient.expectedRequest.Uri)
	})
	t.Run("limit error for members", func(t *testing.T) {
		// given
		spaceService := mock_space.NewMockService(t)

		expectedUri := "test"
		testFile := "test"
		err := createTestFile(testFile, membershipLimit+1)
		assert.NoError(t, err)

		objectTypeId := "customObjectType"
		file, err := os.Open(testFile)
		assert.NoError(t, err)
		defer func() {
			file.Close()
			os.Remove(testFile)
		}()

		exp := prepareExporterWithFile(t, objectTypeId, spaceService, file)

		identityService := mock_identity.NewMockService(t)
		globalName := "name"
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyGlobalName: domain.String(globalName),
		}))

		svc := &service{
			spaceService:    spaceService,
			exportService:   exp,
			identityService: identityService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, true)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
		assert.Equal(t, "", publish.Cid)
	})
	t.Run("default limit error", func(t *testing.T) {
		// given
		spaceService := mock_space.NewMockService(t)

		expectedUri := "test"
		testFile := "test"
		err := createTestFile(testFile, defaultLimit+1)
		assert.NoError(t, err)

		objectTypeId := "customObjectType"
		file, err := os.Open(testFile)
		assert.NoError(t, err)
		defer func() {
			file.Close()
			os.Remove(testFile)
		}()

		exp := prepareExporterWithFile(t, objectTypeId, spaceService, file)

		identityService := mock_identity.NewMockService(t)
		identityService.EXPECT().GetMyProfileDetails(context.Background()).Return("identity", nil, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{}))

		svc := &service{
			spaceService:    spaceService,
			exportService:   exp,
			identityService: identityService,
		}

		// when
		publish, err := svc.Publish(context.Background(), spaceId, objectId, expectedUri, true)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrLimitExceeded)
		assert.Equal(t, "", publish.Cid)
	})
}

func prepaeSpaceService(t *testing.T, isPersonal bool) (*mock_space.MockService, error) {
	spaceService := mock_space.NewMockService(t)
	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().IsPersonal().Return(isPersonal)
	space.EXPECT().Id().Return(spaceId)

	storage, err := spacestorage.NewInMemorySpaceStorage(spacestorage.SpaceStorageCreatePayload{
		AclWithId:           &consensusproto.RawRecordWithId{Id: "aclId"},
		SpaceHeaderWithId:   &spacesyncproto.RawSpaceHeaderWithId{Id: spaceId},
		SpaceSettingsWithId: &treechangeproto.RawTreeChangeWithId{Id: "settingsId"},
	},
	)
	assert.NoError(t, err)
	objectHeads := []string{"heads"}
	_, err = storage.CreateTreeStorage(treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: objectId},
		Heads:         objectHeads,
	})
	assert.NoError(t, err)
	space.EXPECT().Storage().Return(storage)
	spaceService.EXPECT().Get(context.Background(), spaceId).Return(space, nil)
	return spaceService, err
}

func prepareExporter(t *testing.T, objectTypeId string, spaceService *mock_space.MockService) export.Export {
	storeFixture := objectstore.NewStoreFixture(t)
	objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
	assert.Nil(t, err)

	storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:      domain.String(objectId),
			bundle.RelationKeyType:    domain.String(objectTypeId),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		},
		{
			bundle.RelationKeyId:                   domain.String(objectTypeId),
			bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
			bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
			bundle.RelationKeySpaceId:              domain.String(spaceId),
		},
	})

	objectGetter := mock_cache.NewMockObjectGetterComponent(t)

	smartBlockTest := smarttest.New(objectId)
	doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String(objectId),
		bundle.RelationKeyType: domain.String(objectTypeId),
	}))
	doc.AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyId.String(),
		Format: model.RelationFormat_longtext,
	}, &model.RelationLink{
		Key:    bundle.RelationKeyType.String(),
		Format: model.RelationFormat_longtext,
	})
	smartBlockTest.Doc = doc

	objectType := smarttest.New(objectTypeId)
	objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String(objectTypeId),
		bundle.RelationKeyType: domain.String(objectTypeId),
	}))
	objectTypeDoc.AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyId.String(),
		Format: model.RelationFormat_longtext,
	}, &model.RelationLink{
		Key:    bundle.RelationKeyType.String(),
		Format: model.RelationFormat_longtext,
	})
	objectType.Doc = objectTypeDoc
	objectType.SetType(smartblock.SmartBlockTypeObjectType)
	objectGetter.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
	objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

	a := &app.App{}
	mockSender := mock_event.NewMockSender(t)
	a.Register(storeFixture)
	a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter))
	a.Register(process.New())
	a.Register(testutil.PrepareMock(context.Background(), a, spaceService))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_typeprovider.NewMockSmartBlockTypeProvider(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_files.NewMockService(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_account.NewMockService(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_notifications.NewMockNotifications(t)))

	exp := export.New()
	err = exp.Init(a)
	assert.Nil(t, err)
	return exp
}

func prepareExporterWithFile(t *testing.T, objectTypeId string, spaceService *mock_space.MockService, testFile *os.File) export.Export {
	storeFixture := objectstore.NewStoreFixture(t)
	objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
	assert.Nil(t, err)

	fileId := "fileId"
	storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
		{
			bundle.RelationKeyId:      domain.String(fileId),
			bundle.RelationKeyType:    domain.String(objectTypeId),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		},
		{
			bundle.RelationKeyId:      domain.String(objectId),
			bundle.RelationKeyType:    domain.String(objectTypeId),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		},
		{
			bundle.RelationKeyId:                   domain.String(objectTypeId),
			bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
			bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
			bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
			bundle.RelationKeySpaceId:              domain.String(spaceId),
		},
	})

	objectGetter := mock_cache.NewMockObjectGetterComponent(t)

	smartBlockTest := smarttest.New(objectId)
	doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String(objectId),
		bundle.RelationKeyType: domain.String(objectTypeId),
	}))
	doc.AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyId.String(),
		Format: model.RelationFormat_longtext,
	}, &model.RelationLink{
		Key:    bundle.RelationKeyType.String(),
		Format: model.RelationFormat_longtext,
	})
	smartBlockTest.Doc = doc
	smartBlockTest.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"fileBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
	smartBlockTest.AddBlock(simple.New(&model.Block{Id: "fileBlock", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: fileId}}}))

	objectType := smarttest.New(objectTypeId)
	objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:   domain.String(objectTypeId),
		bundle.RelationKeyType: domain.String(objectTypeId),
	}))
	objectTypeDoc.AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyId.String(),
		Format: model.RelationFormat_longtext,
	}, &model.RelationLink{
		Key:    bundle.RelationKeyType.String(),
		Format: model.RelationFormat_longtext,
	})
	objectType.Doc = objectTypeDoc
	objectType.SetType(smartblock.SmartBlockTypeObjectType)

	file := smarttest.New(fileId)
	fileDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:     domain.String(fileId),
		bundle.RelationKeyType:   domain.String(objectTypeId),
		bundle.RelationKeyFileId: domain.String(fileId),
	}))
	fileDoc.AddRelationLinks(&model.RelationLink{
		Key:    bundle.RelationKeyId.String(),
		Format: model.RelationFormat_longtext,
	}, &model.RelationLink{
		Key:    bundle.RelationKeyType.String(),
		Format: model.RelationFormat_longtext,
	})
	file.Doc = fileDoc
	file.SetType(smartblock.SmartBlockTypeFileObject)
	file.SetSpaceId(spaceId)
	space := mock_clientspace.NewMockSpace(t)
	space.EXPECT().Id().Return(spaceId)
	space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{})
	file.SetSpace(space)

	objectGetter.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(3)
	objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil).Times(2)
	objectGetter.EXPECT().GetObject(context.Background(), fileId).Return(file, nil)

	fileService := mock_files.NewMockService(t)
	fileObject := mock_files.NewMockFile(t)
	fileObject.EXPECT().Info().Return(&storage.FileInfo{
		Media: "file",
	})
	fileObject.EXPECT().Meta().Return(&files2.FileMeta{
		Name: testFile.Name(),
	})
	fileObject.EXPECT().Reader(context.Background()).Return(testFile, nil)
	fileService.EXPECT().FileByHash(context.Background(), domain.FullFileId{FileId: domain.FileId(fileId), SpaceId: spaceId}).Return(fileObject, nil)
	a := &app.App{}
	mockSender := mock_event.NewMockSender(t)
	a.Register(storeFixture)
	a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter))
	a.Register(process.New())
	a.Register(testutil.PrepareMock(context.Background(), a, spaceService))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_typeprovider.NewMockSmartBlockTypeProvider(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_account.NewMockService(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, mock_notifications.NewMockNotifications(t)))
	a.Register(testutil.PrepareMock(context.Background(), a, fileService))

	exp := export.New()
	err = exp.Init(a)
	assert.Nil(t, err)
	return exp
}

func createTestFile(fileName string, size int64) error {
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	bufferSize := int64(4 * 1024)
	buffer := make([]byte, bufferSize)

	var written int64
	for written < size {
		_, err := rand.Read(buffer)
		if err != nil {
			return err
		}
		toWrite := bufferSize
		if written+bufferSize > size {
			toWrite = size - written
		}
		n, err := file.Write(buffer[:toWrite])
		if err != nil {
			return err
		}
		written += int64(n)
	}
	file.Sync()
	file.Close()
	return nil
}
