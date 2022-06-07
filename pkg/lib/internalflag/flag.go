package internalflag

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type Set struct {
	flags []int
}

func NewFromState(st *state.State) Set {
	flags := pbtypes.GetIntList(st.LocalDetails(), bundle.RelationKeyInternalFlags.String())

	return Set{
		flags: flags,
	}
}

func ExtractFromDetails(details *types.Struct) Set {
	flags := pbtypes.GetIntList(details, bundle.RelationKeyInternalFlags.String())
	delete(details.Fields, bundle.RelationKeyInternalFlags.String())

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
		st.RemoveLocalDetail(bundle.RelationKeyInternalFlags.String())
		return
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeyInternalFlags, pbtypes.IntList(s.flags...))
}

func AddToDetails(details *types.Struct, flags []*model.InternalFlag) *types.Struct {
	ints := make([]int, 0, len(flags))
	for _, f := range flags {
		ints = append(ints, int(f.Value))
	}
	return addToDetails(details, ints)
}

func addToDetails(details *types.Struct, flags []int) *types.Struct {
	if len(flags) == 0 {
		return details
	}

	if details == nil {
		details = &types.Struct{
			Fields: map[string]*types.Value{},
		}
	}
	if details.Fields == nil {
		details.Fields = map[string]*types.Value{}
	}
	details.Fields[bundle.RelationKeyInternalFlags.String()] = pbtypes.IntList(flags...)

	return details
}
