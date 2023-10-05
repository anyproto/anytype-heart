package space

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app/ocache"

	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

func (s *service) DerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error) {
	res, err := s.derivedIDsCache.Get(ctx, spaceID)
	if err != nil {
		return
	}
	return res.(deriveIDsObject).DerivedSmartblockIds, nil
}

func (s *service) loadDerivedIDs(ctx context.Context, spaceID string) (ocache.Object, error) {
	var sbTypes []coresb.SmartBlockType
	if s.IsPersonal(spaceID) {
		sbTypes = threads.PersonalSpaceTypes
	} else {
		sbTypes = threads.SpaceTypes
	}
	ids, err := s.provider.DeriveObjectIDs(ctx, spaceID, sbTypes)
	if err != nil {
		return nil, err
	}
	return deriveIDsObject{ids}, nil
}

type deriveIDsObject struct {
	threads.DerivedSmartblockIds
}

func (d deriveIDsObject) Close() (err error) {
	return nil
}

func (d deriveIDsObject) TryClose(objectTTL time.Duration) (res bool, err error) {
	return false, nil
}
