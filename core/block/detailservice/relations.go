package detailservice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/slice"
	timeutil "github.com/anyproto/anytype-heart/util/time"
)

var (
	ErrBundledTypeIsReadonly = fmt.Errorf("can't modify bundled object type")
	// ErrTypePropertyBadInput is the request's fault: an unknown section, a
	// key no relation in the space carries, or a mint with no name.
	ErrTypePropertyBadInput = errors.New("bad type property input")
)

// TypePropertySection names one of the four lists a type recommends its
// properties in. Recommended, Featured and Hidden match
// Rpc.ObjectType.Property.Add.Request.Section; File has no wire value, because
// the file list is derived rather than chosen — Add refuses it in either
// direction. It is named here because Remove still has to strip the property
// from all four.
type TypePropertySection int

const (
	TypePropertySectionRecommended TypePropertySection = iota
	TypePropertySectionFeatured
	TypePropertySectionHidden
	TypePropertySectionFile
)

// typePropertySectionKeys lists the four recommended lists in section order,
// so a section indexes its list and a property moves between them by joining
// one and leaving the other three.
var typePropertySectionKeys = []domain.RelationKey{
	TypePropertySectionRecommended: bundle.RelationKeyRecommendedRelations,
	TypePropertySectionFeatured:    bundle.RelationKeyRecommendedFeaturedRelations,
	TypePropertySectionHidden:      bundle.RelationKeyRecommendedHiddenRelations,
	TypePropertySectionFile:        bundle.RelationKeyRecommendedFileRelations,
}

type ObjectTypePropertyAddRequest struct {
	ObjectTypeId string
	// Key is an existing property's key; empty means mint one from Name and Format
	Key    domain.RelationKey
	Name   string
	Format model.RelationFormat
	// Section is the recommended list the property joins (and the only one it stays in)
	Section TypePropertySection
	// EnableInViews is the visibility every view's new column gets
	EnableInViews bool
}

type ObjectTypePropertyAddResult struct {
	Key        domain.RelationKey
	PropertyId string
	// ViewIds lists the views that gained a column
	ViewIds []string
}

type ObjectTypePropertyRemoveResult struct {
	// InUseViewIds lists the views left untouched because they group, sort or
	// filter by the property
	InUseViewIds []string
}

// resolvedProperty is what the type's apply needs to know about a property:
// the object id its lists hold and the link its dataview carries.
type resolvedProperty struct {
	id   string
	link *model.RelationLink
}

