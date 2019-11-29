package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
)

func (p *commonSmart) validateChildrenIds(b *model.Block) (err error) {
	for _, id := range b.ChildrenIds {
		if _, ok := p.versions[id]; !ok {
			return fmt.Errorf("block[%s]: children '%s' not found", b.Id, id)
		}
	}
	return
}
