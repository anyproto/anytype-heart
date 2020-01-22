package anytype

import (
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
)

var (
	saveTimeout = time.Second * 5
)

func NewAnytype(c *core.Anytype) Anytype {
	return &anytype{c}
}

type anytype struct {
	*core.Anytype
}

func (a *anytype) GetBlock(id string) (Block, error) {
	b, err := a.Anytype.GetBlock(id)
	if err != nil {
		return nil, err
	}
	return a.newSmartBlock(b), nil
}

func (a *anytype) newSmartBlock(b core.Block) Block {
	sb := &smartBlock{
		Block:      b,
		blocks:     make(chan []*model.Block, 10),
		flushAndDo: make(chan func()),
		stop:       make(chan struct{}),
		buf:        make(map[string]*model.Block),
	}
	go sb.saveLoop()
	return sb
}

type smartBlock struct {
	core.Block
	blocks     chan []*model.Block
	flushAndDo chan func()
	stop       chan struct{}
	buf        map[string]*model.Block
	m          sync.Mutex
}

func (sb *smartBlock) AddVersions(vers []*model.Block) ([]core.BlockVersion, error) {
	var needFlush bool
	for _, ver := range vers {
		if ver.GetPage() != nil {
			needFlush = true
			break
		}
	}
	sb.blocks <- vers
	if needFlush {
		sb.flushAndDo <- func() {}
	}
	return make([]core.BlockVersion, len(vers)), nil
}

func (sb *smartBlock) saveLoop() {
	ticker := time.NewTicker(saveTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sb.doFlush()
		case f := <-sb.flushAndDo:
			sb.doFlush()
			f()
		case blocks := <-sb.blocks:
			for _, m := range blocks {
				sb.buf[m.Id] = m
			}
		case <-sb.stop:
			sb.doFlush()
			close(sb.stop)
			return
		}
	}
}

func (sb *smartBlock) doFlush() {
	if len(sb.buf) == 0 {
		return
	}
	blocksToSave := make([]*model.Block, 0, len(sb.buf))
	for _, m := range sb.buf {
		blocksToSave = append(blocksToSave, m)
	}
	if _, err := sb.Block.AddVersions(blocksToSave); err != nil {
		fmt.Printf("middle: can't save versions to lib: %v\n", err)
		return
	}
	fmt.Printf("middle: flush %d versions to lib\n", len(blocksToSave))
	sb.buf = make(map[string]*model.Block)
	return
}

func (sb *smartBlock) Close() error {
	sb.stop <- struct{}{}
	<-sb.stop
	return nil
}
