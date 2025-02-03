package lastused

import (
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

const maxInstallationTime = 5 * time.Minute

func SetLastUsedDateForInitialObjectType(id string, details *domain.Details) {
	if !strings.HasPrefix(id, addr.BundledObjectTypeURLPrefix) || details == nil {
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
	details.SetInt64(bundle.RelationKeyLastUsedDate, lastUsed)
}
