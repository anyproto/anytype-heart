package objectorigin

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func Ptr(origin model.ObjectOrigin) *model.ObjectOrigin {
	return &origin
}
