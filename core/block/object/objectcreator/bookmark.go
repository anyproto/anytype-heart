package objectcreator

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/bookmark"
	simpleBookmark "github.com/anyproto/anytype-heart/core/block/simple/bookmark"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/uri"
)

// createBookmark creates a new Bookmark object for provided URL or returns id of existing one
func (s *service) createBookmark(ctx context.Context, spaceId string, req *pb.RpcObjectCreateBookmarkRequest) (objectID string, newDetails *types.Struct, err error) {
	source := pbtypes.GetString(req.Details, bundle.RelationKeySource.String())
	var res bookmark.ContentFuture
	if source != "" {
		u, err := uri.NormalizeURI(source)
		if err != nil {
			return "", nil, fmt.Errorf("process uri: %w", err)
		}
		res = s.bookmark.FetchBookmarkContent(req.SpaceId, u, false)
	} else {
		res = func() *simpleBookmark.ObjectContent {
			return nil
		}
	}
	return s.bookmark.CreateBookmarkObject(ctx, spaceId, req.Details, res)
}
