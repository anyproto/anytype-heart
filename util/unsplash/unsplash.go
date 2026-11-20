package unsplash

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
)

const CName = "unsplash"

type Result struct {
	ID              string
	Description     string
	PictureThumbUrl string
	PictureSmallUrl string
	PictureFullUrl  string
	PictureHDUrl    string
	Artist          string
	ArtistURL       string
}

type Unsplash interface {
	Search(ctx context.Context, query string, max int) ([]Result, error)
	Download(ctx context.Context, id string) (imgPath string, err error)
	app.ComponentRunnable
}

type noOp struct{}

func (noOp) Init(*app.App) error         { return nil }
func (noOp) Name() string                { return CName }
func (noOp) Run(context.Context) error   { return nil }
func (noOp) Close(context.Context) error { return nil }
func (noOp) Search(context.Context, string, int) ([]Result, error) {
	return nil, errors.New("unsplash disabled in offline mode")
}
func (noOp) Download(context.Context, string) (string, error) {
	return "", errors.New("unsplash disabled in offline mode")
}

func New() Unsplash { return noOp{} }
