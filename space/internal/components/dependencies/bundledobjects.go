package dependencies

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

type BundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spc clientspace.Space, ids []string, isNewSpace bool) ([]string, []*types.Struct, error)
	BundledObjectsIdsToInstall(ctx context.Context, spc clientspace.Space, sourceObjectIds []string) (ids domain.BundledObjectIds, err error)
}