func (s *service) ObjectTypePropertyAdd(ctx context.Context, req ObjectTypePropertyAddRequest) (ObjectTypePropertyAddResult, error) {
	var res ObjectTypePropertyAddResult
	// everything that can be rejected is rejected before anything is
	// created: a minted relation is a separate object the type's apply
	// cannot roll back
	if strings.HasPrefix(req.ObjectTypeId, bundle.TypePrefix) {
		return res, ErrBundledTypeIsReadonly
	}
	if req.Section < 0 || int(req.Section) >= len(typePropertySectionKeys) {
		return res, fmt.Errorf("%w: unknown section %d", ErrTypePropertyBadInput, req.Section)
	}
	// the file section is not the caller's to set: it is derived by
	// relationutils.FillRecommendedRelations from a fixed set of file-metadata
	// keys, on the four file types only, and systemobjectreviser recomputes it
	// wholesale — so a value written here is overwritten without notice the
	// next time those types are revised. Rpc.…Property.Add.Request.Section
	// reserves the number rather than offering it; this covers a direct caller.
	if req.Section == TypePropertySectionFile {
		return res, fmt.Errorf("%w: the file section is derived from the file types' metadata keys, not set by a caller", ErrTypePropertyBadInput)
	}
	if req.Key == "" {
		if req.Name == "" {
			return res, fmt.Errorf("%w: a name is required to create a property", ErrTypePropertyBadInput)
		}
		// String() of an unknown enum value is its number, not "", so the
		// name table is the only real check
		if _, known := model.RelationFormat_name[int32(req.Format)]; !known {
			return res, fmt.Errorf("%w: unknown format %d", ErrTypePropertyBadInput, req.Format)
		}
	}
	var spaceId string
	var fileSectioned []string
	err := cache.Do(s.objectGetter, req.ObjectTypeId, func(b smartblock.SmartBlock) error {
		if err := checkDetailsEditable(b); err != nil {
			return err
		}
		spaceId = b.Space().Id()
		fileSectioned = b.NewState().Details().GetStringList(bundle.RelationKeyRecommendedFileRelations)
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("open type %s: %w", req.ObjectTypeId, err)
	}

	var prop resolvedProperty
	if req.Key == "" {
		prop, err = s.mintProperty(ctx, spaceId, req.Name, req.Format)
	} else {
		prop, err = s.resolveProperty(ctx, spaceId, req.Key)
	}
	if err != nil {
		return res, err
	}
	// the other direction of the same rule: a property the type already keeps
	// in its file section cannot be moved into an authorable one, because the
	// move edits the derived list and the reviser puts it back. A property just
	// minted cannot be in that list, so this only ever fires on a resolved key.
	if slices.Contains(fileSectioned, prop.id) {
		return res, fmt.Errorf("%w: %s sits in the type's file section, which is derived rather than set", ErrTypePropertyBadInput, prop.link.Key)
	}

	err = cache.Do(s.objectGetter, req.ObjectTypeId, func(b smartblock.SmartBlock) error {
		if err := checkDetailsEditable(b); err != nil {
			return err
		}
		st := b.NewState()
		// the property joins the section named and leaves the other three,
		// so naming a different section moves it; naming the section it is
		// already in leaves the list as it is, position included
		for section, listKey := range typePropertySectionKeys {
			list := st.Details().GetStringList(listKey)
			var updated []string
			if TypePropertySection(section) == req.Section {
				if slices.Contains(list, prop.id) {
					continue
				}
				updated = append(slices.Clone(list), prop.id)
			} else {
				updated = slice.RemoveMut(slices.Clone(list), prop.id)
			}
			if !slices.Equal(list, updated) {
				st.SetDetailAndBundledRelation(listKey, domain.StringList(updated))
			}
		}
		if block := template.TypeDataviewBlock(st); block != nil {
			edited := block.Copy()
			var changed bool
			res.ViewIds, changed = template.AddTypeDataviewColumn(edited.Model().GetDataview(), prop.link, req.EnableInViews)
			if changed {
				st.Set(edited)
			}
		}
		return b.Apply(st)
	})
	if err != nil {
		return res, fmt.Errorf("add property %s to type %s: %w", prop.link.Key, req.ObjectTypeId, err)
	}
	res.Key = domain.RelationKey(prop.link.Key)
	res.PropertyId = prop.id
	return res, nil
}

// mintProperty creates the relation object the way ObjectCreateRelation
// does, so a property minted here is indistinguishable from one the client
// created first.
func (s *service) mintProperty(ctx context.Context, spaceId, name string, format model.RelationFormat) (resolvedProperty, error) {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetInt64(bundle.RelationKeyRelationFormat, int64(format))
	id, created, err := s.objectCreator.CreateObject(ctx, spaceId, objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyRelation,
		Details:       details,
	})
	if err != nil {
		return resolvedProperty{}, fmt.Errorf("create property %q: %w", name, err)
	}
	key := created.GetString(bundle.RelationKeyRelationKey)
	if key == "" {
		return resolvedProperty{}, fmt.Errorf("create property %q: created object %s carries no key", name, id)
	}
	return resolvedProperty{id: id, link: &model.RelationLink{Key: key, Format: format}}, nil
}

// resolveProperty finds the relation object a key names in the space. A
// misspelt key is the caller's error, not a reason to mint: a live relation
// object must carry it. A bundled key the space has not installed yet is
// installed the way type creation installs its recommended relations —
// idempotent, so a later failure leaves nothing behind that a retry would
// not reuse.
func (s *service) resolveProperty(ctx context.Context, spaceId string, key domain.RelationKey) (resolvedProperty, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(key.String()),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsUninstalled,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
		Limit: 1,
	})
	if err != nil {
		return resolvedProperty{}, fmt.Errorf("query property %s: %w", key, err)
	}
	if len(records) > 0 {
		details := records[0].Details
		return resolvedProperty{
			id: details.GetString(bundle.RelationKeyId),
			link: &model.RelationLink{
				Key:    key.String(),
				Format: model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat)),
			},
		}, nil
	}
	bundled, err := bundle.GetRelation(key)
	if err != nil {
		return resolvedProperty{}, fmt.Errorf("%w: no property with key %q in space", ErrTypePropertyBadInput, key)
	}
	spc, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return resolvedProperty{}, fmt.Errorf("get space %s: %w", spaceId, err)
	}
	if _, _, err = s.objectCreator.InstallBundledObjects(ctx, spc, []string{key.BundledURL()}); err != nil {
		return resolvedProperty{}, fmt.Errorf("install bundled property %s: %w", key, err)
	}
	id, err := spc.GetRelationIdByKey(ctx, key)
	if err != nil {
		return resolvedProperty{}, fmt.Errorf("get relation id by key %s: %w", key, err)
	}
	return resolvedProperty{id: id, link: &model.RelationLink{Key: key.String(), Format: bundled.Format}}, nil
}

