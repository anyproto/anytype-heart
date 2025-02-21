package state

import (
	"slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

// details getters

func (s *State) Details() *domain.Details {
	if s.details == nil && s.parent != nil {
		return s.parent.Details()
	}
	return s.details
}

func (s *State) LocalDetails() *domain.Details {
	if s.localDetails == nil && s.parent != nil {
		return s.parent.LocalDetails()
	}

	return s.localDetails
}

func (s *State) CombinedDetails() *domain.Details {
	// TODO Implement combined details struct with two underlying details
	return s.Details().Merge(s.LocalDetails())
}

func (s *State) AllRelationKeys() []domain.RelationKey {
	return append(s.Details().Keys(), s.LocalDetails().Keys()...)
}

func (s *State) HasRelation(key domain.RelationKey) bool {
	return slices.Contains(s.AllRelationKeys(), key)
}

func (s *State) FileRelationKeys(relLinkGetter relationLinkGetter) []domain.RelationKey {
	var keys []domain.RelationKey
	for _, key := range s.AllRelationKeys() {
		// coverId can contain both hash or predefined cover id
		if key == bundle.RelationKeyCoverId {
			coverType := s.Details().GetInt64(bundle.RelationKeyCoverType)
			if (coverType == 1 || coverType == 4 || coverType == 5) && slice.FindPos(keys, key) == -1 {
				keys = append(keys, key)
			}
			continue
		}
		relLink, err := relLinkGetter.GetRelationLink(key.String())
		if err != nil {
			continue
		}
		if relLink.Format == model.RelationFormat_file {
			if slice.FindPos(keys, key) == -1 {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// details setters

func (s *State) SetDetails(d *domain.Details) *State {
	// TODO: GO-2062 Need to refactor details shortening, as it could cut string incorrectly
	// if d != nil && d.Fields != nil {
	//	shortenDetailsToLimit(s.rootId, d.Fields)
	// }

	local := d.CopyOnlyKeys(bundle.LocalAndDerivedRelationKeys...)
	if local != nil && local.Len() > 0 {
		for k, v := range local.Iterate() {
			s.SetLocalDetail(k, v)
		}
		s.details = d.CopyWithoutKeys(bundle.LocalAndDerivedRelationKeys...)
		return s
	}
	s.details = d
	return s
}

func (s *State) SetLocalDetails(d *domain.Details) {
	s.localDetails = d
}

func (s *State) AddDetails(details *domain.Details) {
	for k, v := range details.Iterate() {
		s.SetDetail(k, v)
	}
}

func (s *State) AddLocalDetails(localDetails *domain.Details) {
	for k, v := range localDetails.Iterate() {
		s.SetDetail(k, v)
	}
}

func (s *State) SetDetail(key domain.RelationKey, value domain.Value) {
	// TODO: GO-2062 Need to refactor details shortening, as it could cut string incorrectly
	// value = shortenValueToLimit(s.rootId, key, value)

	if slice.FindPos(bundle.LocalAndDerivedRelationKeys, key) > -1 {
		s.SetLocalDetail(key, value)
		return
	}

	if s.details == nil && s.parent != nil {
		d := s.parent.Details()
		if d != nil {
			// optimisation so we don't need to copy the struct if nothing has changed
			if prev := d.Get(key); prev.Ok() && prev.Equal(value) {
				return
			}
			s.details = d.Copy()
		}
	}
	if s.details == nil {
		s.details = domain.NewDetails()
	}
	s.details.Set(key, value)
	return
}

func (s *State) SetLocalDetail(key domain.RelationKey, value domain.Value) {
	if s.localDetails == nil && s.parent != nil {
		d := s.parent.LocalDetails()
		if d != nil {
			// optimisation so we don't need to copy the struct if nothing has changed
			if prev := d.Get(key); prev.Ok() && prev.Equal(value) {
				return
			}
			s.localDetails = d.Copy()
		}
	}
	if s.localDetails == nil {
		s.localDetails = domain.NewDetails()
	}
	s.localDetails.Set(key, value)
	return
}

// AddRelationKeys adds details with null value, if no detail corresponding to key was presented
func (s *State) AddRelationKeys(keys ...domain.RelationKey) {
	allKeys := s.AllRelationKeys()
	for _, key := range keys {
		if slices.Contains(allKeys, key) {
			continue
		}
		s.SetDetail(key, domain.Null())
	}
}

// details removers

func (s *State) RemoveRelation(keys ...domain.RelationKey) {
	// remove detail value
	s.RemoveDetail(keys...)
	// remove from the list of featured relations
	var foundInFeatured bool
	featuredList := s.Details().GetStringList(bundle.RelationKeyFeaturedRelations)
	featuredList = slice.Filter(featuredList, func(s string) bool {
		if slice.FindPos(keys, domain.RelationKey(s)) == -1 {
			return true
		}
		foundInFeatured = true
		return false
	})
	if foundInFeatured {
		s.SetDetail(bundle.RelationKeyFeaturedRelations, domain.StringList(featuredList))
	}
	return
}

func (s *State) RemoveDetail(keys ...domain.RelationKey) (ok bool) {
	// TODO It could be lazily copied only if actual deletion is happened
	det := s.Details().Copy()
	if det != nil {
		for _, key := range keys {
			if det.Has(key) {
				det.Delete(key)
				ok = true
			}
		}
	}
	if ok {
		s.SetDetails(det)
	}
	return s.RemoveLocalDetail(keys...) || ok
}

func (s *State) RemoveLocalDetail(keys ...domain.RelationKey) (ok bool) {
	// TODO It could be lazily copied only if actual deletion is happened
	det := s.LocalDetails().Copy()
	if det != nil {
		for _, key := range keys {
			if det.Has(key) {
				det.Delete(key)
				ok = true
			}
		}
	}
	if ok {
		s.SetLocalDetails(det)
	}
	return
}
