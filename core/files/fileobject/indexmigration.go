package fileobject

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type indexMigrationItem struct {
	Id      string
	SpaceId string
}

func (s *service) startIndexMigration() error {
	// Producer
	go func() {
		select {
		case <-s.componentCtx.Done():
		case <-time.After(time.Second * 3):
		}
		toMigrate, err := s.listNonIndexedFiles()
		if err != nil {
			log.Error("list non indexed files", zap.Error(err))
			return
		}

		if len(toMigrate) == 0 {
			return
		}

		for _, item := range toMigrate {
			it := &indexMigrationItem{
				Id:      item.Details.GetString(bundle.RelationKeyId),
				SpaceId: item.Details.GetString(bundle.RelationKeySpaceId),
			}
			select {
			case s.indexMigrationChan <- it:
			case <-s.componentCtx.Done():
				return
			}

		}
	}()

	// Consumer
	go s.indexMigrationWorker()

	return nil
}

func (s *service) listNonIndexedFiles() ([]database.Record, error) {
	return s.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value: domain.Int64List([]model.ObjectTypeLayout{
					model.ObjectType_file,
					model.ObjectType_image,
					model.ObjectType_video,
					model.ObjectType_audio,
					model.ObjectType_pdf,
				}),
			},
			{
				RelationKey: bundle.RelationKeyFileVariantIds,
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
}

func (s *service) indexMigrationWorker() {
	for {
		select {
		case item := <-s.indexMigrationChan:
			err := s.migrateIndex(item)
			if err != nil {
				log.Error("index migration", zap.Error(err))
			}
		case <-s.componentCtx.Done():
			return
		}
	}
}

func (s *service) migrateIndex(item *indexMigrationItem) error {
	spc, err := s.spaceService.Wait(s.componentCtx, item.SpaceId)
	if err != nil {
		return fmt.Errorf("get space: %w", err)
	}

	err = spc.Do(item.Id, func(sb smartblock.SmartBlock) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("touch object: %w", err)
	}
	return nil
}
