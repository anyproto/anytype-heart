package basic

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
)

func CutBlocks(s *state.State, blockIds []string) (blocks []simple.Block) {
	visited := map[string]struct{}{}
	for _, id := range blockIds {
		b := s.Pick(id)
		if b == nil {
			continue
		}

		queue := append(s.Descendants(id), b)
		for _, b := range queue {
			if _, ok := visited[b.Model().Id]; ok {
				continue
			}
			blocks = append(blocks, b.Copy())
			visited[b.Model().Id] = struct{}{}
			s.Unlink(b.Model().Id)
		}
	}
	return blocks
}
