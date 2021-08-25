package doc

import (
	"context"
	"sync"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const CName = "docService"

var log = logging.Logger("anytype-mw-block-doc")

func New() Service {
	return &listener{}
}

type DocInfo struct {
	Id           string
	Links        []string
	FileHashes   []string
	SetRelations []*model.Relation
	SetSource    string
	Creator      string
	State        *state.State
}

type OnDocChangeCallback func(ctx context.Context, info DocInfo) error

type Service interface {
	GetDocInfo(ctx context.Context, id string) (info DocInfo, err error)
	OnWholeChange(cb OnDocChangeCallback)
	ReportChange(ctx context.Context, info DocInfo)

	app.Component
}

type docInfoHandler interface {
	GetDocInfo(ctx context.Context, id string) (info DocInfo, err error)
}

type listener struct {
	wholeCallbacks []OnDocChangeCallback
	docInfoHandler docInfoHandler

	m sync.RWMutex
}

func (l *listener) Init(a *app.App) (err error) {
	l.docInfoHandler = a.MustComponent("blockService").(docInfoHandler)
	return
}

func (l *listener) Name() (name string) {
	return CName
}

func (l *listener) ReportChange(ctx context.Context, info DocInfo) {
	l.m.RLock()
	defer l.m.RUnlock()
	for _, cb := range l.wholeCallbacks {
		if err := cb(ctx, info); err != nil {
			log.Errorf("state change callback error: %v", err)
		}
	}
}

func (l *listener) OnWholeChange(cb OnDocChangeCallback) {
	l.m.Lock()
	defer l.m.Unlock()
	l.wholeCallbacks = append(l.wholeCallbacks, cb)
}

func (l *listener) GetDocInfo(ctx context.Context, id string) (info DocInfo, err error) {
	return l.docInfoHandler.GetDocInfo(ctx, id)
}
