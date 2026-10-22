package resolve

import (
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Formats is the resident relation-format registry: seeded by the engine as
// relation definitions stream through (definitions-before-use guarantees a
// format is known before any object using the relation is resolved), with
// bundled relations as fallback. Safe for concurrent use.
type Formats struct {
	mu sync.RWMutex
	m  map[domain.RelationKey]model.RelationFormat
}

func NewFormats() *Formats {
	return &Formats{m: map[domain.RelationKey]model.RelationFormat{}}
}

func (f *Formats) Register(key domain.RelationKey, format model.RelationFormat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[key] = format
}

func (f *Formats) RelationFormat(key domain.RelationKey) (model.RelationFormat, bool) {
	f.mu.RLock()
	format, ok := f.m[key]
	f.mu.RUnlock()
	if ok {
		return format, true
	}
	if relation, err := bundle.GetRelation(key); err == nil {
		return relation.Format, true
	}
	return 0, false
}
