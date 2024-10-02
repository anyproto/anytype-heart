package spaceindex

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// GetDetails returns empty struct without errors in case details are not found
// todo: get rid of this or change the name method!
func (s *dsObjectStore) GetDetails(id string) (*domain.Details, error) {
	doc, err := s.objects.FindId(s.componentCtx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return domain.NewDetails(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	details, err := domain.JsonToProto(doc.Value())
	if err != nil {
		return nil, fmt.Errorf("unmarshal details: %w", err)
	}
	return details, nil
}

func (s *dsObjectStore) GetUniqueKeyById(id string) (domain.UniqueKey, error) {
	details, err := s.GetDetails(id)
	if err != nil {
		return nil, err
	}
	rawUniqueKey, ok := details.TryString(bundle.RelationKeyUniqueKey)
	if !ok {
		return nil, fmt.Errorf("object does not have unique key in details")
	}
	return domain.UnmarshalUniqueKey(rawUniqueKey)
}

func (s *dsObjectStore) List(includeArchived bool) ([]*database.ObjectInfo, error) {
	var filters []database.FilterRequest
	if includeArchived {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyIsArchived,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Bool(true),
		})
	}
	ids, _, err := s.QueryObjectIds(database.Query{
		Filters: filters,
	})
	if err != nil {
		return nil, fmt.Errorf("query object ids: %w", err)
	}
	return s.GetInfosByIds(ids)
}

func (s *dsObjectStore) HasIds(ids []string) (exists []string, err error) {
	for _, id := range ids {
		_, err := s.objects.FindId(s.componentCtx, id)
		if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
			return nil, fmt.Errorf("get %s: %w", id, err)
		}
		if err == nil {
			exists = append(exists, id)
		}
	}
	return exists, err
}

func (s *dsObjectStore) GetInfosByIds(ids []string) ([]*database.ObjectInfo, error) {
	return s.getObjectsInfo(s.componentCtx, ids)
}

func (s *dsObjectStore) getObjectInfo(ctx context.Context, id string) (*database.ObjectInfo, error) {
	details, err := s.sourceService.DetailsFromIdBasedSource(id)
	if err == nil {
		details.SetString(bundle.RelationKeyId, id)
		return &database.ObjectInfo{
			Id:      id,
			Details: details,
		}, nil
	}

	doc, err := s.objects.FindId(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	details, err = domain.JsonToProto(doc.Value())
	if err != nil {
		return nil, fmt.Errorf("unmarshal details: %w", err)
	}
	snippet := details.GetString(bundle.RelationKeySnippet)

	return &database.ObjectInfo{
		Id:      id,
		Details: details,
		Snippet: snippet,
	}, nil
}

func (s *dsObjectStore) getObjectsInfo(ctx context.Context, ids []string) ([]*database.ObjectInfo, error) {
	objects := make([]*database.ObjectInfo, 0, len(ids))
	for _, id := range ids {
		info, err := s.getObjectInfo(ctx, id)
		if err != nil {
			if errors.Is(err, anystore.ErrDocNotFound) || errors.Is(err, ErrObjectNotFound) || errors.Is(err, ErrNotAnObject) {
				continue
			}
			return nil, err
		}
		if f := info.Details; f != nil {
			// skip deleted objects
			if v, ok := f.TryBool(bundle.RelationKeyIsDeleted); ok && v {
				continue
			}
		}
		objects = append(objects, info)
	}

	return objects, nil
}

func (s *dsObjectStore) GetObjectByUniqueKey(uniqueKey domain.UniqueKey) (*domain.Details, error) {
	records, err := s.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey,
				Value:       domain.String(uniqueKey.Marshal()),
			},
		},
		Limit: 2,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, ErrObjectNotFound
	}

	if len(records) > 1 {
		// should never happen
		return nil, fmt.Errorf("multiple objects with unique key %s", uniqueKey)
	}

	return records[0].Details, nil
}
