package lastused

import (
	"strings"
	"time"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const maxInstallationTime = 5 * time.Minute

type Key interface {
	URL() string
	String() string
}

var log = logging.Logger("update-last-used-date")

func UpdateLastUsedDate(spc smartblock.Space, store objectstore.ObjectStore, key Key) {
	uk, err := domain.UnmarshalUniqueKey(key.URL())
	if err != nil {
		log.Errorf("failed to unmarshall key '%s': %w", key.String(), err)
		return
	}

	if uk.SmartblockType() != coresb.SmartBlockTypeObjectType && uk.SmartblockType() != coresb.SmartBlockTypeRelation {
		log.Errorf("cannot update lastUsedDate for object with key='%s' and smartblockType='%s'. "+
			"Only object types and relations are expected", key.String(), uk.SmartblockType().String())
		return
	}

	details, err := store.GetObjectByUniqueKey(spc.Id(), uk)
	if err != nil {
		log.Errorf("failed to get details of type object '%s': %v", key.String(), err)
		return
	}

	id := pbtypes.GetString(details.Details, bundle.RelationKeyId.String())
	if id == "" {
		log.Errorf("failed to get id from details of type object '%s': %w", key.String(), err)
		return
	}

	if err = spc.Do(id, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetLocalDetail(bundle.RelationKeyLastUsedDate.String(), pbtypes.Int64(time.Now().Unix()))
		return sb.Apply(st)
	}); err != nil {
		log.Errorf("failed to set lastUsedDate to type object '%s': %w", key.String(), err)
	}
}

func SetLastUsedDateForInitialObjectType(id string, details *types.Struct) {
	if !strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) || details == nil || details.Fields == nil {
		return
	}

	var priority int64
	switch id {
	case bundle.TypeKeyNote.BundledURL():
		priority = 1
	case bundle.TypeKeyPage.BundledURL():
		priority = 2
	case bundle.TypeKeyTask.BundledURL():
		priority = 3
	case bundle.TypeKeySet.BundledURL():
		priority = 4
	case bundle.TypeKeyCollection.BundledURL():
		priority = 5
	default:
		priority = 7
	}

	// we do this trick to order crucial Anytype object types by last date
	lastUsed := time.Now().Add(time.Duration(-1 * priority * int64(maxInstallationTime))).Unix()
	details.Fields[bundle.RelationKeyLastUsedDate.String()] = pbtypes.Int64(lastUsed)
}
