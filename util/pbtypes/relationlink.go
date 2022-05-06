package pbtypes

import "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"

type RelationLinks []*model.RelationLink

func (rl RelationLinks) Has(id string) bool {
	for _, l := range rl {
		if l.Id == id {
			return true
		}
	}
	return false
}

func (rl RelationLinks) Key(id string) (key string, ok bool) {
	for _, l := range rl {
		if l.Id == id {
			return l.Key, true
		}
	}
	return
}

func (rl RelationLinks) Append(l *model.RelationLink) RelationLinks {
	return append(rl, l)
}

func (rl RelationLinks) Remove(id string) RelationLinks {
	var n int
	for _, x := range rl {
		if x.Id != id {
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
			Id:  l.Id,
			Key: l.Key,
		})
	}
	return res
}

func (rl RelationLinks) Diff(prev RelationLinks) (added []*model.RelationLink, removed []string) {
	var common = make(map[string]struct{})
	for _, l := range rl {
		if !prev.Has(l.Id) {
			added = append(added, l)
		} else {
			common[l.Id] = struct{}{}
		}
	}
	for _, l := range prev {
		if _, ok := common[l.Id]; !ok {
			removed = append(removed, l.Id)
		}
	}
	return
}
