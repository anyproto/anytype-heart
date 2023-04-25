package internalflag

import (
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

const relationKey = bundle.RelationKeyInternalFlags

type Set struct {
	flags []int
}

func NewFromState(st *state.State) Set {
	flags := pbtypes.GetIntList(st.CombinedDetails(), relationKey.String())

	return Set{
		flags: flags,
	}
}

func (s *Set) Add(flag model.InternalFlagValue) {
	if !s.Has(flag) {
		s.flags = append(s.flags, int(flag))
	}
}

func (s Set) Has(flag model.InternalFlagValue) bool {
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

func (s Set) AddToState(st *state.State) {
	if len(s.flags) == 0 {
		st.RemoveDetail(relationKey.String())
		return
	}
	st.SetDetailAndBundledRelation(relationKey, pbtypes.IntList(s.flags...))
}

func PutToDetails(details *types.Struct, flags []*model.InternalFlag) *types.Struct {
	ints := make([]int, 0, len(flags))
	for _, f := range flags {
		ints = append(ints, int(f.Value))
	}
	return putToDetails(details, ints)
}

func putToDetails(details *types.Struct, flags []int) *types.Struct {
	if details == nil {
		details = &types.Struct{
			Fields: map[string]*types.Value{},
		}
	}
	if details.Fields == nil {
		details.Fields = map[string]*types.Value{}
	}
	details.Fields[relationKey.String()] = pbtypes.IntList(flags...)

	return details
}
