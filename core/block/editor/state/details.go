package state

import (
	"iter"
	"slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	return slices.Collect[domain.RelationKey](s.iterateKeys())
}

func (s *State) HasRelation(key domain.RelationKey) bool {
	return slice.ContainsBySeq(s.iterateKeys(), key)
}

func (s *State) FileRelationKeys(fetcher relationutils.RelationFormatFetcher) []domain.RelationKey {
	var keys []domain.RelationKey
	for key := range s.iterateKeys() {
		// coverId can contain both hash or predefined cover id
		if key == bundle.RelationKeyCoverId {
			coverType := s.Details().GetInt64(bundle.RelationKeyCoverType)
			if (coverType == 1 || coverType == 4 || coverType == 5) && slice.FindPos(keys, key) == -1 {
				keys = append(keys, key)
			}
			continue
		}
		format, err := fetcher.GetRelationFormatByKey(s.SpaceID(), key)
		if err != nil {
			continue
		}
		if format == model.RelationFormat_file {
			if slice.FindPos(keys, key) == -1 {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

func (s *State) iterateKeys() iter.Seq[domain.RelationKey] {
	return func(yield func(domain.RelationKey) bool) {
		for _, seq := range []iter.Seq[domain.RelationKey]{
			s.Details().IterateKeys(),
			s.LocalDetails().IterateKeys(),
		} {
			for key := range seq {
				if !yield(key) {
					return
				}
			}
		}
	}
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
}

// AddRelationKeys adds details with null value, if no detail corresponding to key was presented
func (s *State) AddRelationKeys(keys ...domain.RelationKey) {
	for _, key := range keys {
		if s.HasRelation(key) {
			continue
		}
		s.SetDetail(key, domain.Null())
	}
}

// details removers

func (s *State) RemoveRelation(keys ...domain.RelationKey) {
	// TODO: GO-4284 remove logic regarding relationLinks
	relLinks := s.getRelationLinks()
	relLinksFiltered := make(pbtypes.RelationLinks, 0, len(relLinks))
	for _, link := range relLinks {
		if slice.FindPos(keys, domain.RelationKey(link.Key)) >= 0 {
			continue
		}
		relLinksFiltered = append(relLinksFiltered, &model.RelationLink{
			Key:    link.Key,
			Format: link.Format,
		})
	}
	s.relationLinks = relLinksFiltered
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
