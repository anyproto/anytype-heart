package dump

import (
	"archive/zip"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
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
	for i, id := range objectIDs {
		if err = s.blockService.Do(id, func(b smartblock.SmartBlock) error {
			mo, err := s.getMigrationObject(b)
			if err != nil {
				return err
			}
			wErr := s.writeSnapshotToFile(zw, strconv.FormatInt(int64(i), 10), mo)
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
