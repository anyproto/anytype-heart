package dump

import (
	"archive/zip"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/proto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pb"
	smartblocktype "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const Name = "dump"

const profileFile = "profile"

type Service struct {
	objectStore  objectstore.ObjectStore
	blockService *block.Service
	app.Component
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Name() string {
	return Name
}

func (s *Service) Init(a *app.App) (err error) {
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.blockService = a.MustComponent(block.CName).(*block.Service)
	return nil
}

func (s *Service) Dump(path string, mnemonic string, profile core.Profile) error {
	objectIDs, _, err := s.objectStore.QueryObjectIds(database.Query{}, nil)
	if err != nil {
		return fmt.Errorf("failed to QueryObjectIds: %v", err)
	}

	deletedObjects, _, err := s.objectStore.QueryObjectInfo(database.Query{
		Filters: []*model.BlockContentDataviewFilter{{
			RelationKey: bundle.RelationKeyIsDeleted.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Bool(true),
		},
		},
	}, nil)

	archivedObjects, _, err := s.objectStore.QueryObjectInfo(database.Query{
		Filters: []*model.BlockContentDataviewFilter{{
			RelationKey: bundle.RelationKeyIsArchived.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Bool(true),
		},
		},
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to QueryObjectIds: %v", err)
	}
	fullPath := buildPath(path)
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %v", err)
	}
	zw := zip.NewWriter(f)
	defer zw.Close()
	defer func() {
		if err != nil {
			os.Remove(fullPath)
		}
	}()

	pr := &pb.Profile{
		Mnemonic: mnemonic,
		Name:     profile.Name,
		Avatar:   profile.IconImage,
	}
	wErr := s.writeSnapshotToFile(zw, profileFile, pr)
	if wErr != nil {
		return wErr
	}

	for _, object := range deletedObjects {
		mo, err := s.getMigrationObjectFromObjectInfo(object)
		if err != nil {
			return err
		}
		wErr := s.writeSnapshotToFile(zw, object.Id, mo)
		if wErr != nil {
			return wErr
		}
	}

	for _, object := range archivedObjects {
		mo, err := s.getMigrationObjectFromObjectInfo(object)
		if err != nil {
			return err
		}
		wErr := s.writeSnapshotToFile(zw, object.Id, mo)
		if wErr != nil {
			return wErr
		}
	}

	for _, id := range objectIDs {
		if err = s.blockService.Do(id, func(b smartblock.SmartBlock) error {
			sbType, err := smartblocktype.SmartBlockTypeFromID(b.RootId())
			if err != nil {
				return fmt.Errorf("failed SmartBlockTypeFromID: %v", err)
			}
			if isBundledObject(sbType) {
				return nil
			}
			mo, err := s.getMigrationObject(b)
			if err != nil {
				return err
			}
			wErr := s.writeSnapshotToFile(zw, id, mo)
			if wErr != nil {
				return wErr
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed blockService.Do: %v", err)
		}
	}
	return err
}

func (s *Service) getMigrationObjectFromObjectInfo(object *model.ObjectInfo) (*pb.MigrationObject, error) {
	sbType, err := smartblocktype.SmartBlockTypeFromID(object.Id)
	if err != nil {
		return nil, fmt.Errorf("failed SmartBlockTypeFromID: %v", err)
	}
	sn := &model.SmartBlockSnapshotBase{
		Details:     object.GetDetails(),
		ObjectTypes: object.GetObjectTypeUrls(),
	}
	mo := &pb.MigrationObject{
		SbType:   sbType.ToProto(),
		Snapshot: sn,
	}
	return mo, nil
}

func (s *Service) getMigrationObject(b smartblock.SmartBlock) (*pb.MigrationObject, error) {
	st := b.NewState()
	rootID := st.RootId()
	sbType, err := smartblocktype.SmartBlockTypeFromID(rootID)
	if err != nil {
		return nil, fmt.Errorf("failed SmartBlockTypeFromID: %v", err)
	}
	removedCollectionKeys := make([]string, 0, len(st.StoreKeysRemoved()))
	for key := range st.StoreKeysRemoved() {
		removedCollectionKeys = append(removedCollectionKeys, key)
	}
	sn := &model.SmartBlockSnapshotBase{
		Blocks:                st.Blocks(),
		Details:               st.Details(),
		ObjectTypes:           st.ObjectTypes(),
		RelationLinks:         st.GetRelationLinks(),
		Collections:           st.Store(),
		RemovedCollectionKeys: removedCollectionKeys,
		ExtraRelations:        st.OldExtraRelations(),
	}
	mo := &pb.MigrationObject{
		SbType:   sbType.ToProto(),
		Snapshot: sn,
	}
	return mo, nil
}

func (s *Service) writeSnapshotToFile(zw *zip.Writer, name string, ob proto.Marshaler) error {
	wr, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("failed create file with snapshot: %v", err)
	}
	data, err := ob.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %v", err)
	}
	_, err = wr.Write(data)
	if err != nil {
		return fmt.Errorf("failed write snapshot to file: %v", err)
	}
	return nil
}

func buildPath(path string) string {
	var sb strings.Builder
	sb.WriteString(path)
	sb.WriteRune(filepath.Separator)
	sb.WriteString(Name)
	sb.WriteString(strconv.FormatInt(time.Now().Unix(), 10))
	sb.WriteString(".zip")
	return sb.String()
}

func isBundledObject(objectType smartblocktype.SmartBlockType) bool {
	return objectType == smartblocktype.SmartBlockTypeBundledObjectType ||
		objectType == smartblocktype.SmartBlockTypeBundledTemplate ||
		objectType == smartblocktype.SmartBlockTypeBundledRelation ||
		objectType == smartblocktype.SmartBlockTypeWorkspaceOld ||
		objectType == smartblocktype.SmartBlockTypeWorkspace ||
		objectType == smartblocktype.SmartBlockTypeArchive ||
		objectType == smartblocktype.SmartBlockTypeHome ||
		objectType == smartblocktype.SmartblockTypeMarketplaceRelation ||
		objectType == smartblocktype.SmartblockTypeMarketplaceTemplate ||
		objectType == smartblocktype.SmartblockTypeMarketplaceType
}
