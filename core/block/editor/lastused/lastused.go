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
	case bundle.TypeKeyPage.BundledURL():
		priority = 1
	case bundle.TypeKeyTask.BundledURL():
		priority = 2
	case bundle.TypeKeyCollection.BundledURL():
		priority = 3
	case bundle.TypeKeySet.BundledURL():
		priority = 4
	case bundle.TypeKeyBookmark.BundledURL():
		priority = 5
	case bundle.TypeKeyNote.BundledURL():
		priority = 6
	case bundle.TypeKeyFile.BundledURL():
		priority = 7
	case bundle.TypeKeyImage.BundledURL():
		priority = 8
	case bundle.TypeKeyAudio.BundledURL():
		priority = 9
	case bundle.TypeKeyVideo.BundledURL():
		priority = 10
	case bundle.TypeKeyTemplate.BundledURL():
		priority = 11
	case bundle.TypeKeyParticipant.BundledURL():
		priority = 12
	default:
		priority = 13
	}

	// we do this trick to order crucial Anytype object types by last date
	lastUsed := time.Now().Add(time.Duration(-1 * priority * int64(maxInstallationTime))).Unix()
	details.SetInt64(bundle.RelationKeyLastUsedDate, lastUsed)
}
