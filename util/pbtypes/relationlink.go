package pbtypes

import (
	"slices"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TODO Add domaain model for link
type RelationLinks []*model.RelationLink

func (rl RelationLinks) Get(key string) *model.RelationLink {
	for _, l := range rl {
		if l.Key == key {
			return l
		}
	}
	return nil
}

func (rl RelationLinks) Has(key string) bool {
	for _, l := range rl {
		if l.Key == key {
			return true
		}
	}
	return false
}

func (rl RelationLinks) Append(l *model.RelationLink) RelationLinks {
	return append(rl, l)
}

func (rl RelationLinks) Remove(keys ...string) RelationLinks {
	var n int
	for _, x := range rl {
		if !slices.Contains(keys, x.Key) {
			rl[n] = x
			n++
		}
	}
	return rl[:n]
}

func (rl RelationLinks) Copy() RelationLinks {
	res := make(RelationLinks, 0, len(rl))
	for _, l := range rl {
		res = append(res, &model.RelationLink{
			Format: l.Format,
			Key:    l.Key,
		})
	}
	return res
}

func (rl RelationLinks) Diff(prev RelationLinks) (added []*model.RelationLink, removed []string) {
	var common = make(map[string]struct{})
	for _, l := range rl {
		if !prev.Has(l.Key) {
			added = append(added, l)
		} else {
			common[l.Key] = struct{}{}
		}
	}
	for _, l := range prev {
		if _, ok := common[l.Key]; !ok {
			removed = append(removed, l.Key)
		}
	}
	return
}
