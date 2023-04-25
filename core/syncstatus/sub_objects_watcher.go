package syncstatus

import (
	"sync"
)

type subObjectsWatcher struct {
	sync.Mutex
	subObjects map[string]struct{}
}

func newSubObjectsWatcher() *subObjectsWatcher {
	return &subObjectsWatcher{
		subObjects: map[string]struct{}{},
	}
}

func (s *subObjectsWatcher) Watch(id string) {
	s.Lock()
	defer s.Unlock()

	s.subObjects[id] = struct{}{}
}

func (s *subObjectsWatcher) Unwatch(id string) {
	s.Lock()
	defer s.Unlock()

	delete(s.subObjects, id)
}

func (s *subObjectsWatcher) ForEach(f func(id string)) {
	s.Lock()
	defer s.Unlock()

	for id := range s.subObjects {
		f(id)
	}
}
