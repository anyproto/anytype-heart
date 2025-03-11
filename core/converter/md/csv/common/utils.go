package common

import (
	"bytes"
	"encoding/csv"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func ExtractHeaders(spaceIndex spaceindex.Store, keys []string) ([]string, []string, error) {
	records, err := spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(keys),
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	recordMap := make(map[string]string, len(records))
	for _, record := range records {
		if record.Details.GetBool(bundle.RelationKeyIsDeleted) {
			continue
		}
		key := record.Details.GetString(bundle.RelationKeyRelationKey)
		recordMap[key] = record.Details.GetString(bundle.RelationKeyName)
	}

	headersNames := make([]string, 0, len(recordMap))
	resultKeys := make([]string, 0, len(recordMap))
	for _, key := range keys {
		if name, exists := recordMap[key]; exists {
			headersNames = append(headersNames, name)
			resultKeys = append(resultKeys, key)
		}
	}

	return resultKeys, headersNames, nil
}

func WriteCSV(csvRows [][]string) (*bytes.Buffer, error) {
	buffer := bytes.NewBuffer(nil)
	csvWriter := csv.NewWriter(buffer)
	defer csvWriter.Flush()

	err := csvWriter.WriteAll(csvRows)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}
