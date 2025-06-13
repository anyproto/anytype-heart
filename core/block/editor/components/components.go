package components

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type Text interface {
	domain.EditorComponent

	UpdateTextBlocks(ctx session.Context, ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(ctx session.Context, req pb.RpcBlockSplitRequest) (newId string, err error)
	Merge(ctx session.Context, firstId, secondId string) (err error)
	SetMark(ctx session.Context, mark *model.BlockContentTextMark, blockIds ...string) error
	SetIcon(ctx session.Context, image, emoji string, blockIds ...string) error
	SetText(ctx session.Context, req pb.RpcBlockTextSetTextRequest) (err error)
	TurnInto(ctx session.Context, style model.BlockContentTextStyle, ids ...string) error
}

type Entity interface {
	Components() []domain.EditorComponent
}

func GetComponent[T domain.EditorComponent](e Entity) (T, error) {
	for _, c := range e.Components() {
		v, ok := c.(T)
		if ok {
			return v, nil
		}
	}
	var defValue T
	return defValue, fmt.Errorf("component not found")
}
