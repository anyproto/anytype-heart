package objectlink

import (
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/slice"
)

func CalculateOrphans(store objectstore.ObjectStore, oldLinks, newLinks []string) (orphans []string) {
	removed, _ := slice.DifferenceRemovedAdded(oldLinks, newLinks)
	if len(removed) == 0 {
		return
	}

	for _, id := range removed {
		backLinks, err := store.GetInboundLinksByID(id)
		if err != nil {
			log.With("objectID", id).Errorf("failed to get inbound links from object store to check orphans: %s", err)
			continue
		}

		if len(backLinks) != 0 {
			continue
		}

		orphans = append(orphans, id)
	}

	return orphans
}
