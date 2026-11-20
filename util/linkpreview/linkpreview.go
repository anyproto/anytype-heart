package linkpreview

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "linkpreview"

var ErrPrivateLink = errors.New("link preview disabled in offline mode")

type LinkPreview interface {
	Fetch(ctx context.Context, url string, withResponseBody bool) (model.LinkPreview, []byte, bool, error)
	app.Component
}

type noOp struct{}

func (noOp) Init(*app.App) error         { return nil }
func (noOp) Name() string                { return CName }
func (noOp) Run(context.Context) error   { return nil }
func (noOp) Close(context.Context) error { return nil }
func (noOp) Fetch(context.Context, string, bool) (model.LinkPreview, []byte, bool, error) {
	return model.LinkPreview{}, nil, false, ErrPrivateLink
}

func New() LinkPreview          { return noOp{} }
func NewWithCache() LinkPreview { return noOp{} }
