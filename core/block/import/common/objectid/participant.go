package objectid

import (
	"context"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type participant struct{}

func newParticipant() *participant {
	return &participant{}
}

func (w *participant) GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, _ time.Time, _ bool, _ objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	participantId := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	splitId := strings.Split(participantId, "_")
	identity := splitId[len(splitId)-1]
	newParticipantID := domain.NewParticipantId(spaceID, identity)
	return newParticipantID, treestorage.TreeStorageCreatePayload{}, nil
}
