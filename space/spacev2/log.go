package spacev2

import (
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
)

var log = logger.NewNamed(CName)

func zapSpaceId(id string) zap.Field { return zap.String("spaceId", id) }
func zapError(err error) zap.Field   { return zap.Error(err) }
