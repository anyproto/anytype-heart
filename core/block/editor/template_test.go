package editor

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func NewTemplateTest(t *testing.T, ctrl *gomock.Controller, templateName string) (*Template, error) {
	sb := smarttest.New("root")
	_ = sb.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(templateName),
	}}, false)

	objectStore := testMock.NewMockObjectStore(ctrl)

	templ := &Template{
		Page: &Page{
			SmartBlock:  sb,
			objectStore: objectStore,
		},
	}
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, bundle.TypeKeyPage.String())
	require.NoError(t, err)

	objectStore.EXPECT().GetObjectByUniqueKey(gomock.Any(), uniqueKey).Return(&model.ObjectDetails{
		Details: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(model.ObjectType_basic)),
			},
		},
	}, nil)
	initCtx := &smartblock.InitContext{IsNewObject: true}
	if err := templ.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(templ, initCtx)
	if err := templ.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return templ, nil
}
