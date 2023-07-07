package space

import (
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/session"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

func (s *clientSpace) DerivePredefinedObjects(ctx session.Context, createTrees bool) (predefinedObjectIDs threads.DerivedSmartblockIds, err error) {
	ids := s.core.PredefinedObjects(s.Id())
	if ids.IsFilled() {
		return ids, nil
	}
	ids, err = s.derivePredefinedObjects(ctx, createTrees)
	if err != nil {
		return threads.DerivedSmartblockIds{}, err
	}
	return ids, nil
}

func (s *clientSpace) derivePredefinedObjects(ctx session.Context, createTrees bool) (predefinedObjectIDs threads.DerivedSmartblockIds, err error) {
	sbTypes := []coresb.SmartBlockType{
		coresb.SmartBlockTypeWorkspace,
		coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeArchive,
		coresb.SmartBlockTypeWidget,
		coresb.SmartBlockTypeHome,
	}
	payloads := make([]*treestorage.TreeStorageCreatePayload, len(sbTypes))
	for i, sbt := range sbTypes {
		exists := s.core.PredefinedObjects(s.Id()).HasID(sbt)
		if exists {
			continue
		}
		payloads[i], err = s.DeriveTreeCreatePayload(sbt)
		if err != nil {
			log.With(zap.Error(err)).Debug("derived tree object with error")
			return predefinedObjectIDs, fmt.Errorf("derive tree create payload: %w", err)
		}
		predefinedObjectIDs.InsertId(sbt, payloads[i].RootRawChange.Id)

		s.core.RegisterPredefinedObjects(s.Id(), predefinedObjectIDs)
	}

	for _, payload := range payloads {
		err = s.DeriveObject(ctx, payload, createTrees)
		if err != nil {
			log.With(zap.Error(err)).Debug("derived object with error")
			return predefinedObjectIDs, fmt.Errorf("derive object: %w", err)
		}
	}
	return
}
