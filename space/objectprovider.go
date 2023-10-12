package space

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

type bundledObjectsInstaller interface {
	app.Component
	InstallBundledObjects(ctx context.Context, spc Space, ids []string) ([]string, []*types.Struct, error)
}

func (s *space) InstallBundledObjects(ctx context.Context) error {
	ids := make([]string, 0, len(bundle.SystemTypes)+len(bundle.SystemRelations))
	for _, ot := range bundle.SystemTypes {
		ids = append(ids, ot.BundledURL())
	}
	for _, rk := range bundle.SystemRelations {
		ids = append(ids, rk.BundledURL())
	}
	_, _, err := s.installer.InstallBundledObjects(ctx, s, ids)
	if err != nil {
		return err
	}
	return nil
}
