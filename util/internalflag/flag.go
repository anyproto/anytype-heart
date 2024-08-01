package internalflag

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const relationKey = bundle.RelationKeyInternalFlags

type Set struct {
	flags []int
}

func NewFromState(st *state.State) *Set {
	flags := st.Details().GetFloatList(relationKey)

	return &Set{
		flags: slice.FloatsInto[int](flags),
	}
}

func (s *Set) Add(flag model.InternalFlagValue) {
	if !s.Has(flag) {
		s.flags = append(s.flags, int(flag))
	}
}

func (s *Set) Has(flag model.InternalFlagValue) bool {
	for _, f := range s.flags {
		if f == int(flag) {
			return true
		}
	}
	return false
}

func (s *Set) Remove(flag model.InternalFlagValue) {
	res := s.flags[:0]
	for _, f := range s.flags {
		if f == int(flag) {
			continue
		}
		res = append(res, f)
	}
	s.flags = res
}

func (s *Set) AddToState(st *state.State) {
	if len(s.flags) == 0 {
		st.RemoveDetail(relationKey)
		return
	}
	st.SetDetailAndBundledRelation(relationKey, pbtypes.IntList(s.flags...))
}

func (s *Set) IsEmpty() bool {
	return len(s.flags) == 0
}

func PutToDetails(details *domain.Details, flags []*model.InternalFlag) *domain.Details {
	ints := make([]int, 0, len(flags))
	for _, f := range flags {
		ints = append(ints, int(f.Value))
	}
	return putToDetails(details, ints)
}

func putToDetails(details *domain.Details, flags []int) *domain.Details {
	if details == nil {
		details = domain.NewDetails()
	}
	details.Set(relationKey, flags)

	return details
}