func (s *service) ObjectTypePropertyRemove(ctx context.Context, objectTypeId string, key domain.RelationKey) (ObjectTypePropertyRemoveResult, error) {
	var res ObjectTypePropertyRemoveResult
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return res, ErrBundledTypeIsReadonly
	}
	if key == "" {
		return res, fmt.Errorf("%w: a property key is required", ErrTypePropertyBadInput)
	}
	err := cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		if err := checkDetailsEditable(b); err != nil {
			return err
		}
		relId, err := b.Space().GetRelationIdByKey(ctx, key)
		if err != nil {
			return fmt.Errorf("get relation id by key %s: %w", key, err)
		}
		st := b.NewState()
		// the file section is derived, not authorable, so Remove is not a back
		// door into it either: refusing leaves the type whole, where stripping
		// the list would edit something the reviser recomputes and pruning only
		// the views would leave the list naming a property with no column.
		if slices.Contains(st.Details().GetStringList(bundle.RelationKeyRecommendedFileRelations), relId) {
			return fmt.Errorf("%w: %s sits in the type's file section, which is derived rather than set", ErrTypePropertyBadInput, key)
		}
		// a key the type does not list is not an error: the views and the
		// link are still converged, which is what a client calling this on a
		// half-consistent type needs
		for _, listKey := range typePropertySectionKeys {
			list := st.Details().GetStringList(listKey)
			updated := slice.RemoveMut(slices.Clone(list), relId)
			if !slices.Equal(list, updated) {
				st.SetDetailAndBundledRelation(listKey, domain.StringList(updated))
			}
		}
		if block := template.TypeDataviewBlock(st); block != nil {
			edited := block.Copy()
			dv := edited.Model().GetDataview()
			keys := []string{key.String()}
			plan := template.PlanTypeDataviewColumnPrune(dv, keys)
			for _, inUse := range plan.InUse {
				res.InUseViewIds = append(res.InUseViewIds, inUse.ViewId)
			}
			pruned := template.PruneTypeDataviewColumns(dv, keys)
			if template.DropUnreferencedTypeDataviewLink(dv, key.String()) || pruned {
				st.Set(edited)
			}
		}
		return b.Apply(st)
	})
	if err != nil {
		return res, fmt.Errorf("remove property %s from type %s: %w", key, objectTypeId, err)
	}
	return res, nil
}

func (s *service) ObjectTypeSetRelations(objectTypeId string, relationObjectIds []string) error {
	return s.objectTypeSetRelations(objectTypeId, relationObjectIds, false)
}

func (s *service) ObjectTypeSetFeaturedRelations(objectTypeId string, relationObjectIds []string) error {
	return s.objectTypeSetRelations(objectTypeId, relationObjectIds, true)
}

func (s *service) objectTypeSetRelations(
	objectTypeId string, relationList []string, isFeatured bool,
) error {
	if strings.HasPrefix(objectTypeId, bundle.TypePrefix) {
		return ErrBundledTypeIsReadonly
	}
	relationToSet := bundle.RelationKeyRecommendedRelations
	if isFeatured {
		relationToSet = bundle.RelationKeyRecommendedFeaturedRelations
	}
	return cache.Do(s.objectGetter, objectTypeId, func(b smartblock.SmartBlock) error {
		if err := checkDetailsEditable(b); err != nil {
			return err
		}
		st := b.NewState()
		st.SetDetailAndBundledRelation(relationToSet, domain.StringList(relationList))
		return b.Apply(st)
	})
}

