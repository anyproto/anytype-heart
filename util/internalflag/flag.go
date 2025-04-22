package internalflag

import (
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const relationKey = bundle.RelationKeyInternalFlags

type Set struct {
	flags []float64
}

func NewFromState(st *state.State) *Set {
	flags := st.Details().GetFloat64List(relationKey)

	return &Set{
		flags: flags,
	}
}

func (s *Set) Add(flag model.InternalFlagValue) {
	if !s.Has(flag) {
		s.flags = append(s.flags, float64(flag))
	}
}

func (s *Set) Has(flag model.InternalFlagValue) bool {
	for _, f := range s.flags {
		if f == float64(flag) {
			return true
		}
	}
	return false
}

func (s *Set) Remove(flag model.InternalFlagValue) {
	res := s.flags[:0]
	for _, f := range s.flags {
		if f == float64(flag) {
			continue
		}
		res = append(res, f)
	}
	s.flags = res
}

func (s *Set) AddToState(st *state.State) {
	if len(s.flags) == 0 {
		st.SetDetailAndBundledRelation(relationKey, domain.Float64List([]float64{}))
		return
	}
	st.SetDetail(relationKey, domain.Float64List(s.flags))
}

func (s *Set) IsEmpty() bool {
	return len(s.flags) == 0
}

func PutToDetails(details *domain.Details, flags []*model.InternalFlag) *domain.Details {
	raw := make([]float64, 0, len(flags))
	for _, f := range flags {
		raw = append(raw, float64(f.Value))
	}
	return putToDetails(details, raw)
}

func putToDetails(details *domain.Details, flags []float64) *domain.Details {
	if details == nil {
		details = domain.NewDetails()
	}
	details.SetFloat64List(relationKey, flags)

	return details
}
