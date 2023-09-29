package testprovider

import (
	"github.com/anyproto/anytype-heart/core/block/import/objectid"
	"github.com/anyproto/anytype-heart/core/block/import/objectid/mock_objectid"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

type TestProvider struct {
	idGetter *mock_objectid.MockIDGetter
}

func NewTestProvider(idGetter *mock_objectid.MockIDGetter) *TestProvider {
	return &TestProvider{idGetter: idGetter}
}

func (t TestProvider) ProvideIdGetter(_ smartblock.SmartBlockType) (objectid.IdGetter, error) {
	return t.idGetter, nil
}