func (s *service) ObjectTypeListConflictingRelations(spaceId, typeObjectId string) ([]string, error) {
	records, err := s.store.SpaceIndex(spaceId).QueryByIds([]string{typeObjectId})
	if err != nil {
		return nil, fmt.Errorf("failed to query object type: %w", err)
	}

	if len(records) != 1 {
		return nil, fmt.Errorf("failed to query object type, expected 1 record")
	}

	details := records[0].Details
	allRecommendedRelations := lo.Uniq(slices.Concat(
		details.GetStringList(bundle.RelationKeyRecommendedRelations),
		details.GetStringList(bundle.RelationKeyRecommendedFeaturedRelations),
		details.GetStringList(bundle.RelationKeyRecommendedHiddenRelations),
		details.GetStringList(bundle.RelationKeyRecommendedFileRelations),
	))

	allRelationKeys := make([]string, 0, len(allRecommendedRelations))
	err = s.store.SpaceIndex(spaceId).QueryIterate(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(typeObjectId),
		},
	}}, func(details *domain.Details) {
		for _, key := range details.Keys() {
			if !slices.Contains(allRelationKeys, string(key)) {
				allRelationKeys = append(allRelationKeys, string(key))
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate over all objects to collect their relations: %w", err)
	}

	records, err = s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(allRelationKeys),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch relations by keys: %w", err)
	}

	conflictingRelations := make([]string, 0, len(records))
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if !slices.Contains(allRecommendedRelations, id) {
			conflictingRelations = append(conflictingRelations, id)
		}
	}

	return conflictingRelations, nil
}

func (s *service) ListRelationsWithValue(spaceId string, value domain.Value) ([]*pb.RpcRelationListWithValueResponseResponseItem, error) {
	var (
		countersByKeys     = make(map[domain.RelationKey]int64)
		detailHandlesValue = generateFilter(value)
	)

	err := s.store.SpaceIndex(spaceId).QueryIterate(
		database.Query{Filters: nil},
		func(details *domain.Details) {
			for key, valueToCheck := range details.Iterate() {
				if detailHandlesValue(valueToCheck) {
					if counter, ok := countersByKeys[key]; ok {
						countersByKeys[key] = counter + 1
					} else {
						countersByKeys[key] = 1
					}
				}
			}
		})

	if err != nil {
		return nil, fmt.Errorf("failed to query objects: %w", err)
	}

	keys := maps.Keys(countersByKeys)
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == bundle.RelationKeyMentions {
			return true
		}
		if keys[j] == bundle.RelationKeyMentions {
			return false
		}
		return keys[i] < keys[j]
	})

	list := make([]*pb.RpcRelationListWithValueResponseResponseItem, len(keys))
	for i, key := range keys {
		list[i] = &pb.RpcRelationListWithValueResponseResponseItem{
			RelationKey: string(key),
			Counter:     countersByKeys[key],
		}
	}

	return list, nil
}

func generateFilter(value domain.Value) func(v domain.Value) bool {
	equalOrHasFilter := func(v domain.Value) bool {
		if list, ok := v.TryListValues(); ok {
			for _, element := range list {
				if element.Equal(value) {
					return true
				}
			}
		}
		return v.Equal(value)
	}

	stringValue := value.String()
	if stringValue == "" {
		return equalOrHasFilter
	}

	sbt, err := typeprovider.SmartblockTypeFromID(stringValue)
	if err != nil {
		log.Error("failed to determine smartblock type", zap.Error(err))
	}

	if sbt != coresb.SmartBlockTypeDate {
		return equalOrHasFilter
	}

	// date object section

	dateObject, err := dateutil.BuildDateObjectFromId(stringValue)
	if err != nil {
		log.Error("failed to parse Date object id", zap.Error(err))
		return equalOrHasFilter
	}

	start := timeutil.CutToDay(dateObject.Time())
	end := start.Add(24 * time.Hour)
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	startDateObject := dateutil.NewDateObject(start, false)
	shortId := startDateObject.Id()

	// filter for date objects is able to find relations with values between the borders of queried day
	// - for relations with number format it checks timestamp value is between timestamps of this day midnights
	// - for relations carrying string list it checks if some of the strings has day prefix, e.g.
	// if _date_2023-12-12-08-30-50Z-0200 is queried, then all relations with prefix _date_2023-12-12 will be returned
	return func(v domain.Value) bool {
		numberValue := v.Int64()
		if numberValue >= startTimestamp && numberValue < endTimestamp {
			return true
		}

		for _, element := range v.WrapToList() {
			if element.Equal(value) {
				return true
			}
			if strings.HasPrefix(element.String(), shortId) {
				return true
			}
		}

		return v.Equal(value)
	}
}
